package main

import (
	"archive/zip"
	"encoding/base64"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ledongthuc/pdf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Task 表示一个任务
type Task struct {
	ID       int       `json:"id"`
	Title    string    `json:"title"`
	Subtitle string    `json:"subtitle"`
	Time     string    `json:"time"`
	Steps    int       `json:"steps"`
	Status   string    `json:"status"`
	Messages []Message `json:"messages,omitempty"`
}

// Message 表示一条消息
type Message struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	Time     string `json:"time"`
	ToolName string `json:"tool_name,omitempty"`
	ToolArgs string `json:"tool_args,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

// TodoItem 表示待办事项
type TodoItem struct {
	ID        int    `json:"id"`
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// FileInfo 表示文件信息
type FileInfo struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// Config 表示应用配置
type Config struct {
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
	BaseURL string `json:"baseURL"`
}

// Channel 表示 IM 频道
type Channel struct {
	ID        int    `json:"id"`
	Platform  string `json:"platform"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	Desc      string `json:"desc"`
	Connected bool   `json:"connected"`
}

// ChannelInput 添加频道输入
type ChannelInput struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Token    string `json:"token"`
	Desc     string `json:"desc"`
}

// Skill 表示技能
type Skill struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Keywords string `json:"keywords"`
	Enabled  bool   `json:"enabled"`
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
}

// SkillInput 添加技能输入
type SkillInput struct {
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	Keywords    string `json:"keywords"`
	FileContent string `json:"fileContent"`
	FileName    string `json:"fileName"`
}

// CronJob 表示定时任务
type CronJob struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Expr    string `json:"expr"`
	Desc    string `json:"desc"`
	Enabled bool   `json:"enabled"`
	NextRun string `json:"nextRun"`
}

// CronInput 添加定时任务输入
type CronInput struct {
	Name    string `json:"name"`
	Expr    string `json:"expr"`
	Desc    string `json:"desc"`
	Enabled bool   `json:"enabled"`
}

// GuiApp 是 Wails 应用结构
type GuiApp struct {
	ctx           context.Context
	mu            sync.Mutex
	tasks         []*Task
	currentTask   *Task
	workDir       string
	nextTaskID    int
	apiKey        string
	model         string
	baseURL       string
	channels      []*Channel
	nextChannelID int
	skills        []*Skill
	nextSkillID   int
	crons         []*CronJob
	nextCronID    int
	skillsDir     string
	currentCancel context.CancelFunc
}

// NewGuiApp 创建新的 GUI 应用
func NewGuiApp() *GuiApp {
	return &GuiApp{
		tasks:      make([]*Task, 0),
		nextTaskID: 1,
	}
}

// startup 在应用启动时调用
func (a *GuiApp) startup(ctx context.Context) {
	a.ctx = ctx
	a.workDir, _ = os.Getwd()

	// 从环境变量读取 API Key
	a.apiKey = os.Getenv("DEEPSEEK_API_KEY")
	a.model = "deepseek-v4-pro"
	a.baseURL = "https://api.deepseek.com/v1"

	// 创建技能文件存储目录
	a.skillsDir = filepath.Join(a.workDir, "skills")
	os.MkdirAll(a.skillsDir, 0755)

	// 自动加载 skills 目录中已有的技能文件
	a.loadSkillsFromDir()
}

// loadSkillsFromDir 扫描 skills 目录，自动注册技能
func (a *GuiApp) loadSkillsFromDir() {
	entries, err := os.ReadDir(a.skillsDir)
	if err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过非 markdown 文件和非技能文件
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}

		// 检查是否已注册（避免重复）
		alreadyRegistered := false
		for _, sk := range a.skills {
			if sk.FilePath == filepath.Join(a.skillsDir, name) {
				alreadyRegistered = true
				break
			}
		}
		if alreadyRegistered {
			continue
		}

		// 从文件名推断技能名称（去掉扩展名，替换下划线为空格）
		skillName := strings.TrimSuffix(name, filepath.Ext(name))
		skillName = strings.ReplaceAll(skillName, "_", " ")
		skillName = strings.ReplaceAll(skillName, "-", " ")
		// 首字母大写
		if len(skillName) > 0 {
			skillName = strings.ToUpper(skillName[:1]) + skillName[1:]
		}

		sk := &Skill{
			ID:       a.nextSkillID,
			Name:     skillName,
			Desc:     "预设技能：" + skillName,
			Keywords: "",
			Enabled:  true,
			FileName: name,
			FilePath: filepath.Join(a.skillsDir, name),
		}
		a.nextSkillID++
		a.skills = append(a.skills, sk)
	}
}

// GetTasks 获取所有任务
func (a *GuiApp) GetTasks() []*Task {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tasks
}

