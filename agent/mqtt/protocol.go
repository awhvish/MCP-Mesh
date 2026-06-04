// Package mqtt owns the MQTT topic names and JSON message shapes
// for the agent side of the MCP Mesh protocol.
//
// Every struct here has a direct Python counterpart in hub/protocol.py.
// When you change a field name or topic string, update both files.
package mqtt

import (
	"encoding/json"
	"fmt"
	"time"
)

// ── Topic helpers ─────────────────────────────────────────────────────────────

const (
	topicRegister = "mesh/devices/%s/register"
	topicStatus   = "mesh/devices/%s/status"
	topicCall     = "mesh/devices/%s/tools/call"
	topicResult   = "mesh/devices/%s/tools/result"
)

func RegisterTopic(device string) string { return fmt.Sprintf(topicRegister, device) }
func StatusTopic(device string) string   { return fmt.Sprintf(topicStatus, device) }
func CallTopic(device string) string     { return fmt.Sprintf(topicCall, device) }
func ResultTopic(device string) string   { return fmt.Sprintf(topicResult, device) }

// ── Message types ─────────────────────────────────────────────────────────────

// ToolDef describes one tool this device exposes.
// The hub stores these and uses them to build the NLP context prompt (Phase 5).
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema,omitempty"`
}

// RegisterMsg is published with retain=true on every connect.
// retain=true means Mosquitto stores the last value and replays it
// to any subscriber that joins later — including a hub that restarts
// after the device is already connected.
type RegisterMsg struct {
	Type         string    `json:"type"`           // always "register"
	DeviceName   string    `json:"device_name"`
	DeviceType   string    `json:"device_type"`    // pc | android | printer
	OS           string    `json:"os"`             // linux | windows | darwin
	IP           string    `json:"ip"`             // LAN IP
	MCPPort      int       `json:"mcp_port"`       // local MCP server
	TransferPort int       `json:"transfer_port"`  // file transfer HTTP
	Tools        []ToolDef `json:"tools"`
	RegisteredAt time.Time `json:"registered_at"`
}

// StatusMsg is the Last Will Testament payload.
// The broker publishes this automatically if the device disconnects
// without sending a proper MQTT DISCONNECT packet (e.g. network drop,
// process killed). The hub receives it on mesh/devices/{id}/status
// and marks the device offline in the registry.
type StatusMsg struct {
	Status     string `json:"status"`      // "offline" | "online"
	DeviceName string `json:"device_name"`
}

// CallMsg is sent by the hub to invoke a tool on this device.
// The device receives it on mesh/devices/{id}/tools/call.
type CallMsg struct {
	Type   string         `json:"type"`    // always "call_tool"
	CallID string         `json:"call_id"` // correlates with ResultMsg
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
}

// ResultMsg is sent back to the hub after tool execution.
// Exactly one of Result or Error will be non-empty.
type ResultMsg struct {
	Type   string `json:"type"`             // always "tool_result"
	CallID string `json:"call_id"`
	Result string `json:"result,omitempty"` // tool output (JSON or plain text)
	Error  string `json:"error,omitempty"`  // error message if execution failed
}

// ── Encode / Decode ───────────────────────────────────────────────────────────

func MarshalRegister(msg RegisterMsg) ([]byte, error) { return json.Marshal(msg) }
func MarshalStatus(msg StatusMsg) ([]byte, error)     { return json.Marshal(msg) }
func MarshalResult(msg ResultMsg) ([]byte, error)     { return json.Marshal(msg) }

// DecodeCall parses a raw MQTT payload into a CallMsg.
// Returns an error if the JSON is malformed or the type field is wrong.
func DecodeCall(data []byte) (*CallMsg, error) {
	var msg CallMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decoding call message: %w", err)
	}
	return &msg, nil
}
