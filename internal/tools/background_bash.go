package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"taiji-code/types"
	"time"
)

// ─── 后台进程管理 ─────────────────────────────────────────────────────────

// BackgroundProcess 后台进程
type BackgroundProcess struct {
	ID        string
	Command   string
	Cmd       *exec.Cmd
	Output    strings.Builder
	StartTime time.Time
	Done      bool
	ExitCode  int
	mu        sync.Mutex
}

// ProcessManager 后台进程管理器
type ProcessManager struct {
	mu       sync.Mutex
	procs    map[string]*BackgroundProcess
	counter  int
}

// NewProcessManager 创建进程管理器
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		procs: make(map[string]*BackgroundProcess),
	}
}

// Spawn 启动后台进程
func (pm *ProcessManager) Spawn(command, workDir string) string {
	pm.mu.Lock()
	pm.counter++
	id := fmt.Sprintf("bg_%d", pm.counter)
	pm.mu.Unlock()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(cmd.Environ(), "TERM=dumb")

	proc := &BackgroundProcess{
		ID:        id,
		Command:   command,
		Cmd:       cmd,
		StartTime: time.Now(),
	}

	// 实际用pipe来持续读取
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		proc.mu.Lock()
		proc.Output.WriteString(fmt.Sprintf("创建管道失败: %v", err))
		proc.Done = true
		proc.ExitCode = -1
		proc.mu.Unlock()
	} else {
		cmd.Stderr = cmd.Stdout // 合并stderr

		if err := cmd.Start(); err != nil {
			proc.mu.Lock()
			proc.Output.WriteString(fmt.Sprintf("启动失败: %v", err))
			proc.Done = true
			proc.ExitCode = -1
			proc.mu.Unlock()
		} else {
			// 后台读取输出
			go func() {
				ioBuf := make([]byte, 4096)
				for {
					n, err := stdout.Read(ioBuf)
					if n > 0 {
						proc.mu.Lock()
						proc.Output.Write(ioBuf[:n])
						proc.mu.Unlock()
					}
					if err != nil {
						break
					}
				}
			}()

			// 后台等待完成
			go func() {
				_ = cmd.Wait()
				proc.mu.Lock()
				proc.Done = true
				if cmd.ProcessState != nil {
					proc.ExitCode = cmd.ProcessState.ExitCode()
				}
				proc.mu.Unlock()
			}()
		}
	}

	pm.mu.Lock()
	pm.procs[id] = proc
	pm.mu.Unlock()

	return id
}

// GetOutput 获取进程输出
func (pm *ProcessManager) GetOutput(id string) (string, bool, error) {
	pm.mu.Lock()
	proc, ok := pm.procs[id]
	pm.mu.Unlock()
	if !ok {
		return "", false, fmt.Errorf("进程 %s 不存在", id)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	output := proc.Output.String()
	if output == "" {
		output = "[暂无输出]"
	}
	return output, proc.Done, nil
}

// Kill 终止进程
func (pm *ProcessManager) Kill(id string) error {
	pm.mu.Lock()
	proc, ok := pm.procs[id]
	pm.mu.Unlock()
	if !ok {
		return fmt.Errorf("进程 %s 不存在", id)
	}

	if proc.Done {
		return fmt.Errorf("进程 %s 已结束", id)
	}

	return proc.Cmd.Process.Kill()
}

// List 列出所有后台进程
func (pm *ProcessManager) List() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var result []string
	for id, proc := range pm.procs {
		status := "运行中"
		if proc.Done {
			status = fmt.Sprintf("已完成(退出码:%d)", proc.ExitCode)
		}
		elapsed := time.Since(proc.StartTime).Truncate(time.Second)
		result = append(result, fmt.Sprintf("  %s: %s [%s] (%v)", id, proc.Command, status, elapsed))
	}
	return result
}

// ─── 工具定义 ────────────────────────────────────────────────────────────

// BackgroundBash 后台执行命令
type BackgroundBash struct {
	Manager *ProcessManager
	WorkDir string
}

func (t *BackgroundBash) Name() string       { return "bash_bg" }
func (t *BackgroundBash) Description() string { return "在后台执行耗时命令，不阻塞对话。用bash_output查看结果，bash_kill终止进程。" }
func (t *BackgroundBash) PermissionLevel() types.PermissionLevel {
	return types.PermAskUser
}

func (t *BackgroundBash) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要后台执行的命令",
			},
		},
		"required": []interface{}{"command"},
	}
}

func (t *BackgroundBash) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return "", fmt.Errorf("command is required")
	}

	id := t.Manager.Spawn(cmd, t.WorkDir)
	return fmt.Sprintf("后台进程已启动: %s\n用 bash_output 查看输出，bash_kill 终止进程", id), nil
}

// BashOutput 查看后台进程输出
type BashOutput struct {
	Manager *ProcessManager
}

func (t *BashOutput) Name() string       { return "bash_output" }
func (t *BashOutput) Description() string { return "查看后台进程的输出。不指定ID则列出所有后台进程。" }
func (t *BashOutput) PermissionLevel() types.PermissionLevel {
	return types.PermAutoApprove
}

func (t *BashOutput) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"process_id": map[string]interface{}{
				"type":        "string",
				"description": "进程ID(不指定则列出所有后台进程)",
			},
		},
	}
}

func (t *BashOutput) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	id, _ := args["process_id"].(string)

	if id == "" {
		procs := t.Manager.List()
		if len(procs) == 0 {
			return "没有运行中的后台进程", nil
		}
		return "后台进程:\n" + strings.Join(procs, "\n"), nil
	}

	output, done, err := t.Manager.GetOutput(id)
	if err != nil {
		return "", err
	}

	status := "运行中"
	if done {
		status = "已完成"
	}
	return fmt.Sprintf("[%s] %s:\n%s", id, status, output), nil
}

// BashKill 终止后台进程
type BashKill struct {
	Manager *ProcessManager
}

func (t *BashKill) Name() string       { return "bash_kill" }
func (t *BashKill) Description() string { return "终止后台进程。" }
func (t *BashKill) PermissionLevel() types.PermissionLevel {
	return types.PermAskUser
}

func (t *BashKill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"process_id": map[string]interface{}{
				"type":        "string",
				"description": "要终止的进程ID",
			},
		},
		"required": []interface{}{"process_id"},
	}
}

func (t *BashKill) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	id, _ := args["process_id"].(string)
	if id == "" {
		return "", fmt.Errorf("process_id is required")
	}

	if err := t.Manager.Kill(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("进程 %s 已终止", id), nil
}
