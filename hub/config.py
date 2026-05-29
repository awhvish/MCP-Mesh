from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path

import yaml


@dataclass
class MQTTConfig:
    host: str = "localhost"
    port: int = 1883
    username: str = "meshuser"
    password: str = ""
    tls: bool = False
    keepalive: int = 60


@dataclass
class HubConfig:
    host: str = "0.0.0.0"
    port: int = 7700
    secret: str = ""


@dataclass
class Config:
    anthropic_api_key: str = ""
    log_level: str = "INFO"
    log_format: str = "pretty"
    mqtt: MQTTConfig = field(default_factory=MQTTConfig)
    hub: HubConfig = field(default_factory=HubConfig)

    def validate(self) -> None:
        errors: list[str] = []
        if not self.anthropic_api_key:
            errors.append(
                "anthropic_api_key is required — set it in config/hub.yml "
                "or export ANTHROPIC_API_KEY"
            )
        if not self.hub.secret:
            errors.append(
                "hub.secret is required — generate with: "
                "python -c \"import secrets; print(secrets.token_hex(32))\""
            )
        if not self.mqtt.password:
            errors.append(
                "mqtt.password is required — set it in config/hub.yml "
                "or export MQTT_PASSWORD"
            )
        if errors:
            raise ValueError("Configuration errors:\n  " + "\n  ".join(errors))


def load(path: str = "config/hub.yml") -> Config:
    data: dict = {}
    config_path = Path(path)
    if config_path.exists():
        with open(config_path) as f:
            data = yaml.safe_load(f) or {}

    mqtt_raw = data.get("mqtt", {})
    hub_raw = data.get("hub", {})

    return Config(
        anthropic_api_key=(
            data.get("anthropic_api_key") or os.environ.get("ANTHROPIC_API_KEY", "")
        ),
        log_level=data.get("log_level", "INFO").upper(),
        log_format=data.get("log_format", "pretty"),
        mqtt=MQTTConfig(
            host=mqtt_raw.get("host", "localhost"),
            port=int(mqtt_raw.get("port", 1883)),
            username=mqtt_raw.get("username", "meshuser"),
            password=(
                mqtt_raw.get("password") or os.environ.get("MQTT_PASSWORD", "")
            ),
            tls=bool(mqtt_raw.get("tls", False)),
            keepalive=int(mqtt_raw.get("keepalive", 60)),
        ),
        hub=HubConfig(
            host=hub_raw.get("host", "0.0.0.0"),
            port=int(hub_raw.get("port", 7700)),
            secret=(
                hub_raw.get("secret") or os.environ.get("MESH_SECRET", "")
            ),
        ),
    )
