"""
Device Registry — the hub's in-memory record of every connected agent.

Lifecycle:
  1. Agent connects to Mosquitto and publishes a retained register message.
  2. Hub receives it → Registry.register() upserts the Device entry.
  3. If the agent disconnects ungracefully, Mosquitto fires the LWT →
     Registry.mark_offline() flips its status to "offline".
  4. If the agent reconnects, it re-publishes the retained register message →
     Registry.register() flips it back to "online" and refreshes the tool list.

Thread-safety:
  All mutations go through asyncio.Lock so concurrent MQTT callbacks
  (which are posted to the event loop via run_coroutine_threadsafe)
  never race each other.
"""
from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional


@dataclass
class ToolDef:
    """One tool exposed by a device."""
    name: str
    description: str
    schema: dict = field(default_factory=dict)


@dataclass
class Device:
    """
    Everything the hub knows about one agent.

    mcp_url and transfer_url are computed properties so they always
    reflect the current ip/port without any caching issues.
    """
    name: str
    device_type: str          # "pc" | "android" | "printer"
    os: str                   # "linux" | "windows" | "darwin"
    ip: str
    mcp_port: int             # local MCP server (Claude Desktop connects here)
    transfer_port: int        # file transfer HTTP server
    tools: list[ToolDef]
    registered_at: datetime
    last_seen: datetime
    status: str = "online"    # "online" | "offline"

    @property
    def mcp_url(self) -> str:
        return f"http://{self.ip}:{self.mcp_port}"

    @property
    def transfer_url(self) -> str:
        return f"http://{self.ip}:{self.transfer_port}"

    def to_dict(self) -> dict:
        return {
            "name":         self.name,
            "device_type":  self.device_type,
            "os":           self.os,
            "ip":           self.ip,
            "mcp_url":      self.mcp_url,
            "transfer_url": self.transfer_url,
            "tools":        [{"name": t.name, "description": t.description}
                             for t in self.tools],
            "status":       self.status,
            "registered_at": self.registered_at.isoformat(),
            "last_seen":    self.last_seen.isoformat(),
        }


class Registry:
    """
    Thread-safe, asyncio-compatible registry of connected devices.

    All public methods are coroutines so callers always await them —
    this keeps the locking pattern consistent and avoids accidental
    synchronous access from different coroutines.
    """

    def __init__(self) -> None:
        self._devices: dict[str, Device] = {}
        self._lock = asyncio.Lock()

    async def register(self, payload: dict) -> Device:
        """
        Upsert a device from its registration payload.

        If the device already exists (reconnect case), we update its IP
        and tool list but keep the original registered_at timestamp.
        This means registered_at truly reflects first seen time, while
        last_seen reflects the most recent connect.
        """
        async with self._lock:
            now = datetime.now(timezone.utc)
            name = payload["device_name"]

            tools = [
                ToolDef(
                    name=t.get("name", ""),
                    description=t.get("description", ""),
                    schema=t.get("schema", {}),
                )
                for t in payload.get("tools", [])
            ]

            if name in self._devices:
                # Reconnect — refresh mutable fields only
                dev = self._devices[name]
                dev.ip        = payload.get("ip", dev.ip)
                dev.tools     = tools
                dev.last_seen = now
                dev.status    = "online"
            else:
                dev = Device(
                    name=name,
                    device_type=payload.get("device_type", "unknown"),
                    os=payload.get("os", "unknown"),
                    ip=payload.get("ip", ""),
                    mcp_port=int(payload.get("mcp_port", 7800)),
                    transfer_port=int(payload.get("transfer_port", 7801)),
                    tools=tools,
                    registered_at=now,
                    last_seen=now,
                )
                self._devices[name] = dev

            return dev

    async def mark_offline(self, name: str) -> None:
        """Called when the broker fires the device's Last Will Testament."""
        async with self._lock:
            if name in self._devices:
                self._devices[name].status = "offline"

    async def touch(self, name: str) -> None:
        """
        Update last_seen without any other change.
        Called on every inbound message from a device so we have a
        liveness signal even when devices aren't sending tool results.
        """
        async with self._lock:
            if name in self._devices:
                self._devices[name].last_seen = datetime.now(timezone.utc)

    # ── Read-only accessors (no lock needed — dict reads are atomic in CPython) ─

    def get(self, name: str) -> Optional[Device]:
        return self._devices.get(name)

    def get_all(self) -> dict[str, Device]:
        return dict(self._devices)

    def online_devices(self) -> dict[str, Device]:
        return {n: d for n, d in self._devices.items() if d.status == "online"}

    def count(self) -> int:
        return len(self._devices)
