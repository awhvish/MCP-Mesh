"""
Hub MQTT client — connects to Mosquitto, fans incoming messages out to handlers.

Threading model (important):
  paho-mqtt runs its own network thread internally (loop_start).
  Its callbacks (_on_connect, _on_disconnect, _on_message) fire on THAT thread,
  not on the asyncio event loop. To safely touch asyncio objects (like the
  Registry's asyncio.Lock or asyncio.Future objects), every callback uses
  asyncio.run_coroutine_threadsafe() to hand work back to the main loop.

Reconnection:
  paho handles reconnects automatically after the initial connection succeeds.
  reconnect_delay_set(1, 60) means it backs off from 1 s up to 60 s between
  attempts. The _connected Event is cleared on disconnect and set again once
  the broker acknowledges the re-subscribe.

Pending call futures (Phase 3):
  When the hub calls a tool on a device, it creates an asyncio.Future keyed
  by call_id. When the device's result arrives here in _handle_result, the
  future is resolved. Phase 3 wires this up; the dict is already here so
  mqtt_client.py needs no changes in Phase 3.
"""
from __future__ import annotations

import asyncio
import json
import logging
from typing import Optional

import paho.mqtt.client as mqtt

from hub import protocol as proto
from hub import logger as log_module
from hub.config import MQTTConfig
from hub.registry import Registry

log = log_module.get("hub.mqtt")