// CreateTask 创建新任务
func (a *GuiApp) CreateTask(title string) *Task {
	a.mu.Lock()
	defer a.mu.Unlock()

	task := &Task{
		ID:       a.nextTaskID,
		Title:    title,
		Subtitle: "内容由 AI 生成",
		Time:     time.Now().Format("15:04"),
		Steps:    0,
		Status:   "active",
		Messages: make([]Message, 0),
	}
	a.nextTaskID++
	a.tasks = append([]*Task{task}, a.tasks...)
	a.currentTask = task

	return task
}

// SendMessage 发送消息到当前任务
func (a *GuiApp) SendMessage(message string, filePaths string, skillId int, imageBase64 string) string {
	a.mu.Lock()
	if a.currentTask == nil {
		a.mu.Unlock()
		return "错误：没有活跃任务"
	}
	if a.apiKey == "" {
		a.mu.Unlock()
		return "错误：未配置 API KEY"
	}

	// 查找选中的技能
	var skillContent string
	var skillName string
	if skillId > 0 {
		for _, sk := range a.skills {
			if sk.ID == skillId && sk.Enabled {
				skillName = sk.Name
				if sk.FilePath != "" {
					skillContent = a.readFileContent(sk.FilePath)
					fmt.Printf("[DEBUG] Skill found: %s, FilePath: %s, Content length: %d\n", skillName, sk.FilePath, len(skillContent))
				}
				break
			}
		}
		if skillContent == "" {
			fmt.Printf("[DEBUG] skillId=%d but no skill content found. Skills count: %d\n", skillId, len(a.skills))
			for _, sk := range a.skills {
				fmt.Printf("[DEBUG]   Skill ID=%d Name=%s Enabled=%v FilePath=%s\n", sk.ID, sk.Name, sk.Enabled, sk.FilePath)
			}
		}
	} else {
		fmt.Printf("[DEBUG] skillId=0, no skill selected\n")
	}

	// 解析文件路径
	var files []string
	if strings.TrimSpace(filePaths) != "" {
		files = strings.Split(filePaths, ",")
		for i, f := range files {
			files[i] = strings.TrimSpace(f)
		}
	}

	// 读取文件内容
	var fileContents string
	if len(files) > 0 {
		fileContents = a.readFilesContent(files)
	}

	// 构建带文件内容的消息
	fullMessage := message
	if fileContents != "" {
		fullMessage = message + "\n\n--- 导入文件内容 ---\n" + fileContents
	}

	// 处理图片（支持多图，用 |||IMG||| 分隔）
	var imageDatas []string
	if imageBase64 != "" {
		imageDatas = strings.Split(imageBase64, "|||IMG|||")
		// 过滤空字符串
		var filtered []string
		for _, d := range imageDatas {
			if strings.TrimSpace(d) != "" {
				filtered = append(filtered, d)
			}
		}
		imageDatas = filtered
	}

	// 添加用户消息
	displayMsg := message
	var displayExtras []string
	if skillName != "" {
		displayExtras = append(displayExtras, "🛠️ 已调用技能："+skillName)
	}
	if len(files) > 0 {
		var fileNames []string
		for _, f := range files {
			fileNames = append(fileNames, filepath.Base(f))
		}
		displayExtras = append(displayExtras, "📎 已导入文件："+strings.Join(fileNames, "、"))
	}
	if len(displayExtras) > 0 {
		displayMsg = message + "\n\n" + strings.Join(displayExtras, " ")
	}
	a.currentTask.Messages = append(a.currentTask.Messages, Message{
		Type:    "user",
		Content: displayMsg,
		Time:    time.Now().Format("15:04"),
	})
	a.currentTask.Steps++
	a.mu.Unlock()

	// 调用 DeepSeek API
	resp, err := a.callDeepSeekAPI(fullMessage, skillContent, imageDatas)

	a.mu.Lock()
	defer a.mu.Unlock()

	if err != nil {
		errMsg := Message{
			Type:    "error",
			Content: err.Error(),
			Time:    time.Now().Format("15:04"),
			IsError: true,
		}
		a.currentTask.Messages = append(a.currentTask.Messages, errMsg)
		a.currentTask.Status = "error"
		wailsruntime.EventsEmit(a.ctx, "message_update", errMsg)
		return err.Error()
	}

	// 添加助手响应
	assistantMsg := Message{
		Type:    "assistant",
		Content: resp,
		Time:    time.Now().Format("15:04"),
	}
	a.currentTask.Messages = append(a.currentTask.Messages, assistantMsg)
	a.currentTask.Status = "completed"
	wailsruntime.EventsEmit(a.ctx, "message_update", assistantMsg)
	wailsruntime.EventsEmit(a.ctx, "task_complete", map[string]interface{}{
		"taskId": a.currentTask.ID,
		"status": "completed",
	})

	return "完成"
}

