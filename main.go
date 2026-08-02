package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"taiji-code/internal/agent"
	"taiji-code/internal/config"
	"taiji-code/internal/llm"
	"taiji-code/internal/mcp"
	"taiji-code/internal/memory"
	"taiji-code/internal/permission"
	"taiji-code/internal/tools"
	"taiji-code/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const Version = "0.3.0"

func main() {
	cfg := config.Load()

	// ── 命令行参数（在API Key检查之前处理）──
	// 预扫描全局标志，确保无论参数顺序如何都能正确解析
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--output-format":
			if i+1 < len(os.Args) {
				cfg.OutputFormat = os.Args[i+1]
			}
		case "--permission-mode":
			if i+1 < len(os.Args) {
				cfg.PermissionMode = config.PermissionMode(os.Args[i+1])
			}
		}
	}

	resumeMode := false
	continueMode := false
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--version", "-v":
			fmt.Printf("太极 Code v%s\n", Version)
			os.Exit(0)
		case "--config":
			运行配置(cfg)
			os.Exit(0)
		case "--help", "-h":
			打印帮助()
			os.Exit(0)
		case "--resume":
			resumeMode = true
		case "--continue":
			continueMode = true
		case "--prompt", "-p":
			if i+1 < len(os.Args) {
				i++
				// 单次提问模式
				运行单次提问(cfg, os.Args[i])
				return
			}
		}
	}

	// ── 检测管道模式 ──
	管道输入 := ""
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
			os.Exit(1)
		}
		管道输入 = strings.TrimSpace(string(data))
	}

	// ── 检查API Key ──
	if !cfg.HasAPIKey() {
		fmt.Println("错误: 未配置 DeepSeek API Key")
		fmt.Println()
		fmt.Println("请通过以下方式之一配置:")
		fmt.Println("  1. 设置环境变量: export DEEPSEEK_API_KEY=你的密钥")
		fmt.Println("  2. 运行配置命令: taiji-code --config")
		fmt.Println()
		fmt.Println("获取 API Key: https://platform.deepseek.com/")
		os.Exit(1)
	}

	工作目录, _ := os.Getwd()

	// ── 初始化组件 ──
	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	client.SetTemperature(cfg.Temperature)
	procMgr := tools.NewProcessManager()
	todoStore := tools.NewTodoStore()
	registry := 注册工具(工作目录, procMgr, todoStore)
	perm := permission.NewHandler()
	mem := memory.New(工作目录, cfg.MemoryFile)
	ag := agent.New(client, registry, perm, mem, 工作目录, cfg.MaxTokens, cfg.MaxToolSteps)

	// 设置权限模式
	switch cfg.PermissionMode {
	case config.PermModeBypass:
		perm.AutoApproveAll = true
	case config.PermModePlanOnly:
		ag.SetMode(agent.ModePlan)
	case config.PermModeAutoEdit:
		ag.SetMode(agent.ModeAutoEdit)
	}

	// 设置输出风格
	ag.SetOutputStyle(cfg.ResponseStyle)

	// 初始化MCP连接
	mcpClient := mcp.NewClient()
	if len(cfg.McpServers) > 0 {
		ctx := context.Background()
		for _, srv := range cfg.McpServers {
			err := mcpClient.ConnectServer(ctx, mcp.ServerConfig{
				Name:    srv.Name,
				Command: srv.Command,
				Args:    srv.Args,
				Env:     srv.Env,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "MCP服务器 %s 连接失败: %v\n", srv.Name, err)
			}
		}
		defer mcpClient.Close()
	}

	// ── 管道模式 ──
	if 管道输入 != "" {
		运行管道模式(cfg, ag, perm, 管道输入)
		return
	}

	// ── resume/continue模式 ──
	if resumeMode || continueMode {
		if 已恢复, 消息 := ag.AutoResumeSession(); 已恢复 {
			fmt.Fprintf(os.Stderr, "%s\n", 消息)
		} else {
			fmt.Fprintln(os.Stderr, "没有找到可恢复的会话")
			os.Exit(1)
		}
	}

	// ── 交互模式 ──
	运行交互模式(cfg, client, registry, perm, mem, ag, procMgr, todoStore, mcpClient, 工作目录)
}

