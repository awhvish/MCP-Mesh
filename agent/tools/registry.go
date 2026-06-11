package tools

import (
	"fmt"

	"github.com/mcp-mesh/agent/config"
	agentmqtt "github.com/mcp-mesh/agent/mqtt"
)

type toolHandler func(params map[string]any) (string, error)

type toolEntry struct {
	def     agentmqtt.ToolDef
	handler toolHandler
}

// Registry maps tool names to their implementations and metadata.
// It satisfies mqtt.ToolExecutor so it can be passed directly to mqtt.NewClient.
type Registry struct {
	cfg   *config.Config
	tools map[string]toolEntry
}

func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		cfg:   cfg,
		tools: make(map[string]toolEntry),
	}

	r.add(agentmqtt.ToolDef{
		Name:        "get_system_info",
		Description: "Returns CPU usage, memory, and disk stats for this device",
	}, r.getSystemInfo)

	r.add(agentmqtt.ToolDef{
		Name:        "list_files",
		Description: "Lists files and directories at a given path",
		Schema:      map[string]any{"path": "string — directory to list"},
	}, r.listFiles)

	r.add(agentmqtt.ToolDef{
		Name:        "read_file",
		Description: "Reads the contents of a file (max 1 MB)",
		Schema:      map[string]any{"path": "string — file to read"},
	}, r.readFile)

	if cfg.Agent.AllowDestructive {
		r.add(agentmqtt.ToolDef{
			Name:        "shutdown_system",
			Description: "Shuts down this device immediately",
		}, r.shutdownSystem)
	}

	return r
}

func (r *Registry) add(def agentmqtt.ToolDef, handler toolHandler) {
	r.tools[def.Name] = toolEntry{def: def, handler: handler}
}

func (r *Registry) Execute(tool string, params map[string]any) (string, error) {
	entry, ok := r.tools[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tool)
	}
	return entry.handler(params)
}

func (r *Registry) ListTools() []agentmqtt.ToolDef {
	defs := make([]agentmqtt.ToolDef, 0, len(r.tools))
	for _, entry := range r.tools {
		defs = append(defs, entry.def)
	}
	return defs
}