// CancelCurrentTask 中止当前正在执行的任务
func (a *GuiApp) CancelCurrentTask() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.currentCancel != nil {
		a.currentCancel()
		return "任务已中止"
	}
	return "没有正在执行的任务"
}

// DeepSeek API 请求结构
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ToolDef     `json:"tools,omitempty"`
}

type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type ChatMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type ToolDef struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function ToolCallFunc    `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// getToolDefinitions 返回可用工具定义
func getToolDefinitions() []ToolDef {
	return []ToolDef{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "shell_exec",
				Description: "在本地执行 shell 命令（Windows 下使用 cmd /c，Linux/Mac 使用 bash -c）。返回 stdout 和 stderr。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的 shell 命令",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "read_file",
				Description: "读取本地文件内容（文本文件）。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径（绝对或相对路径）",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "write_file",
				Description: "将内容写入本地文件。如果文件不存在则创建，存在则覆盖。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "要写入的文件内容",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_files",
				Description: "列出指定目录下的文件和子目录。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "目录路径，默认为当前工作目录",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "screenshot",
				Description: "截取当前屏幕截图，返回 base64 编码的 PNG 图片。可用于查看当前桌面状态。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "click",
				Description: "在屏幕指定坐标位置点击鼠标左键。坐标原点在屏幕左上角。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x": map[string]interface{}{
							"type":        "number",
							"description": "屏幕水平坐标（像素）",
						},
						"y": map[string]interface{}{
							"type":        "number",
							"description": "屏幕垂直坐标（像素）",
						},
					},
					"required": []string{"x", "y"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "type_text",
				Description: "在当前焦点位置模拟键盘输入文本。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"text": map[string]interface{}{
							"type":        "string",
							"description": "要输入的文本内容",
						},
					},
					"required": []string{"text"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "press_key",
				Description: "模拟按下特定键盘按键。支持 Enter、Tab、Escape、Backspace、Delete、方向键(Up/Down/Left/Right)、F1-F12 等。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key": map[string]interface{}{
							"type":        "string",
							"description": "按键名称，如 Enter、Tab、Escape、Backspace、Up、Down、Left、Right、F1 等",
						},
					},
					"required": []string{"key"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "generate_image",
				Description: "根据文字描述生成 AI 图片。仅在用户明确要求生成/画图片时才调用。如果用户只是描述场景或写文案，不要调用此工具。图片将保存到工作目录。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "图片描述（英文效果更佳），例如：a cyberpunk city at night, neon lights, rain",
						},
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "保存文件名（可选，默认自动生成）",
						},
					},
					"required": []string{"prompt"},
				},
			},
		},
	}
}

// executeTool 执行工具调用并返回结果
func (a *GuiApp) executeTool(name string, argsJSON string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "工具参数解析失败：" + err.Error()
	}

	switch name {
	case "shell_exec":
		cmdStr, _ := args["command"].(string)
		if cmdStr == "" {
			return "错误：command 参数为空"
		}
		return a.execShell(cmdStr)

	case "read_file":
		path, _ := args["path"].(string)
		if path == "" {
			return "错误：path 参数为空"
		}
		// 相对路径基于 workDir
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.workDir, path)
		}
		content := a.readFileContent(path)
		if content == "" {
			return "[文件为空或无法读取]"
		}
		return content

	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" {
			return "错误：path 参数为空"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.workDir, path)
		}
		// 确保目录存在
		dir := filepath.Dir(path)
		os.MkdirAll(dir, 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "写入失败：" + err.Error()
		}
		return fmt.Sprintf("文件已写入：%s（%d 字节）", path, len(content))

	case "list_files":
		path, _ := args["path"].(string)
		if path == "" {
			path = a.workDir
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.workDir, path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return "列出目录失败：" + err.Error()
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("目录：%s\n", path))
		for _, e := range entries {
			if e.IsDir() {
				sb.WriteString(fmt.Sprintf("  [目录] %s/\n", e.Name()))
			} else {
				info, _ := e.Info()
				size := info.Size()
				sb.WriteString(fmt.Sprintf("  [文件] %s (%s)\n", e.Name(), formatSize(size)))
			}
		}
		return sb.String()

	case "screenshot":
		// 截图并返回 base64
		return a.takeScreenshot()

	case "click":
		x, _ := args["x"].(float64)
		y, _ := args["y"].(float64)
		return a.clickAt(int(x), int(y))

	case "type_text":
		text, _ := args["text"].(string)
		if text == "" {
			return "错误：text 参数为空"
		}
		return a.typeText(text)

	case "press_key":
		key, _ := args["key"].(string)
		if key == "" {
			return "错误：key 参数为空"
		}
		return a.pressKey(key)

	case "generate_image":
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return "错误：prompt 参数为空"
		}
		filename, _ := args["filename"].(string)
		return a.generateImage(prompt, filename)

	default:
		return "未知工具：" + name
	}
}

// takeScreenshot 截图并返回 base64
func (a *GuiApp) takeScreenshot() string {
	// 使用 PowerShell 截图
	psScript := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen
$bitmap = New-Object System.Drawing.Bitmap($screen.Bounds.Width, $screen.Bounds.Height)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Bounds.Location, [System.Drawing.Point]::Empty, $screen.Bounds.Size)
$ms = New-Object System.IO.MemoryStream
$bitmap.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
$bytes = $ms.ToArray()
$base64 = [Convert]::ToBase64String($bytes)
Write-Output $base64
`
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "截图失败：" + err.Error() + "\n" + stderr.String()
	}
	
	base64Data := strings.TrimSpace(stdout.String())
	if base64Data == "" {
		return "截图失败：返回数据为空"
	}
	
	return "截图成功（base64 长度：" + fmt.Sprintf("%d", len(base64Data)) + "）\n\ndata:image/png;base64," + base64Data
}

