// Package mqtt provides the agent's MQTT connection to the hub.
//
// Concurrency model:
//   paho-mqtt is callback-driven and thread-safe internally.
//   onConnect and onToolCall fire on paho's goroutine.
//   We don't share any mutable state with the main goroutine, so no
//   additional locking is needed beyond what paho provides.
package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/mcp-mesh/agent/config"
)

// ── ToolExecutor interface ────────────────────────────────────────────────────

// ToolExecutor is the contract between the MQTT client and the tool layer.
// In Phase 2 only a no-op stub is used. Phase 3 provides the real implementation.
//
// Execute returns a string (typically JSON) or an error.
// The MQTT client wraps the error into a ResultMsg.Error field.
//
// ListTools returns the tool definitions that are sent in the registration
// message so the hub knows what this device can do.
type ToolExecutor interface {
	Execute(tool string, params map[string]any) (string, error)
	ListTools() []ToolDef
}

// noopExecutor satisfies ToolExecutor for Phase 2 — returns a clear error
// instead of panicking, making it safe to receive tool calls before Phase 3.
type noopExecutor struct{}

func (n *noopExecutor) Execute(tool string, _ map[string]any) (string, error) {
	return "", fmt.Errorf("tool executor not initialised (Phase 3 pending)")
}
func (n *noopExecutor) ListTools() []ToolDef { return nil }

// ── Client ────────────────────────────────────────────────────────────────────

// Client wraps a paho MQTT connection and ties it to the tool executor.
type Client struct {
	cfg      *config.Config
	executor ToolExecutor
	paho     paho.Client
}

// NewClient creates a Client. Pass nil for executor to use the no-op stub.
func NewClient(cfg *config.Config, executor ToolExecutor) *Client {
	if executor == nil {
		executor = &noopExecutor{}
	}
	return &Client{cfg: cfg, executor: executor}
}

// Connect establishes the broker connection and blocks until it succeeds
// or returns an error.
//
// LWT (Last Will Testament):
//   We register the LWT before connecting. If this process dies without
//   sending a clean DISCONNECT, the broker publishes the LWT payload on our
//   behalf. The hub receives it, calls registry.mark_offline(), and the
//   device appears offline in GET /devices.
//
// retain=true on the registration publish:
//   The broker stores our registration payload. Any new subscriber
//   (e.g. the hub after a restart) receives it immediately on subscribe
//   without waiting for us to re-publish.
func (c *Client) Connect() error {
	lwt, err := json.Marshal(StatusMsg{
		Status:     "offline",
		DeviceName: c.cfg.DeviceName,
	})
	if err != nil {
		return fmt.Errorf("encoding LWT payload: %w", err)
	}

	opts := paho.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", c.cfg.MQTT.Host, c.cfg.MQTT.Port))
	opts.SetClientID("mesh-agent-" + c.cfg.DeviceName)
	opts.SetUsername(c.cfg.MQTT.Username)
	opts.SetPassword(c.cfg.MQTT.Password)
	opts.SetKeepAlive(time.Duration(c.cfg.MQTT.Keepalive) * time.Second)

	// LWT — broker publishes this automatically on ungraceful disconnect
	opts.SetWill(
		StatusTopic(c.cfg.DeviceName),
		string(lwt),
		1,    // QoS 1: at-least-once
		true, // retain: hub sees it even after re-subscribing
	)

	// Auto-reconnect with backoff. paho retries indefinitely;
	// ConnectRetryInterval is the initial delay, doubling up to MaxReconnectInterval.
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetMaxReconnectInterval(60 * time.Second)

	opts.SetOnConnectHandler(c.onConnect)
	opts.SetConnectionLostHandler(c.onConnectionLost)

	c.paho = paho.NewClient(opts)

	// Connect() is synchronous: it blocks until the TCP handshake and
	// MQTT CONNACK complete, or until an error occurs.
	token := c.paho.Connect()
	token.Wait()
	return token.Error()
}

// Disconnect sends a clean MQTT DISCONNECT before closing the TCP connection.
// This tells the broker NOT to fire the LWT — we publish "offline" ourselves
// so the hub's registry updates correctly even on graceful shutdown.
func (c *Client) Disconnect() {
	if c.paho == nil || !c.paho.IsConnected() {
		return
	}

	// Publish our own offline status (LWT only fires on ungraceful exit)
	payload, _ := json.Marshal(StatusMsg{
		Status:     "offline",
		DeviceName: c.cfg.DeviceName,
	})
	c.paho.Publish(StatusTopic(c.cfg.DeviceName), 1, true, string(payload)).Wait()

	c.paho.Disconnect(500) // 500 ms grace period to flush pending messages
	slog.Info("mqtt disconnected gracefully")
}

