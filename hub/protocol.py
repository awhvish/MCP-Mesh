"""
MQTT message protocol for MCP Mesh.

Every message exchanged between hub and agent is JSON over MQTT.
This module owns the topic structure and message shapes for both sides.

Topic map:
  mesh/devices/{id}/register      Device → Hub   retained=True, on every connect
  mesh/devices/{id}/status        Device → Hub   LWT payload = {"status":"offline"}
  mesh/devices/{id}/tools/call    Hub → Device   tool invocation
  mesh/devices/{id}/tools/result  Device → Hub   tool result
"""
from __future__ import annotations

import json
import uuid


# ── Topic templates ───────────────────────────────────────────────────────────

T_REGISTER = "mesh/devices/{}/register"
T_STATUS    = "mesh/devices/{}/status"
T_CALL      = "mesh/devices/{}/tools/call"
T_RESULT    = "mesh/devices/{}/tools/result"

# Wildcards the hub subscribes to — '+' matches exactly one level
SUB_REGISTER = "mesh/devices/+/register"
SUB_STATUS   = "mesh/devices/+/status"
SUB_RESULT   = "mesh/devices/+/tools/result"


# ── Helpers ───────────────────────────────────────────────────────────────────

def new_call_id() -> str:
    """8-char hex ID — short enough to log, unique enough for LAN scale."""
    return uuid.uuid4().hex[:8]


def extract_device_name(topic: str) -> str:
    """
    'mesh/devices/alice-laptop/tools/result' → 'alice-laptop'
    Splits on '/' and returns the third segment (index 2).
    """
    parts = topic.split("/")
    return parts[2] if len(parts) >= 3 else ""


def decode(payload: bytes) -> dict:
    return json.loads(payload.decode())


def encode(data: dict) -> str:
    return json.dumps(data)


# ── Outbound message builders (hub → device) ──────────────────────────────────

def call_msg(call_id: str, tool: str, params: dict) -> str:
    """
    Build a tool-call message.
    The device receives this on mesh/devices/{id}/tools/call,
    executes the tool, and replies with result_msg on the result topic.
    """
    return encode({
        "type":    "call_tool",
        "call_id": call_id,
        "tool":    tool,
        "params":  params,
    })