class MQTTClient:
    def __init__(self, config: MQTTConfig, registry: Registry) -> None:
        self.config   = config
        self.registry = registry

        # Set by connect(); used to post work back from paho's thread
        self._loop: Optional[asyncio.AbstractEventLoop] = None
        self._client: Optional[mqtt.Client] = None

        # asyncio Event: set = connected, cleared = disconnected/reconnecting
        self._connected = asyncio.Event()

        # call_id → Future[str]: resolved when a tool result arrives (Phase 3)
        self._pending: dict[str, asyncio.Future] = {}

    # ── Setup ─────────────────────────────────────────────────────────────────

    def _build_paho_client(self) -> mqtt.Client:
        """
        Create and configure the paho Client.

        CallbackAPIVersion.VERSION2 uses the new paho-mqtt 2.x callback
        signatures:
          on_connect(client, userdata, flags, reason_code, properties)
          on_disconnect(client, userdata, disconnect_flags, reason_code, properties)
        The old VERSION1 signatures differ and will raise TypeError if mixed.
        """
        client = mqtt.Client(
            mqtt.CallbackAPIVersion.VERSION2,
            client_id="mcp-mesh-hub",
            clean_session=True,
        )
        client.username_pw_set(self.config.username, self.config.password)

        # Exponential backoff: retry every 1 s after first failure, up to 60 s
        client.reconnect_delay_set(min_delay=1, max_delay=60)

        client.on_connect    = self._on_connect
        client.on_disconnect = self._on_disconnect
        client.on_message    = self._on_message

        return client

    # ── paho callbacks — run on paho's background thread ──────────────────────

    def _on_connect(self, client, userdata, flags, reason_code, properties):
        if reason_code.is_failure:
            log.error("mqtt_connect_failed", reason=str(reason_code))
            return

        log.info("mqtt_connected",
                 broker=self.config.host,
                 port=self.config.port)

        # Subscribe to all device topics.
        # QoS 1 = at-least-once delivery: the broker re-sends if the hub
        # doesn't acknowledge within its retry window.
        subs = [
            (proto.SUB_REGISTER, 1),
            (proto.SUB_STATUS,   1),
            (proto.SUB_RESULT,   1),
        ]
        client.subscribe(subs)
        log.info("mqtt_subscribed", topics=[s[0] for s in subs])

        # Because Mosquitto stores retained messages, re-subscribing after
        # a reconnect immediately replays the registration of every agent
        # that is still online — no manual re-registration needed.

        # Signal the asyncio side that we're ready
        if self._loop:
            self._loop.call_soon_threadsafe(self._connected.set)

    def _on_disconnect(self, client, userdata, disconnect_flags, reason_code, properties):
        log.warning("mqtt_disconnected",
                    reason=str(reason_code),
                    will_reconnect=not reason_code.is_failure)
        if self._loop:
            self._loop.call_soon_threadsafe(self._connected.clear)

    def _on_message(self, client, userdata, msg: mqtt.MQTTMessage):
        """
        Called by paho's thread for every incoming message.
        We decode JSON here (cheap, no I/O) then hand the coroutine to the
        event loop so all asyncio objects are touched from the right thread.
        """
        topic = msg.topic
        try:
            payload = proto.decode(msg.payload)
        except Exception as exc:
            log.warning("mqtt_bad_payload", topic=topic, error=str(exc))
            return

        device_name = proto.extract_device_name(topic)

        if self._loop:
            asyncio.run_coroutine_threadsafe(
                self._dispatch(topic, device_name, payload),
                self._loop,
            )

    # ── Dispatch — runs on the asyncio event loop ──────────────────────────────

    async def _dispatch(self, topic: str, device_name: str, payload: dict) -> None:
        """Route an incoming message to the right handler by topic suffix."""
        if topic.endswith("/register"):
            await self._handle_register(device_name, payload)
        elif topic.endswith("/status"):
            await self._handle_status(device_name, payload)
        elif topic.endswith("/tools/result"):
            await self._handle_result(device_name, payload)

    async def _handle_register(self, device_name: str, payload: dict) -> None:
        device = await self.registry.register(payload)
        log.info("device_registered",
                 device=device.name,
                 type=device.device_type,
                 os=device.os,
                 ip=device.ip,
                 tools=len(device.tools))

    async def _handle_status(self, device_name: str, payload: dict) -> None:
        """
        Status messages come from two sources:
          1. The LWT (Last Will Testament) — broker sends {"status":"offline"}
             automatically when the device drops without a clean disconnect.
          2. The device itself at graceful shutdown — same payload, but
             delivered by the device before it calls Disconnect().
        Both are handled identically: flip the registry entry to offline.
        """
        status = payload.get("status", "")
        if status == "offline":
            await self.registry.mark_offline(device_name)
            log.info("device_offline", device=device_name)
        else:
            # "online" heartbeats — just refresh last_seen
            await self.registry.touch(device_name)

    async def _handle_result(self, device_name: str, payload: dict) -> None:
        """
        Resolve the pending Future for this call_id so the hub's
        dispatcher (Phase 3) gets the result back to the caller.
        """
        await self.registry.touch(device_name)
        call_id = payload.get("call_id", "")
        future  = self._pending.pop(call_id, None)

        if future is None:
            # Result arrived but no one is waiting — timed out or duplicate
            log.debug("tool_result_unmatched",
                      device=device_name,
                      call_id=call_id)
            return

        if payload.get("error"):
            future.set_exception(RuntimeError(payload["error"]))
        else:
            future.set_result(payload.get("result", ""))

        log.info("tool_result_dispatched",
                 device=device_name,
                 call_id=call_id)

    # ── Public API ────────────────────────────────────────────────────────────

    async def connect(self) -> None:
        """
        Start the MQTT client and wait up to 10 s for the broker handshake.
        Raises RuntimeError if the broker is not reachable within the timeout.
        """
        self._loop   = asyncio.get_running_loop()
        self._client = self._build_paho_client()

        # connect_async() queues the TCP connect without blocking —
        # loop_start() spins up paho's background thread to process it.
        self._client.connect_async(
            self.config.host,
            self.config.port,
            self.config.keepalive,
        )
        self._client.loop_start()

        try:
            await asyncio.wait_for(self._connected.wait(), timeout=10.0)
        except asyncio.TimeoutError:
            raise RuntimeError(
                f"Could not reach MQTT broker at "
                f"{self.config.host}:{self.config.port} within 10 s. "
                "Is Mosquitto running?"
            )

    async def disconnect(self) -> None:
        if self._client:
            self._client.loop_stop()
            self._client.disconnect()
            log.info("mqtt_disconnected_clean")

    def publish(self, topic: str, payload: dict | str,
                retain: bool = False, qos: int = 1) -> None:
        """
        Fire-and-forget publish. Safe to call from the asyncio event loop
        because paho.publish() is thread-safe.
        retain=True makes Mosquitto store the last message on this topic
        so new subscribers receive it immediately on subscribe.
        """
        if not self._client:
            return
        raw = payload if isinstance(payload, str) else json.dumps(payload)
        self._client.publish(topic, raw, qos=qos, retain=retain)

    def register_pending(self, call_id: str) -> asyncio.Future:
        """
        Create and register a Future for an in-flight tool call.
        Called by the dispatcher (Phase 3) before publishing the call message.
        """
        future = self._loop.create_future()
        self._pending[call_id] = future
        return future

    @property
    def is_connected(self) -> bool:
        return self._connected.is_set()