// ── paho event handlers ───────────────────────────────────────────────────────

// onConnect fires on every successful connection, including reconnects.
// We always re-publish registration here because:
//   a) On first connect, the hub gets our registration for the first time.
//   b) On reconnect, we may have updated our tool list; the hub needs the
//      latest. Because we publish with retain=true, the broker overwrites
//      the stale stored message with the fresh one.
func (c *Client) onConnect(client paho.Client) {
	slog.Info("mqtt connected",
		"broker", c.cfg.MQTT.Host,
		"port", c.cfg.MQTT.Port,
	)

	if err := c.publishRegistration(); err != nil {
		slog.Error("registration publish failed", "error", err)
	}

	// Subscribe to tool calls on our dedicated topic
	callTopic := CallTopic(c.cfg.DeviceName)
	if token := client.Subscribe(callTopic, 1, c.onToolCall); token.Wait() && token.Error() != nil {
		slog.Error("subscribe failed", "topic", callTopic, "error", token.Error())
		return
	}
	slog.Info("ready for tool calls", "topic", callTopic)
}

func (c *Client) onConnectionLost(_ paho.Client, err error) {
	slog.Warn("mqtt connection lost — reconnecting",
		"error", err,
	)
	// paho handles reconnects automatically; we just log here
}

// onToolCall is called by paho's goroutine for every message on the call topic.
//
// Flow:
//   1. Decode the CallMsg from JSON
//   2. Call executor.Execute() — this may be slow (disk I/O, subprocess, etc.)
//   3. Build a ResultMsg with either the output or the error
//   4. Publish the ResultMsg back to the hub's result topic
//
// executor.Execute() is called synchronously on paho's callback goroutine.
// For Phase 2 (no-op executor) this is fine. Phase 3 will optionally wrap
// slow tools in a goroutine if needed.
func (c *Client) onToolCall(_ paho.Client, msg paho.Message) {
	call, err := DecodeCall(msg.Payload())
	if err != nil {
		slog.Error("malformed tool call", "error", err)
		return
	}

	slog.Info("tool call received",
		"call_id", call.CallID,
		"tool", call.Tool,
		"params", call.Params,
	)

	result, execErr := c.executor.Execute(call.Tool, call.Params)

	var res ResultMsg
	if execErr != nil {
		res = ResultMsg{
			Type:   "tool_result",
			CallID: call.CallID,
			Error:  execErr.Error(),
		}
		slog.Warn("tool execution failed",
			"call_id", call.CallID,
			"tool", call.Tool,
			"error", execErr,
		)
	} else {
		res = ResultMsg{
			Type:   "tool_result",
			CallID: call.CallID,
			Result: result,
		}
		slog.Info("tool call success",
			"call_id", call.CallID,
			"tool", call.Tool,
		)
	}

	payload, _ := MarshalResult(res)
	// retain=false: results are point-in-time, not useful to store
	c.paho.Publish(ResultTopic(c.cfg.DeviceName), 1, false, string(payload))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *Client) publishRegistration() error {
	msg := RegisterMsg{
		Type:         "register",
		DeviceName:   c.cfg.DeviceName,
		DeviceType:   c.cfg.DeviceType,
		OS:           runtime.GOOS,
		IP:           getLANIP(),
		MCPPort:      c.cfg.Agent.Port,
		TransferPort: c.cfg.Agent.TransferPort,
		Tools:        c.executor.ListTools(),
		RegisteredAt: time.Now().UTC(),
	}

	payload, err := MarshalRegister(msg)
	if err != nil {
		return fmt.Errorf("marshalling registration: %w", err)
	}

	// retain=true: broker stores this so the hub gets it even after restarting
	token := c.paho.Publish(RegisterTopic(c.cfg.DeviceName), 1, true, string(payload))
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("publishing registration: %w", err)
	}

	slog.Info("registration published",
		"device", c.cfg.DeviceName,
		"ip", msg.IP,
		"tools", len(msg.Tools),
		"os", msg.OS,
	)
	return nil
}

// getLANIP returns the device's LAN-facing IP address.
//
// Technique: open a UDP "connection" to a public IP (8.8.8.8).
// No packets are actually sent — this is purely a routing table lookup.
// The OS fills in the source address that would be used for that route,
// which is our LAN IP. Falls back to hostname lookup if UDP fails.
func getLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		hostname, _ := os.Hostname()
		addrs, _ := net.LookupHost(hostname)
		if len(addrs) > 0 {
			return addrs[0]
		}
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
