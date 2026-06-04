"""
MCP Mesh — Central Hub entry point.

Startup order (matters):
  1. Load + validate config          — fail fast, clear error message
  2. Set up logging                  — all subsequent log calls are formatted
  3. Create Registry + MQTTClient    — objects only, no I/O yet
  4. FastAPI lifespan connects MQTT  — actual broker handshake happens here
  5. uvicorn serves the API          — /health and /devices are now live

Shutdown order (reverse):
  uvicorn signals lifespan exit → MQTT disconnect → process exits
"""
from __future__ import annotations

import asyncio
import sys
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse

from hub import config as cfg_module
from hub import logger as log_module
from hub.mqtt_client import MQTTClient
from hub.registry import Registry


def build_app(config, registry: Registry, mqtt: MQTTClient) -> FastAPI:
    """
    Construct the FastAPI application.

    The lifespan context manager replaces the old @app.on_event("startup")
    pattern — it's the modern FastAPI way and guarantees cleanup even if
    startup raises.
    """
    @asynccontextmanager
    async def lifespan(app: FastAPI):
        log = log_module.get("hub.main")
        try:
            await mqtt.connect()
            log.info("hub_ready",
                     host=config.hub.host,
                     port=config.hub.port,
                     mqtt=f"{config.mqtt.host}:{config.mqtt.port}")
        except RuntimeError as exc:
            log.error("mqtt_startup_failed", error=str(exc))
            sys.exit(1)

        yield  # ← application runs here

        await mqtt.disconnect()

    app = FastAPI(
        title="MCP Mesh Hub",
        version="0.1.0",
        lifespan=lifespan,
    )

    # ── Routes ────────────────────────────────────────────────────────────────

    @app.get("/health")
    def health():
        """
        Liveness probe. Returns 200 as long as the process is up.
        The mqtt field tells you whether the broker connection is live.
        Used by systemd (ExecStartPost health check) and monitoring tools.
        """
        return {
            "status":  "ok",
            "mqtt":    "connected" if mqtt.is_connected else "disconnected",
            "devices": registry.count(),
        }

    @app.get("/devices")
    def devices():
        """
        Return all known devices (online and offline).
        Each entry includes the device's MCP URL so clients know where
        to connect Claude Desktop for direct per-device access.
        """
        return {
            name: dev.to_dict()
            for name, dev in registry.get_all().items()
        }

    @app.get("/devices/{name}")
    def device(name: str):
        """Return a single device by name, or 404 if unknown."""
        dev = registry.get(name)
        if dev is None:
            raise HTTPException(status_code=404, detail=f"Device '{name}' not found")
        return dev.to_dict()

    # Phase 5 will add:
    #   POST /command  — NLP command endpoint

    return app


async def main() -> None:
    config = cfg_module.load()
    log_module.setup(config)
    log = log_module.get("hub.main")

    log.info("hub_starting",
             host=config.hub.host,
             port=config.hub.port)

    try:
        config.validate()
    except ValueError as exc:
        log.error("config_invalid", error=str(exc))
        sys.exit(1)

    registry   = Registry()
    mqtt_client = MQTTClient(config.mqtt, registry)
    app        = build_app(config, registry, mqtt_client)

    server_cfg = uvicorn.Config(
        app,
        host=config.hub.host,
        port=config.hub.port,
        log_level="warning",   # uvicorn's own logs suppressed; structlog handles ours
    )
    await uvicorn.Server(server_cfg).serve()


if __name__ == "__main__":
    asyncio.run(main())
