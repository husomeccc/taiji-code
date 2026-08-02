package tools

import (
	"context"
	"taiji-code/types"
)

// Tool is the interface all tools must implement
type Tool interface {
	// Name returns the tool's identifier
	Name() string

	// Description returns what this tool does (shown to LLM)
	Description() string

	// Parameters returns the JSON Schema for parameters
	Parameters() map[string]interface{}

	// Execute runs the tool with given arguments
	Execute(ctx context.Context, args map[string]interface{}) (string, error)

	// PermissionLevel returns the required permission
	PermissionLevel() types.PermissionLevel
}

// Registry holds all available tools
type Registry struct {
	tools map[string]Tool
	order []string // maintain insertion order
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Get returns a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns all tool definitions for the LLM
func (r *Registry) Definitions() []types.ToolDefinition {
	defs := make([]types.ToolDefinition, 0, len(r.tools))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, types.ToolDefinition{
			Type: "function",
			Function: types.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

// Names returns all registered tool names
func (r *Registry) Names() []string {
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}
