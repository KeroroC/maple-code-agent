package tool

import "fmt"

// Registry holds named Tool instances and provides lookup and metadata listing.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Panics on duplicate name.
func (r *Registry) Register(t Tool) {
	name := t.Meta().Name
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool already registered: %s", name))
	}
	r.tools[name] = t
}

// Lookup returns the tool with the given name, or an error if not found.
func (r *Registry) Lookup(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// AllMeta returns metadata for every registered tool.
func (r *Registry) AllMeta() []ToolMeta {
	out := make([]ToolMeta, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Meta())
	}
	return out
}