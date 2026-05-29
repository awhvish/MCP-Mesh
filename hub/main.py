"""
MCP Mesh — Central Hub
Entry point. Phases 2+ will wire MQTT, device registry, NLP router,
file transfer coordinator, and FastAPI server here.
"""

from __future__ import annotations

import asyncio
import sys

from hub import config as cfg_module
from hub import logger as log_module


async def main() -> None:
    config = cfg_module.load()

    # Logging must be set up before any other imports that log
    log_module.setup(config)
    log = log_module.get("hub.main")

    log.info(
        "hub_starting",
        host=config.hub.host,
        port=config.hub.port,
        log_level=config.log_level,
        log_format=config.log_format,
    )

    # Validate — will raise with a clear message if anything is missing
    try:
        config.validate()
    except ValueError as exc:
        log.error("config_invalid", error=str(exc))
        sys.exit(1)

    log.info("config_ok", mqtt_host=config.mqtt.host, mqtt_port=config.mqtt.port)

    # ── Phase 2: MQTT client + device registry will be initialised here ──
    # ── Phase 5: NLP router will be initialised here ──────────────────────
    # ── Phase 6: File transfer coordinator will be initialised here ───────
    # ── Phase 2: FastAPI server will start here ───────────────────────────

    log.info("hub_ready", msg="Phase 1 complete — config and logging verified")


if __name__ == "__main__":
    asyncio.run(main())
