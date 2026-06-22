package tool

import "fmt"

// Registry 持有命名的 Tool 实例，并提供查找和元数据列表功能。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建一个空的注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 添加一个工具。名称重复时会 panic。
func (r *Registry) Register(t Tool) {
	name := t.Meta().Name
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool already registered: %s", name))
	}
	r.tools[name] = t
}

// Lookup 返回具有给定名称的工具，未找到时返回错误。
func (r *Registry) Lookup(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// AllMeta 返回每个已注册工具的元数据。
func (r *Registry) AllMeta() []ToolMeta {
	out := make([]ToolMeta, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Meta())
	}
	return out
}
