// MCP Mesh — Device Agent entry point.
//
// Startup sequence:
//   1. Parse flags and load config
//   2. Set up structured logging
//   3. Connect to MQTT broker (registers device, subscribes to tool calls)
//   4. Block on SIGINT/SIGTERM
//   5. Disconnect cleanly (publishes offline status before TCP close)
//
// Future phases add to step 3 (before blocking):
//   Phase 3: register tools with ToolRegistry, pass to MQTT client
//   Phase 4: start local MCP server (mark3labs/mcp-go)
//   Phase 6: start file transfer HTTP server (Gin)
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcp-mesh/agent/config"
	agentlogger "github.com/mcp-mesh/agent/logger"
	agentmqtt "github.com/mcp-mesh/agent/mqtt"
)

func main() {
	configPath := flag.String("config", "config/agent.yml", "path to agent config file")
	dryRun     := flag.Bool("dry-run", false, "validate config and exit without connecting")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Logger not yet configured — plain slog so the error is always visible
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	agentlogger.Setup(cfg.LogLevel, cfg.LogFormat)

	slog.Info("agent starting",
		"device",         cfg.DeviceName,
		"type",           cfg.DeviceType,
		"mcp_port",       cfg.Agent.Port,
		"transfer_port",  cfg.Agent.TransferPort,
		"broker",         cfg.MQTT.Host,
	)

	if *dryRun {
		slog.Info("dry run complete — config valid, exiting")
		return
	}

	// Phase 2: connect to MQTT broker.
	// nil executor → noopExecutor (returns error for all tool calls).
	// Phase 3 replaces nil with a real ToolRegistry.
	mqttClient := agentmqtt.NewClient(cfg, nil)
	if err := mqttClient.Connect(); err != nil {
		slog.Error("mqtt connect failed", "error", err)
		os.Exit(1)
	}

	// ── Phase 3: tool registry goes here ──────────────────────────────────
	// ── Phase 4: local MCP server starts here ─────────────────────────────
	// ── Phase 6: file transfer HTTP server starts here ────────────────────

	slog.Info("agent ready",
		"device", cfg.DeviceName,
		"msg",    "connected to hub, waiting for tool calls",
	)

	// Block until the OS sends SIGINT (Ctrl+C) or SIGTERM (systemd stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("agent shutting down", "device", cfg.DeviceName)
	mqttClient.Disconnect()
}