// clickAt 在指定坐标点击
func (a *GuiApp) clickAt(x, y int) string {
	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)
Start-Sleep -Milliseconds 100
[System.Windows.Forms.Control]::MouseButtons
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")]
    public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, IntPtr dwExtraInfo);
    public const uint MOUSEEVENTF_LEFTDOWN = 0x02;
    public const uint MOUSEEVENTF_LEFTUP = 0x04;
}
"@
[Mouse]::mouse_event([Mouse]::MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [IntPtr]::Zero)
[Mouse]::mouse_event([Mouse]::MOUSEEVENTF_LEFTUP, 0, 0, 0, [IntPtr]::Zero)
`, x, y)
	
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "点击失败：" + err.Error()
	}
	
	return fmt.Sprintf("已在坐标 (%d, %d) 点击", x, y)
}

// typeText 输入文本
func (a *GuiApp) typeText(text string) string {
	// 使用 PowerShell 的 SendKeys
	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Start-Sleep -Milliseconds 500
[System.Windows.Forms.SendKeys]::SendWait(%q)
`, text)
	
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "输入失败：" + err.Error()
	}
	
	return "已输入文本：" + text
}

// pressKey 按键
func (a *GuiApp) pressKey(key string) string {
	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Start-Sleep -Milliseconds 200
[System.Windows.Forms.SendKeys]::SendWait(%q)
`, key)
	
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "按键失败：" + err.Error()
	}
	
	return "已按键：" + key
}

// generateImage 使用 Pollinations.ai 免费 API 生成图片
func (a *GuiApp) generateImage(prompt string, filename string) string {
	// URL 编码 prompt
	encodedPrompt := url.QueryEscape(prompt)
	
	// Pollinations.ai API URL（免费，无需 API Key）
	apiURL := fmt.Sprintf("https://image.pollinations.ai/prompt/%s?width=1024&height=1024&nologo=true&model=flux", encodedPrompt)
	
	// 生成文件名
	if filename == "" {
		filename = fmt.Sprintf("generated_%d.png", time.Now().Unix())
	}
	if !strings.HasSuffix(filename, ".png") && !strings.HasSuffix(filename, ".jpg") {
		filename += ".png"
	}
	
	// 确保目录存在
	outputPath := filepath.Join(a.workDir, filename)
	dir := filepath.Dir(outputPath)
	os.MkdirAll(dir, 0755)
	
	// 下载图片
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "图片生成失败（网络错误）：" + err.Error()
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("图片生成失败（HTTP %d）", resp.StatusCode)
	}
	
	// 保存到文件
	file, err := os.Create(outputPath)
	if err != nil {
		return "保存失败：" + err.Error()
	}
	defer file.Close()
	
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "保存失败：" + err.Error()
	}
	
	fileInfo, _ := file.Stat()
	size := fileInfo.Size()
	
	return fmt.Sprintf("图片已生成：%s（%s）", outputPath, formatSize(size))
}

// execShell 执行 shell 命令
func (a *GuiApp) execShell(command string) string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	cmd.Dir = a.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := stdout.String()
	if stderr.Len() > 0 {
		result += "\n[STDERR] " + stderr.String()
	}
	if err != nil {
		result += fmt.Sprintf("\n[EXIT] %v", err)
	}
	// 限制输出大小
	if len(result) > 30000 {
		result = result[:30000] + "\n...（输出过长，已截断）"
	}
	return result
}

// formatSize 格式化文件大小
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

// extractSkillWorkflow 从技能内容中提取核心工作流步骤（用于作为 user 消息注入）
func extractSkillWorkflow(skillContent string) string {
	// 按优先级查找工作流相关的段落标题
	keywords := []string{
		"## 工作流程", "## 核心原则", "## 处理流程", "## Steps",
		"## 三步走", "## 输出格式", "## 按模板输出", "## 输入处理",
		"三步走", "工作流程",
	}

	var workflowParts []string
	content := skillContent

	for _, kw := range keywords {
		idx := 0
		for {
			pos := strings.Index(content[idx:], kw)
			if pos == -1 {
				break
			}
			start := idx + pos
			// 从标题后截取内容，直到下一个 ## 标题或 800 字符
			sectionStart := start
			nextH2 := strings.Index(content[sectionStart+2:], "\n## ")
			end := len(content)
			if nextH2 != -1 && nextH2+sectionStart+2 < end {
				end = nextH2 + sectionStart + 2
			}
			if end-sectionStart > 800 {
				end = sectionStart + 800
			}
			part := strings.TrimSpace(content[sectionStart:end])
			if len(part) > 20 {
				workflowParts = append(workflowParts, part)
			}
			idx = start + len(kw)
			if len(workflowParts) >= 3 {
				break
			}
		}
		if len(workflowParts) >= 3 {
			break
		}
	}

	// 如果没找到结构化段落，取前 600 字符
	if len(workflowParts) == 0 {
		if len(content) > 600 {
			return content[:600]
		}
		return content
	}

	result := strings.Join(workflowParts, "\n\n")
	if len(result) > 1500 {
		result = result[:1500]
	}
	return result
}

// callDeepSeekAPI 调用 DeepSeek API（支持 tool calling 循环）
func (a *GuiApp) callDeepSeekAPI(userMessage string, skillContent string, imageDatas []string) (string, error) {
	// 创建可取消的 context
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	a.mu.Lock()
	a.currentCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.currentCancel = nil
		a.mu.Unlock()
	}()

	// 构建系统提示词
	systemPrompt := `你是太极 Code AI 助手。【核心规则】当用户激活技能时，你必须先根据技能工作流向用户询问具体的输出要求（如风格、格式、长度、重点等），确认用户需求后再严格按工作流执行。禁止跳过询问直接执行，禁止提供选项列表让用户选择，禁止仅做文本分析而不执行。
你可以使用以下工具：
- shell_exec：执行本地 shell 命令
- read_file：读取文件内容
- write_file：写入文件
- list_files：列出目录文件
- generate_image：根据文字描述生成 AI 图片

【工具调用规则】
- generate_image 仅在用户明确要求"生成图片"、"画图"、"画一张图"时才调用。如果用户只是描述场景、写文案、写提示词，禁止调用此工具，直接用文字回复即可。
- 不要将用户的创意写作、场景描述、分镜脚本等内容当作画图请求。

当需要执行代码或操作文件时，请直接使用工具执行，而不是仅告诉用户如何操作。
工作目录：` + a.workDir

	// 构建消息历史
	var messages []ChatMessage

	// 系统提示词
	messages = append(messages, ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	a.mu.Lock()
	for _, msg := range a.currentTask.Messages {
		if msg.Type == "user" {
			messages = append(messages, ChatMessage{Role: "user", Content: msg.Content})
		} else if msg.Type == "assistant" {
			messages = append(messages, ChatMessage{Role: "assistant", Content: msg.Content})
		}
	}
	a.mu.Unlock()

	// 图片处理：DeepSeek API 暂不支持 image_url 多模态格式
	// 如果有图片，在文本消息前添加图片数量提示
	if len(imageDatas) > 0 && len(messages) > 0 {
		lastMsg := &messages[len(messages)-1]
		if lastMsg.Role == "user" {
			originalText := lastMsg.Content.(string)
			lastMsg.Content = fmt.Sprintf("[用户上传了 %d 张图片]\n%s", len(imageDatas), originalText)
		}
	}

	// 技能注入：用 system 消息注入工作流（比 user 消息更有约束力）
	if skillContent != "" {
		workflow := extractSkillWorkflow(skillContent)
		outputFormat := ""
		if idx := strings.Index(skillContent, "按模板输出"); idx >= 0 {
			end := strings.Index(skillContent[idx:], "\n## ")
			if end < 0 {
				end = len(skillContent) - idx
			}
			outputFormat = skillContent[idx : idx+end]
			if len(outputFormat) > 600 {
				outputFormat = outputFormat[:600]
			}
		}
		
		skillSystemMsg := "【当前激活技能·强制执行】\n\n"
		skillSystemMsg += "你必须按以下工作流执行，禁止询问用户、禁止提供选项、禁止仅做分析。\n\n"
		skillSystemMsg += "── 工作流 ──\n" + workflow + "\n\n"
		if outputFormat != "" {
			skillSystemMsg += "── 输出格式 ──\n" + outputFormat + "\n\n"
		}
		skillSystemMsg += "立即按以上流程处理用户任务，直接输出完整结果。"
		
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: skillSystemMsg,
		})
	}

	tools := getToolDefinitions()
	client := &http.Client{Timeout: 300 * time.Second}

	// Tool calling 循环（不限制轮次）
	for round := 0; ; round++ {
		// 创建请求
		reqBody := ChatRequest{
			Model:    a.model,
			Messages: messages,
			Tools:    tools,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", fmt.Errorf("序列化请求失败：%v", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", fmt.Errorf("创建请求失败：%v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("请求失败：%v", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("读取响应失败：%v", err)
		}

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("API 错误 (%d): %s", resp.StatusCode, string(body))
		}

		var chatResp ChatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return "", fmt.Errorf("解析响应失败：%v", err)
		}

		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("API 返回空响应")
		}

		choice := chatResp.Choices[0]

		// 检查是否有工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 添加助手消息（含 tool_calls）
			messages = append(messages, ChatMessage{
				Role:      "assistant",
				Content:   choice.Message.Content,
				ToolCalls: choice.Message.ToolCalls,
			})

			// 在前端显示工具调用
			for _, tc := range choice.Message.ToolCalls {
				msg := Message{
					Type:     "tool_call",
					Content:  tc.Function.Arguments,
					Time:     time.Now().Format("15:04"),
					ToolName: tc.Function.Name,
					ToolArgs: tc.Function.Arguments,
				}
				a.mu.Lock()
				a.currentTask.Messages = append(a.currentTask.Messages, msg)
				a.mu.Unlock()
				wailsruntime.EventsEmit(a.ctx, "message_update", msg)
			}

			// 执行每个工具并追加结果
			for _, tc := range choice.Message.ToolCalls {
				result := a.executeTool(tc.Function.Name, tc.Function.Arguments)

				msg := Message{
					Type:     "tool_result",
					Content:  result,
					Time:     time.Now().Format("15:04"),
					ToolName: tc.Function.Name,
				}
				a.mu.Lock()
				a.currentTask.Messages = append(a.currentTask.Messages, msg)
				a.mu.Unlock()
				wailsruntime.EventsEmit(a.ctx, "message_update", msg)

				// 添加工具结果到消息历史
				messages = append(messages, ChatMessage{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
			}

			// 继续循环，让模型处理工具结果
			continue
		}

		// 没有工具调用，返回最终文本响应
		return choice.Message.Content, nil
	}
}

// GetTaskDetail 获取任务详情
func (a *GuiApp) GetTaskDetail(taskID int) *Task {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, task := range a.tasks {
		if task.ID == taskID {
			return task
		}
	}
	return nil
}

// GetTodos 获取待办事项
func (a *GuiApp) GetTodos() []TodoItem {
	return []TodoItem{}
}

// GetWorkingFiles 获取工作文件列表
func (a *GuiApp) GetWorkingFiles() []FileInfo {
	if a.workDir == "" {
		return []FileInfo{}
	}

	entries, err := os.ReadDir(a.workDir)
	if err != nil {
		return []FileInfo{}
	}

	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if isCodeFile(name) {
			files = append(files, FileInfo{
				Name: name,
				Icon: getFileIcon(ext),
			})
		}
	}
	return files
}

// getFileIcon 根据扩展名返回图标
func getFileIcon(ext string) string {
	switch ext {
	case ".go":
		return "📄"
	case ".js", ".ts":
		return "📄"
	case ".py":
		return "📄"
	case ".md":
		return "📝"
	case ".json":
		return "📋"
	case ".yaml", ".yml":
		return "⚙️"
	default:
		return "📄"
	}
}

// GetConfig 获取配置信息
func (a *GuiApp) GetConfig() *Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &Config{
		APIKey:  a.apiKey,
		Model:   a.model,
		BaseURL: a.baseURL,
	}
}

// SetConfig 设置配置
func (a *GuiApp) SetConfig(cfg *Config) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if cfg.APIKey != "" {
		a.apiKey = cfg.APIKey
	}
	if cfg.Model != "" {
		a.model = cfg.Model
	}
	if cfg.BaseURL != "" {
		a.baseURL = cfg.BaseURL
	}
	
	return "配置已保存"
}

// SetWorkDir 设置工作目录
func (a *GuiApp) SetWorkDir(dir string) string {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "目录不存在：" + dir
	}
	a.mu.Lock()
	a.workDir = dir
	a.mu.Unlock()
	return "工作目录已设置为：" + dir
}

// 辅助函数
func isCodeFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	codeExts := []string{".go", ".js", ".ts", ".py", ".java", ".c", ".cpp", ".h", ".rs", ".rb", ".php", ".swift", ".kt", ".md", ".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".sql", ".sh"}
	for _, e := range codeExts {
		if ext == e {
			return true
		}
	}
	return false
}

// readFilesContent 读取多个文件的内容
func (a *GuiApp) readFilesContent(files []string) string {
	var sb strings.Builder
	for _, f := range files {
		content := a.readFileContent(f)
		if content != "" {
			sb.WriteString(fmt.Sprintf("\n=== 文件：%s ===\n%s\n", filepath.Base(f), content))
		}
	}
	return sb.String()
}

// readFileContent 根据文件类型读取内容
func (a *GuiApp) readFileContent(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	// 文本类文件直接读取
	textExts := map[string]bool{
		".txt": true, ".md": true, ".csv": true, ".json": true,
		".xml": true, ".yaml": true, ".yml": true, ".toml": true,
		".html": true, ".htm": true, ".css": true, ".js": true,
		".ts": true, ".jsx": true, ".tsx": true, ".go": true,
		".py": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".rs": true, ".rb": true, ".php": true,
		".swift": true, ".kt": true, ".scala": true, ".r": true,
		".sql": true, ".sh": true, ".bash": true, ".bat": true,
		".ps1": true, ".lua": true, ".pl": true, ".ini": true,
		".cfg": true, ".conf": true, ".log": true, ".env": true,
		".dockerfile": true, ".makefile": true, ".cmake": true,
		".gradle": true, ".properties": true, ".proto": true,
		".graphql": true, ".vue": true, ".svelte": true,
	}
	if textExts[ext] {
		return a.readTextFile(filePath)
	}

	// PDF 文件
	if ext == ".pdf" {
		return a.readPDFFile(filePath)
	}

	// Office 文件 (docx, xlsx, pptx)
	if ext == ".docx" || ext == ".xlsx" || ext == ".pptx" {
		return a.readOfficeFile(filePath)
	}

	// 旧版 Office 文件
	if ext == ".doc" || ext == ".xls" || ext == ".ppt" {
		return fmt.Sprintf("[旧版 Office 格式 %s，建议转换为 .docx/.xlsx/.pptx 后重新导入]", filepath.Base(filePath))
	}

	// 图片文件
	imgExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".bmp": true, ".svg": true, ".webp": true, ".ico": true,
	}
	if imgExts[ext] {
		return fmt.Sprintf("[图片文件：%s，当前版本暂不支持图片内容提取]", filepath.Base(filePath))
	}

	// 其他文件尝试作为文本读取
	return a.readTextFile(filePath)
}

// readTextFile 读取文本文件
func (a *GuiApp) readTextFile(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("[无法读取文件：%v]", err)
	}
	content := string(data)
	// 限制大小，最大 50KB
	if len(content) > 50000 {
		content = content[:50000] + "\n...（内容过长，已截断）"
	}
	return content
}

// readPDFFile 读取 PDF 文件文本内容
func (a *GuiApp) readPDFFile(filePath string) string {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return fmt.Sprintf("[无法解析 PDF：%v]", err)
	}
	defer f.Close()

	var sb strings.Builder
	totalPage := r.NumPage()
	for pageIndex := 1; pageIndex <= totalPage; pageIndex++ {
		p := r.Page(pageIndex)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	result := sb.String()
	if len(result) > 50000 {
		result = result[:50000] + "\n...（内容过长，已截断）"
	}
	if result == "" {
		return "[PDF 文件中未提取到文本内容（可能是扫描版 PDF）]"
	}
	return result
}

// readOfficeFile 读取 Office 文件 (docx/xlsx/pptx)
func (a *GuiApp) readOfficeFile(filePath string) string {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Sprintf("[无法打开 Office 文件：%v]", err)
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		// 只读取 document.xml / sheet / slide 中的文本
		if strings.Contains(f.Name, "word/document.xml") ||
			strings.Contains(f.Name, "xl/sharedStrings.xml") ||
			strings.Contains(f.Name, "ppt/slides/slide") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			// 简单提取 XML 中的文本内容
			text := extractXMLText(string(data))
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}

	result := strings.TrimSpace(sb.String())
	if len(result) > 50000 {
		result = result[:50000] + "\n...（内容过长，已截断）"
	}
	if result == "" {
		return "[Office 文件中未提取到文本内容]"
	}
	return result
}

// extractXMLText 从 XML 中提取文本内容
func extractXMLText(xml string) string {
	var sb strings.Builder
	// 提取 <w:t> 标签内容 (Word)
	for _, tag := range []string{"w:t", "t", "a:t"} {
		openTag := "<" + tag
		closeTag := "</" + tag + ">"
		idx := 0
		for {
			start := strings.Index(xml[idx:], openTag)
			if start == -1 {
				break
			}
			start += idx
			// 找到 > 结束
			end := strings.Index(xml[start:], ">")
			if end == -1 {
				break
			}
			end += start + 1
			closeIdx := strings.Index(xml[end:], closeTag)
			if closeIdx == -1 {
				break
			}
			text := xml[end : end+closeIdx]
			// 清理 XML 实体
			text = strings.ReplaceAll(text, "&amp;", "&")
			text = strings.ReplaceAll(text, "&lt;", "<")
			text = strings.ReplaceAll(text, "&gt;", ">")
			text = strings.ReplaceAll(text, "&quot;", "\"")
			text = strings.ReplaceAll(text, "&apos;", "'")
			sb.WriteString(text)
			idx = end + closeIdx + len(closeTag)
		}
	}
	return sb.String()
}

// ========== 频道管理 ==========

func (a *GuiApp) GetChannels() []*Channel {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.channels
}

func (a *GuiApp) AddChannel(input *ChannelInput) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := &Channel{
		ID:       a.nextChannelID,
		Platform: input.Platform,
		Name:     input.Name,
		Token:    input.Token,
		Desc:     input.Desc,
	}
	a.nextChannelID++
	a.channels = append(a.channels, ch)
	return "频道已添加"
}

func (a *GuiApp) ToggleChannel(id int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.channels {
		if ch.ID == id {
			ch.Connected = !ch.Connected
			return "ok"
		}
	}
	return "频道不存在"
}

func (a *GuiApp) DeleteChannel(id int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, ch := range a.channels {
		if ch.ID == id {
			a.channels = append(a.channels[:i], a.channels[i+1:]...)
			return "已删除"
		}
	}
	return "频道不存在"
}

// ========== 技能管理 ==========

func (a *GuiApp) GetSkills() []*Skill {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.skills
}

func (a *GuiApp) AddSkill(input *SkillInput) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var fileName, filePath string
	if input.FileContent != "" && input.FileName != "" {
		fileName = input.FileName
		// 确保目录名唯一：技能名_文件名
		safeName := strings.ReplaceAll(input.Name, " ", "_")
		saveName := safeName + "_" + fileName
		filePath = filepath.Join(a.skillsDir, saveName)

		// 解码 base64 内容并保存
		decoded, err := base64.StdEncoding.DecodeString(input.FileContent)
		if err != nil {
			// 如果不是 base64，直接保存原文
			decoded = []byte(input.FileContent)
		}
		if err := os.WriteFile(filePath, decoded, 0644); err != nil {
			return "文件保存失败：" + err.Error()
		}
	}

	sk := &Skill{
		ID:       a.nextSkillID,
		Name:     input.Name,
		Desc:     input.Desc,
		Keywords: input.Keywords,
		Enabled:  true,
		FileName: fileName,
		FilePath: filePath,
	}
	a.nextSkillID++
	a.skills = append(a.skills, sk)
	return "技能已安装"
}

func (a *GuiApp) ToggleSkill(id int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, sk := range a.skills {
		if sk.ID == id {
			sk.Enabled = !sk.Enabled
			return "ok"
		}
	}
	return "技能不存在"
}

// ========== 定时任务管理 ==========

func (a *GuiApp) GetCrons() []*CronJob {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cr := range a.crons {
		cr.NextRun = calcNextRun(cr.Expr)
	}
	return a.crons
}

func (a *GuiApp) AddCron(input *CronInput) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	cr := &CronJob{
		ID:      a.nextCronID,
		Name:    input.Name,
		Expr:    input.Expr,
		Desc:    input.Desc,
		Enabled: input.Enabled,
	}
	a.nextCronID++
	a.crons = append(a.crons, cr)
	return "定时任务已创建"
}

func (a *GuiApp) ToggleCron(id int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cr := range a.crons {
		if cr.ID == id {
			cr.Enabled = !cr.Enabled
			return "ok"
		}
	}
	return "任务不存在"
}

func (a *GuiApp) DeleteCron(id int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, cr := range a.crons {
		if cr.ID == id {
			a.crons = append(a.crons[:i], a.crons[i+1:]...)
			return "已删除"
		}
	}
	return "任务不存在"
}

// calcNextRun 根据 cron 表达式计算下次执行时间（简化版）
func calcNextRun(expr string) string {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return "格式错误"
	}
	minute := parts[0]
	hour := parts[1]

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for i := 0; i < 2880; i++ {
		next = next.Add(time.Minute)
		h := next.Hour()
		m := next.Minute()

		hMatch := hour == "*" || fmt.Sprintf("%d", h) == hour
		mMatch := minute == "*" || fmt.Sprintf("%d", m) == minute

		if hMatch && mMatch {
			return next.Format("2006-01-02 15:04")
		}
	}
	return "无法计算"
}
