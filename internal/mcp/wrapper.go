package mcp

import (
	"context"
	"fmt"
	"taiji-code/internal/tools"
	"taiji-code/types"
)

// McpToolWrapper 将MCP工具包装为Tool接口
type McpToolWrapper struct {
	client     *Client
	serverName string
	def        types.ToolDefinition
}

// NewMcpToolWrapper 创建MCP工具包装器
func NewMcpToolWrapper(client *Client, serverName string, def types.ToolDefinition) *McpToolWrapper {
	return &McpToolWrapper{
		client:     client,
		serverName: serverName,
		def:        def,
	}
}

func (t *McpToolWrapper) Name() string        { return t.def.Function.Name }
func (t *McpToolWrapper) Description() string  { return t.def.Function.Description }
func (t *McpToolWrapper) Parameters() map[string]interface{} {
	if t.def.Function.Parameters != nil {
		return t.def.Function.Parameters
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}
func (t *McpToolWrapper) PermissionLevel() types.PermissionLevel {
	return types.PermAskUser
}

func (t *McpToolWrapper) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	result, err := t.client.CallTool(ctx, t.serverName, t.def.Function.Name, args)
	if err != nil {
		return "", fmt.Errorf("MCP工具调用失败: %w", err)
	}
	return result, nil
}

// RegisterMcpTools 将所有MCP工具注册到Registry
func RegisterMcpTools(client *Client, registry *tools.Registry) int {
	count := 0
	serverTools := client.GetAllServerTools()
	for _, st := range serverTools {
		wrapper := NewMcpToolWrapper(client, st.ServerName, st.Def)
		registry.Register(wrapper)
		count++
	}
	return count
}

// 确保接口被实现
var _ tools.Tool = (*McpToolWrapper)(nil)
