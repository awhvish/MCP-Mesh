// MCP Mesh — Device Agent
// Entry point. Phases 2+ will wire the MQTT client, tool executor,
// local MCP server, and file transfer HTTP server here.
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mcp-mesh/agent/config"
	"github.com/mcp-mesh/agent/logger"
)

func main() {
	configPath := flag.String("config", "config/agent.yml", "path to agent config file")
	dryRun := flag.Bool("dry-run", false, "validate config and exit without starting")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Logger not yet set up — use plain slog so the error is always visible
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	logger.Setup(cfg.LogLevel, cfg.LogFormat)

	slog.Info("agent starting",
		"device", cfg.DeviceName,
		"type", cfg.DeviceType,
		"mcp_port", cfg.Agent.Port,
		"transfer_port", cfg.Agent.TransferPort,
		"mqtt_host", cfg.MQTT.Host,
		"mqtt_port", cfg.MQTT.Port,
	)

	if *dryRun {
		slog.Info("dry run complete", "result", "config valid — exiting")
		return
	}

	// ── Phase 2: MQTT client.Connect() will go here ───────────────────────
	// ── Phase 3: Tool registry.Register() calls will go here ─────────────
	// ── Phase 4: MCP server.Start() will go here ─────────────────────────
	// ── Phase 6: Transfer HTTP server.Start() will go here ───────────────

	slog.Info("agent ready", "msg", "Phase 1 complete — config and logging verified")

	// Block until SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("agent shutting down")
}
