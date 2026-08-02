package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"taiji-code/types"
)

// MCP (Model Context Protocol) 客户端
// 支持通过stdio与MCP服务器通信，动态加载外部工具

// ServerConfig MCP服务器配置
type ServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env,omitempty"`
}

// Client MCP客户端
type Client struct {
	mu       sync.Mutex
	servers  map[string]*ServerConn
	tools    []types.ToolDefinition // 从所有服务器聚合的工具
}

// ServerConn 单个MCP服务器连接
type ServerConn struct {
	Config  ServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	tools   []types.ToolDefinition
	connMu  sync.Mutex // 保护send/receive的串行化
	reqID   int        // 递增的请求ID
}

// NewClient 创建MCP客户端
func NewClient() *Client {
	return &Client{
		servers: make(map[string]*ServerConn),
	}
}

// ConnectServer 连接到MCP服务器
func (c *Client) ConnectServer(ctx context.Context, cfg ServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建stdin管道失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建stdout管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动MCP服务器 %s 失败: %w", cfg.Name, err)
	}

	conn := &ServerConn{
		Config: cfg,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}

	// 发送initialize请求
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "taiji-code",
				"version": "0.2.0",
			},
		},
	}

	if err := conn.send(initReq); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("初始化MCP服务器失败: %w", err)
	}

	// 读取初始化响应
	if _, err := conn.readResponse(); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("读取初始化响应失败: %w", err)
	}

	// 获取工具列表
	toolsReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	if err := conn.send(toolsReq); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("获取工具列表失败: %w", err)
	}

	resp, err := conn.readResponse()
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("读取工具列表失败: %w", err)
	}

	// 解析工具
	conn.tools = parseTools(resp)

	c.servers[cfg.Name] = conn
	c.tools = append(c.tools, conn.tools...)

	return nil
}

// CallTool 调用MCP工具
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error) {
	c.mu.Lock()
	conn, ok := c.servers[serverName]
	c.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("MCP服务器 %s 未连接", serverName)
	}

	// 使用连接级锁保证send+receive串行化
	conn.connMu.Lock()
	defer conn.connMu.Unlock()

	conn.reqID++
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      conn.reqID,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	if err := conn.send(req); err != nil {
		return "", fmt.Errorf("调用MCP工具失败: %w", err)
	}

	resp, err := conn.readResponse()
	if err != nil {
		return "", fmt.Errorf("读取MCP工具响应失败: %w", err)
	}

	// 检查JSON-RPC错误
	if errResp, hasErr := resp["error"]; hasErr {
		errMap, _ := errResp.(map[string]interface{})
		if errMap != nil {
			msg, _ := errMap["message"].(string)
			code, _ := errMap["code"].(float64)
			return "", fmt.Errorf("MCP错误(%d): %s", int(code), msg)
		}
		return "", fmt.Errorf("MCP错误: %v", errResp)
	}

	return formatToolResponse(resp), nil
}

// GetTools 返回所有MCP工具定义
func (c *Client) GetTools() []types.ToolDefinition {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

// GetServerNames 返回已连接的服务器名
func (c *Client) GetServerNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	for name := range c.servers {
		names = append(names, name)
	}
	return names
}

// ServerToolInfo 带服务器信息的工具
type ServerToolInfo struct {
	ServerName string
	Def        types.ToolDefinition
}

// GetAllServerTools 返回所有服务器的工具（带服务器名）
func (c *Client) GetAllServerTools() []ServerToolInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []ServerToolInfo
	for name, conn := range c.servers {
		for _, tool := range conn.tools {
			result = append(result, ServerToolInfo{
				ServerName: name,
				Def:        tool,
			})
		}
	}
	return result
}

// Close 关闭所有连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, conn := range c.servers {
		conn.stdin.Close()
		conn.cmd.Process.Kill()
		conn.cmd.Wait()
	}
	c.servers = make(map[string]*ServerConn)
	c.tools = nil
}

// ─── 内部方法 ────────────────────────────────────────────────────────────

func (conn *ServerConn) send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(conn.stdin, "%s\n", data)
	return err
}

func (conn *ServerConn) readResponse() (map[string]interface{}, error) {
	line, err := conn.stdout.ReadString('\n')
	if err != nil {
		return nil, err
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// parseTools 从MCP响应中解析工具定义
func parseTools(resp map[string]interface{}) []types.ToolDefinition {
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil
	}

	toolsRaw, ok := result["tools"].([]interface{})
	if !ok {
		return nil
	}

	var defs []types.ToolDefinition
	for _, t := range toolsRaw {
		toolMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		desc, _ := toolMap["description"].(string)
		schema, _ := toolMap["inputSchema"].(map[string]interface{})

		defs = append(defs, types.ToolDefinition{
			Type: "function",
			Function: types.FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  schema,
			},
		})
	}

	return defs
}

// formatToolResponse 格式化工具响应
func formatToolResponse(resp map[string]interface{}) string {
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		data, _ := json.Marshal(resp)
		return string(data)
	}

	content, ok := result["content"].([]interface{})
	if !ok {
		data, _ := json.Marshal(result)
		return string(data)
	}

	var parts []string
	for _, item := range content {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := itemMap["text"].(string); ok {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n")
}