// ─── 运行模式 ────────────────────────────────────────────────────────────

func 运行管道模式(cfg *config.Config, ag *agent.Agent, perm *permission.Handler, input string) {
	perm.AskFunc = func(toolName, description string) bool {
		return true
	}
	ctx := context.Background()
	resp, err := ag.Chat(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	switch cfg.OutputFormat {
	case "json":
		resultJSON, _ := json.Marshal(resp)
		fmt.Printf(`{"type":"result","result":%s,"cost":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}`+"\n",
			resultJSON, ag.GetCost().InputTokens, ag.GetCost().OutputTokens, ag.GetCost().TotalTokens)
	default:
		fmt.Println(resp)
	}
}

func 运行单次提问(cfg *config.Config, input string) {
	工作目录, _ := os.Getwd()
	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	client.SetTemperature(cfg.Temperature)
	procMgr := tools.NewProcessManager()
	todoStore := tools.NewTodoStore()
	registry := 注册工具(工作目录, procMgr, todoStore)
	perm := permission.NewHandler()
	perm.AskFunc = func(toolName, description string) bool { return true }
	mem := memory.New(工作目录, cfg.MemoryFile)
	ag := agent.New(client, registry, perm, mem, 工作目录, cfg.MaxTokens, cfg.MaxToolSteps)
	ag.SetTodoStore(todoStore)

	ctx := context.Background()
	resp, err := ag.Chat(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 输出格式处理
	switch cfg.OutputFormat {
	case "json":
		fmt.Printf(`{"type":"result","result":%q,"cost":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}`+"\n",
			resp, ag.GetCost().InputTokens, ag.GetCost().OutputTokens, ag.GetCost().TotalTokens)
	default:
		fmt.Println(resp)
	}
}

func 运行交互模式(cfg *config.Config, client *llm.Client, registry *tools.Registry,
	perm *permission.Handler, mem *memory.Memory, ag *agent.Agent,
	procMgr *tools.ProcessManager, todoStore *tools.TodoStore,
	mcpClient *mcp.Client, 工作目录 string) {

	model := ui.NewModel()
	model.SetModel(cfg.Model)
	model.SetVersion(Version)
	model.SetAgentMode(string(ag.GetMode()))

	// 共享todoStore
	ag.SetTodoStore(todoStore)

	// 注册SubAgent工具
	subPool := agent.NewSubAgentPool(client, registry, perm, 工作目录, cfg.MaxTokens)
	registry.Register(&agent.SubAgentTool{Pool: subPool})

	// 注册MCP工具
	mcpCount := mcp.RegisterMcpTools(mcpClient, registry)

	// 显示项目信息
	proj := ag.GetProject()
	if proj != nil && proj.Type != "unknown" {
		model.AddBlock("info", fmt.Sprintf("检测到项目: %s (%s)", proj.Name, proj.Type))
	}

	// 显示MCP连接信息
	serverNames := mcpClient.GetServerNames()
	if len(serverNames) > 0 {
		model.AddBlock("info", fmt.Sprintf("MCP服务器: %s (%d个工具)", strings.Join(serverNames, ", "), mcpCount))
	}

	// 权限处理
	perm.AskFunc = func(toolName, description string) bool {
		return true
	}

	// 先创建Program（用于流式推送）
	p := tea.NewProgram(model, tea.WithAltScreen())

	// 代理运行器（带流式显示）
	model.AgentRunner = func(input string) <-chan ui.AgentResponse {
		ch := make(chan ui.AgentResponse, 1)

		// 同步TUI模式到Agent
		switch model.GetAgentMode() {
		case "plan":
			ag.SetMode(agent.ModePlan)
		case "auto_edit":
			ag.SetMode(agent.ModeAutoEdit)
		default:
			ag.SetMode(agent.ModeNormal)
		}

		ag.SaveSnapshot("用户输入前")

		// 流式显示：OnContent → p.Send(StreamToken)
		ag.OnContent = func(text string) {
			p.Send(ui.StreamToken{Text: text})
		}

		ag.OnToolCall = func(name string, args map[string]interface{}) {
			argsJSON, _ := json.Marshal(args)
			argsStr := string(argsJSON)
			if len(argsStr) > 200 {
				argsStr = argsStr[:200] + "..."
			}
			p.Send(ui.BlockMsg{BlockType: "tool_call", ToolName: name, ToolArgs: argsStr})
			if name == "write_file" || name == "edit_file" {
				ag.GetToolCache().Clear()
			}
		}

		ag.OnToolResult = func(name string, result string, isError bool) {
			blockType := "tool_result"
			if isError {
				blockType = "error"
			}
			display := result
			runes := []rune(display)
			if len(runes) > 600 {
				display = string(runes[:600]) + "\n    ... (已截断)"
			}
			p.Send(ui.BlockMsg{BlockType: blockType, Content: display, ToolName: name})
		}

		ag.OnUsage = func(cost agent.CostTracker) {
			p.Send(ui.BlockMsg{BlockType: "usage", Content: cost.FormatUsage()})
		}

		ag.OnCompact = func() {
			p.Send(ui.BlockMsg{BlockType: "info", Content: "上下文超过阈值，已自动压缩对话历史"})
		}

		go func() {
			ctx, cancel := context.WithCancel(context.Background())
			model.CancelFunc = cancel
			defer cancel()
			_, err := ag.Chat(ctx, input)
			// 结束流式显示
			p.Send(ui.StreamToken{Done: true})
			ch <- ui.AgentResponse{Content: "", Err: err}
		}()

		return ch
	}

	// 命令处理器
	model.CommandHandler = func(cmd string) string {
		return 处理命令(cmd, ag, cfg, mem, client, model, mcpClient, todoStore, perm, 工作目录)
	}

	// 文件路径补全
	model.TabCompleter = func(input string) []string {
		return 补全文件路径(input, 工作目录)
	}

	// 打印横幅
	ui.PrintBanner(Version)
	fmt.Printf("  工作目录: %s\n", 工作目录)
	fmt.Printf("  模型: %s\n", cfg.Model)
	fmt.Printf("  会话: %s\n", ag.GetSessionID())
	fmt.Printf("  模式: %s\n", ag.GetMode())
	if proj != nil && proj.Type != "unknown" {
		fmt.Printf("  项目: %s (%s)\n", proj.Name, proj.Type)
	}
	fmt.Println()

	defer func() {
		_ = ag.SaveSession()
		mcpClient.Close()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// ─── 工具注册 ────────────────────────────────────────────────────────────

func 注册工具(工作目录 string, procMgr *tools.ProcessManager, todoStore *tools.TodoStore) *tools.Registry {
	r := tools.NewRegistry()
	r.Register(&tools.ReadFile{})
	r.Register(&tools.WriteFile{})
	r.Register(&tools.EditFile{})
	r.Register(tools.NewBash(工作目录))
	r.Register(&tools.ListDir{})
	r.Register(&tools.GlobFind{})
	r.Register(&tools.GrepSearch{})
	r.Register(tools.NewGitTool(工作目录))
	r.Register(&tools.NotebookEdit{})
	// 新增工具
	r.Register(&tools.WebFetch{})
	r.Register(&tools.WebSearch{})
	r.Register(&tools.BackgroundBash{Manager: procMgr, WorkDir: 工作目录})
	r.Register(&tools.BashOutput{Manager: procMgr})
	r.Register(&tools.BashKill{Manager: procMgr})
	r.Register(&tools.TodoWrite{Store: todoStore})
	return r
}

// ─── 斜杠命令 ────────────────────────────────────────────────────────────

func 处理命令(cmd string, ag *agent.Agent, cfg *config.Config, mem *memory.Memory,
	client *llm.Client, model *ui.Model, mcpClient *mcp.Client, todoStore *tools.TodoStore, perm *permission.Handler, 工作目录 string) string {

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	switch command {
	case "/help", "/h", "/帮助":
		return `可用命令:
  /help            显示帮助
  /clear           清空对话历史
  /compact         压缩对话上下文
  /model [名称]    查看/切换模型
  /usage           显示Token用量和费用
  /context         显示当前上下文大小
  /memory          查看项目记忆(TAIJI.md)
  /tools           列出可用工具
  /sessions        列出已保存的会话
  /save            保存当前会话
  /load [编号]     加载指定会话
  /checkpoint      创建检查点(git stash)
  /rollback        回滚到最近检查点
  /project         显示项目信息
  /cache           显示工具缓存状态
  /config          显示当前配置
  /status          显示详细状态
  /doctor          诊断检查
  /export          导出对话为Markdown
  /rewind [编号]   回退到快照
  /review          代码审查
  /mode [模式]     切换模式(normal/plan/auto_edit)
  /style [风格]    切换输出风格(concise/normal/detailed)
  /vim             切换Vim键绑定
  /permissions     查看/切换权限模式
  /hooks           查看已注册Hooks
  /skills          查看已加载技能
  /todos           查看任务列表
  /mcp             查看MCP服务器状态
  /init            初始化项目(TAIJI.md+.taiji/)
  /quit, /q        退出

快捷键:
  Shift+Tab        循环切换模式(标准→规划→自动)
  Ctrl+G           切换多行输入模式
  Ctrl+C           中断当前操作 / 退出
  Ctrl+L           清屏
  Tab              文件路径/命令补全
  PgUp/PgDn        上下滚动历史
  ↑/↓              浏览输入历史`

	case "/clear", "/清空":
		ag.ClearHistory()
		return "对话历史已清空"

	case "/compact", "/压缩":
		return ag.Compact()

	case "/model", "/模型":
		if len(args) > 0 {
			newModel := args[0]
			cfg.Model = newModel
			client.SetModel(newModel)
			model.SetModel(newModel)
			return fmt.Sprintf("已切换模型: %s", newModel)
		}
		return fmt.Sprintf("当前模型: %s\n可用模型: deepseek-chat, deepseek-reasoner", cfg.Model)

	case "/usage", "/用量":
		return ag.GetCost().FormatUsage()

	case "/context", "/上下文":
		tokens := ag.EstimateContextTokens()
		msgCount := len(ag.GetMessages())
		return fmt.Sprintf("上下文: ~%d tokens (%d 条消息)", tokens, msgCount)

	case "/memory", "/记忆":
		content, err := mem.Load()
		if err != nil {
			return fmt.Sprintf("读取记忆失败: %v", err)
		}
		if content == "" {
			return "项目记忆为空。在工作目录创建 TAIJI.md 文件来添加项目上下文。"
		}
		if len(content) > 1500 {
			content = content[:1500] + "\n... (已截断)"
		}
		return content

	case "/tools", "/工具":
		reg := 注册工具(".", tools.NewProcessManager(), tools.NewTodoStore())
		toolNames := reg.Names()
		cnNames := make([]string, len(toolNames))
		for i, name := range toolNames {
			cnNames[i] = 翻译工具名(name) + " (" + name + ")"
		}
		return fmt.Sprintf("可用工具 (%d):\n  %s", len(cnNames), strings.Join(cnNames, "\n  "))

	case "/sessions", "/会话":
		sessions, err := ag.ListSessions()
		if err != nil {
			return fmt.Sprintf("列出会话失败: %v", err)
		}
		return agent.FormatSessionList(sessions)

	case "/save", "/保存":
		if err := ag.SaveSession(); err != nil {
			return fmt.Sprintf("保存失败: %v", err)
		}
		return fmt.Sprintf("会话已保存: %s", ag.GetSessionID())

	case "/load", "/加载":
		if len(args) == 0 {
			return "用法: /load <会话编号>"
		}
		if err := ag.LoadSession(args[0]); err != nil {
			return fmt.Sprintf("加载失败: %v", err)
		}
		return fmt.Sprintf("已加载会话: %s (%d条消息)", args[0], len(ag.GetMessages()))

	case "/config", "/配置":
		return fmt.Sprintf("当前配置:\n  接口地址: %s\n  模型: %s\n  最大输出: %d tokens\n  温度: %.1f\n  最大步数: %d\n  输出格式: %s\n  输出风格: %s\n  权限模式: %s",
			cfg.BaseURL, cfg.Model, cfg.MaxTokens, cfg.Temperature, cfg.MaxToolSteps,
			cfg.OutputFormat, cfg.ResponseStyle, cfg.PermissionMode)

	case "/status", "/状态":
		return ag.GetStatus()

	case "/doctor", "/诊断":
		return ag.Doctor()

	case "/export", "/导出":
		return ag.ExportConversation()

	case "/rewind", "/回退":
		if len(args) == 0 {
			return ag.ListSnapshots()
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return "用法: /rewind [快照编号]  (不带参数查看可用快照)"
		}
		result, err := ag.Rewind(idx)
		if err != nil {
			return fmt.Sprintf("回退失败: %v", err)
		}
		return result

	case "/review", "/审查":
		scope := ""
		if len(args) > 0 {
			scope = strings.Join(args, " ")
		}
		return ag.ReviewCode(scope)

	case "/mode", "/模式":
		if len(args) == 0 {
			return fmt.Sprintf("当前模式: %s\n可用模式: normal(标准), plan(规划/只读), auto_edit(自动编辑)", ag.GetMode())
		}
		switch args[0] {
		case "normal", "标准":
			ag.SetMode(agent.ModeNormal)
			model.SetAgentMode("normal")
			return "已切换到标准模式"
		case "plan", "规划":
			ag.SetMode(agent.ModePlan)
			model.SetAgentMode("plan")
			return "已切换到规划模式(只读)"
		case "auto_edit", "自动":
			ag.SetMode(agent.ModeAutoEdit)
			model.SetAgentMode("auto_edit")
			return "已切换到自动编辑模式"
		default:
			return "未知模式: " + args[0] + " (可用: normal, plan, auto_edit)"
		}

	case "/style", "/风格":
		if len(args) == 0 {
			return fmt.Sprintf("当前风格: %s\n可用风格: concise(简洁), normal(标准), detailed(详细)", ag.GetOutputStyle())
		}
		ag.SetOutputStyle(args[0])
		return fmt.Sprintf("已切换输出风格: %s", args[0])

	case "/vim":
		model.SetVimMode(!model.IsVimMode())
		if model.IsVimMode() {
			return "Vim键绑定已开启 (j/k滚动, 空输入时生效)"
		}
		return "Vim键绑定已关闭"

	case "/permissions", "/权限":
		if len(args) == 0 {
			return fmt.Sprintf("当前权限模式: %s\n可用模式: default(逐条确认), auto_edit(自动编辑), plan_only(仅规划), bypass(跳过所有权限)", cfg.PermissionMode)
		}
		switch args[0] {
		case "default":
			cfg.PermissionMode = config.PermModeDefault
			ag.SetMode(agent.ModeNormal)
			model.SetAgentMode("normal")
			perm.AutoApproveAll = false
		case "auto_edit":
			cfg.PermissionMode = config.PermModeAutoEdit
			ag.SetMode(agent.ModeAutoEdit)
			model.SetAgentMode("auto_edit")
			perm.AutoApproveAll = false
		case "plan_only":
			cfg.PermissionMode = config.PermModePlanOnly
			ag.SetMode(agent.ModePlan)
			model.SetAgentMode("plan")
			perm.AutoApproveAll = false
		case "bypass":
			cfg.PermissionMode = config.PermModeBypass
			ag.SetMode(agent.ModeNormal)
			model.SetAgentMode("normal")
			perm.AutoApproveAll = true
		default:
			return "未知权限模式: " + args[0]
		}
		return fmt.Sprintf("权限模式已切换: %s", args[0])

	case "/hooks":
		return ag.GetHookManager().ListHooks()

	case "/skills", "/技能":
		return ag.GetSkillManager().ListSkills()

	case "/todos", "/任务":
		return todoStore.FormatTasks()

	case "/mcp":
		names := mcpClient.GetServerNames()
		if len(names) == 0 {
			return "未连接任何MCP服务器\n在 ~/.taiji-code/config.json 的 mcp_servers 中配置"
		}
		toolsList := mcpClient.GetTools()
		return fmt.Sprintf("MCP服务器 (%d):\n  %s\nMCP工具 (%d)",
			len(names), strings.Join(names, "\n  "), len(toolsList))

	case "/checkpoint", "/检查点":
		label := "手动"
		if len(args) > 0 {
			label = strings.Join(args, " ")
		}
		result, err := ag.CreateCheckpoint(label)
		if err != nil {
			return fmt.Sprintf("检查点创建失败: %v", err)
		}
		return "检查点已创建: " + result

	case "/rollback", "/回滚":
		result, err := ag.RollbackCheckpoint()
		if err != nil {
			return fmt.Sprintf("回滚失败: %v", err)
		}
		return "已回滚到最近检查点: " + result

	case "/project", "/项目":
		proj := ag.GetProject()
		if proj == nil || proj.Type == "unknown" {
			return "未检测到已知项目类型"
		}
		info := fmt.Sprintf("项目: %s\n类型: %s\nGit: %v\n记忆文件: %v",
			proj.Name, proj.Type, proj.HasGit, proj.HasMemory)
		if proj.BuildCommand != "" {
			info += fmt.Sprintf("\n构建: %s\n测试: %s\n运行: %s",
				proj.BuildCommand, proj.TestCommand, proj.RunCommand)
		}
		return info

	case "/cache", "/缓存":
		cache := ag.GetToolCache()
		return fmt.Sprintf("工具缓存: %d 条 (只读操作缓存，30秒过期)", cache.Size())

	case "/init", "/初始化":
		return 初始化项目(工作目录, mem)

	case "/quit", "/q", "/exit", "/退出":
		_ = ag.SaveSession()
		os.Exit(0)
		return ""

	default:
		return fmt.Sprintf("未知命令: %s (输入 /help 查看帮助)", command)
	}
}

// ─── 辅助函数 ────────────────────────────────────────────────────────────

func 翻译工具名(name string) string {
	m := map[string]string{
		"read_file":     "读取文件",
		"write_file":    "写入文件",
		"edit_file":     "编辑文件",
		"bash":          "执行命令",
		"bash_bg":       "后台执行",
		"bash_output":   "查看输出",
		"bash_kill":     "终止进程",
		"list_dir":      "列出目录",
		"glob_find":     "查找文件",
		"grep_search":   "搜索内容",
		"git":           "Git操作",
		"web_fetch":     "抓取网页",
		"web_search":    "搜索网页",
		"todo_write":    "任务管理",
		"sub_agent":     "子代理",
		"notebook_edit": "编辑Notebook",
	}
	if cn, ok := m[name]; ok {
		return cn
	}
	return name
}

func 运行配置(cfg *config.Config) {
	fmt.Println("太极 Code 配置")
	fmt.Println("═══════════════════════════════")
	fmt.Println()

	if cfg.HasAPIKey() {
		key := cfg.APIKey
		if len(key) > 12 {
			fmt.Printf("API Key: %s...%s (已配置)\n", key[:8], key[len(key)-4:])
		} else {
			fmt.Println("API Key: (已配置)")
		}
	} else {
		fmt.Println("API Key: 未配置")
	}

	fmt.Printf("接口地址: %s\n", cfg.BaseURL)
	fmt.Printf("模型: %s\n", cfg.Model)
	fmt.Printf("最大输出: %d tokens\n", cfg.MaxTokens)
	fmt.Printf("温度: %.1f\n", cfg.Temperature)
	fmt.Printf("输出格式: %s\n", cfg.OutputFormat)
	fmt.Printf("输出风格: %s\n", cfg.ResponseStyle)
	fmt.Printf("权限模式: %s\n", cfg.PermissionMode)
	fmt.Println()
	fmt.Println("配置方法:")
	fmt.Println("  1. 设置环境变量 DEEPSEEK_API_KEY")
	fmt.Println("  2. 编辑 ~/.taiji-code/config.json")
}

func 打印帮助() {
	fmt.Printf(`太极 Code v%s — AI编程助手

用法:
  taiji-code                     启动交互式会话
  taiji-code -p "问题"           单次提问模式
  taiji-code --resume            恢复上次会话
  taiji-code --continue          继续上次会话
  echo "问题" | taiji-code       管道模式
  taiji-code --config            查看/配置API密钥
  taiji-code --version           显示版本
  taiji-code --help              显示帮助

命令行参数:
  --prompt, -p "问题"            单次提问
  --resume                       恢复上次会话
  --continue                     继续上次会话
  --output-format [格式]         输出格式(text/json/stream-json)
  --permission-mode [模式]       权限模式(default/auto_edit/plan_only/bypass)

环境变量:
  DEEPSEEK_API_KEY               DeepSeek API密钥 (必需)

配置文件:
  ~/.taiji-code/config.json      主配置文件

项目文件:
  TAIJI.md                       项目记忆 (类似CLAUDE.md)
  .taiji/sessions/               会话历史
  .taiji/skills/                 技能目录
  .taiji/hooks.json              Hooks配置
`, Version)
}

// 初始化项目 创建TAIJI.md和.taiji/目录结构
func 初始化项目(工作目录 string, mem *memory.Memory) string {
	var sb strings.Builder

	// 创建.taiji目录
	dirs := []string{
		filepath.Join(工作目录, ".taiji"),
		filepath.Join(工作目录, ".taiji", "sessions"),
		filepath.Join(工作目录, ".taiji", "skills"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			sb.WriteString(fmt.Sprintf("  ✗ 创建目录失败 %s: %v\n", dir, err))
		} else {
			sb.WriteString(fmt.Sprintf("  ✓ %s/\n", dir))
		}
	}

	// 创建TAIJI.md
	if err := mem.CreateDefault(); err != nil {
		sb.WriteString(fmt.Sprintf("  ✗ 创建TAIJI.md失败: %v\n", err))
	} else {
		sb.WriteString("  ✓ TAIJI.md (项目记忆)\n")
	}

	// 创建hooks.json模板
	hooksPath := filepath.Join(工作目录, ".taiji", "hooks.json")
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		hooksTemplate := `[]`
		if err := os.WriteFile(hooksPath, []byte(hooksTemplate), 0644); err == nil {
			sb.WriteString("  ✓ .taiji/hooks.json (Hooks配置模板)\n")
		}
	}

	sb.WriteString("\n项目初始化完成！编辑 TAIJI.md 添加项目上下文。")
	return sb.String()
}

// 补全文件路径 Tab补全文件路径
func 补全文件路径(input string, 工作目录 string) []string {
	if input == "" {
		return nil
	}

	// 如果是斜杠命令，补全命令
	if strings.HasPrefix(input, "/") {
		commands := []string{
			"/help", "/clear", "/compact", "/model", "/usage",
			"/context", "/memory", "/tools", "/sessions", "/save",
			"/load", "/config", "/quit", "/checkpoint", "/rollback",
			"/project", "/cache", "/status", "/doctor", "/export",
			"/rewind", "/review", "/mode", "/style", "/vim",
			"/permissions", "/hooks", "/skills", "/todos", "/mcp", "/init",
		}
		var matches []string
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, input) {
				matches = append(matches, cmd)
			}
		}
		return matches
	}

	// 文件路径补全
	dir := filepath.Dir(input)
	prefix := filepath.Base(input)

	searchDir := dir
	if !filepath.IsAbs(searchDir) {
		searchDir = filepath.Join(工作目录, searchDir)
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	var matches []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) {
			fullPath := filepath.Join(filepath.Dir(input), name)
			if entry.IsDir() {
				fullPath += string(filepath.Separator)
			}
			matches = append(matches, fullPath)
		}
	}

	return matches
}
