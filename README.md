# MCP Mesh

NLP-driven LAN control plane — manage every device on your network (PCs, phones, printers) from a single terminal using natural language and MCP.

```
> shut down my laptop
  ⚠  Shut down alice-laptop? [y/N]: y
  ✓  Shutdown initiated. System will power off in 5 seconds.

> list files in phone's Downloads folder
  ✓  photo.jpg  (4.2 MB)  2026-05-29
      video.mp4 (118 MB)  2026-05-28
      notes.txt (1.1 KB)  2026-05-27

> transfer photo.jpg from phone to laptop
  ✓  Transferring photo.jpg (4.2 MB) ████████████████████ 100%  done in 1.2s
```

---

## Architecture

```
CLI  ──HTTP──▶  Central Hub (Python)
                  ├── FastAPI REST
                  ├── MQTT client  ──── Mosquitto broker ────┐
                  ├── Device Registry                        │
                  └── NLP Router (Claude Sonnet)         MQTT pub/sub
                                                             │
                              ┌──────────────────────────────┘
                              │
              ┌───────────────┼──────────────────┐
              ▼               ▼                  ▼
          PC Agent       Phone Agent       Printer Agent  (Go binary)
          :7800 MCP      :7800 MCP         :7800 MCP
          :7801 HTTP     :7801 HTTP        :7801 HTTP
```

**Hub-and-spoke topology.** One hub per LAN; all commands route through it. Device agents are statically compiled Go binaries that run on any Linux/Windows/macOS device — including Android via Termux.

Each agent also exposes a local MCP server on port 7800, so Claude Desktop can connect directly to any device without the hub.

---

## Tech Stack

| Layer | Technology |
|---|---|
| NLP routing | Claude Sonnet 4.6 (`anthropic` SDK) |
| Hub API | Python · FastAPI · uvicorn |
| Hub logging | `structlog` (pretty dev / JSON prod) |
| Device agents | Go 1.23 · single static binary |
| Messaging | MQTT · Eclipse Mosquitto |
| Local MCP server | `mark3labs/mcp-go` |
| File transfer | Chunked HTTP · SHA-256 verified |
| Deploy | systemd · Docker (hub) · one-line install (agents) |

---

## Quick Start

### 1 — Hub machine (always-on PC)

```bash
# Install Mosquitto
sudo apt install mosquitto mosquitto-clients
sudo mosquitto_passwd -c /etc/mosquitto/passwd meshuser
sudo cp deploy/mosquitto.conf /etc/mosquitto/conf.d/mcp-mesh.conf
sudo systemctl restart mosquitto

# Configure hub
cp config/hub.example.yml config/hub.yml
# Edit config/hub.yml — set anthropic_api_key, hub.secret, mqtt.password

# Install Python deps and start
pip install -r requirements.txt
python hub/main.py
```

### 2 — Every other device (PC or phone via Termux)

```bash
# Build agent binary on hub machine
make build-all

# Copy binary + config to target device, then on the device:
bash deploy/install-agent.sh my-laptop 192.168.1.100
# Edit ~/.config/mcp-mesh/agent.yml — set secret and mqtt.password
~/.local/bin/mesh-agent
```

### 3 — CLI (any machine on the LAN)

```bash
python cli/main.py
```

---

## Example Commands

```
# System control
shut down my laptop
reboot the server
put alice-phone to sleep
cancel the pending shutdown on bob-laptop

# Files
list files in /home/user/Documents on my laptop
read config.txt from my laptop
find all .pdf files on my phone
delete old-backup.zip from laptop (asks for confirmation)

# File transfer
transfer photo.jpg from phone to laptop
copy report.pdf from laptop to /sdcard/Documents on phone

# Android (Termux)
call +1-555-0100 from my phone
send sms to mom saying "on my way"
what's my phone's battery level?
show me my contacts named John

# Info
what processes is my laptop running?
show network info for all devices
what devices are connected?
```

---

## Configuration

### Hub (`config/hub.yml`)

```yaml
anthropic_api_key: "sk-ant-..."   # or: export ANTHROPIC_API_KEY=...
log_level: "INFO"
log_format: "pretty"              # pretty | json

hub:
  host: "0.0.0.0"
  port: 7700
  secret: "random-64-char-hex"   # or: export MESH_SECRET=...

mqtt:
  host: "localhost"
  port: 1883
  username: "meshuser"
  password: "yourpassword"       # or: export MQTT_PASSWORD=...
```

### Agent (`config/agent.yml`)

```yaml
device_name: "alice-laptop"      # or: export MESH_DEVICE_NAME=...
device_type: "pc"                # pc | android | printer
secret: "same-as-hub-secret"

mqtt:
  host: "192.168.1.100"          # hub's LAN IP
  password: "yourpassword"

agent:
  port: 7800
  transfer_port: 7801
  allowed_paths: ["/home", "/tmp"]
  allow_run_command: false
  allow_destructive: true
```

---

## Build

```bash
# Current platform
make build                  # → dist/agent

# All platforms (from one machine)
make build-all              # → dist/agent-linux-{amd64,arm64,arm}
                            #   dist/agent-windows-amd64.exe
                            #   dist/agent-macos-arm64

# Android (Termux) — arm64
cd agent && GOOS=linux GOARCH=arm64 go build -o ../dist/agent-android .
```

---

## Project Structure

```
.
├── hub/                    Python — central hub
│   ├── config.py           Config loading + validation
│   ├── logger.py           structlog setup
│   ├── main.py             Entry point
│   ├── mqtt_client.py      Mosquitto connection (Phase 2)
│   ├── registry.py         Connected device tracking (Phase 2)
│   ├── nlp.py              Claude NLP router (Phase 5)
│   └── transfer.py         File transfer coordinator (Phase 6)
├── agent/                  Go — device agent
│   ├── main.go             Entry point
│   ├── config/             Config loading + validation
│   ├── logger/             slog setup
│   ├── mqtt/               MQTT client (Phase 2)
│   ├── tools/              Filesystem, system, Android tools (Phase 3)
│   ├── mcp/                Local MCP server (Phase 4)
│   └── transfer/           File transfer HTTP server (Phase 6)
├── cli/                    Python — interactive terminal (Phase 8)
├── config/                 Example config files
├── deploy/                 Mosquitto config, systemd units, install script
├── tests/                  Unit + integration tests (Phase 9)
├── Makefile
├── requirements.txt
└── README.md
```

---

## Development Status

| Phase | Description | Status |
|---|---|---|
| 1 | Foundation — config, logging, scaffolding | ✅ Done |
| 2 | Hub MQTT client + device registry | 🔲 Next |
| 3 | Go agent MQTT + tool execution engine | 🔲 |
| 4 | Local MCP server per device | 🔲 |
| 5 | NLP router (Claude) | 🔲 |
| 6 | File transfer protocol | 🔲 |
| 7 | Android agent (Termux) | 🔲 |
| 8 | Interactive CLI | 🔲 |
| 9 | Test suite | 🔲 |
| 10 | Production hardening (auth, TLS, logging) | 🔲 |
| 11 | Deployment (systemd, Docker, install script) | 🔲 |

---

## Connecting Claude Desktop Directly to a Device

Each agent runs a local MCP server on port 7800. Add it to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "my-laptop": {
      "url": "http://192.168.1.10:7800"
    },
    "my-phone": {
      "url": "http://192.168.1.20:7800"
    }
  }
}
```

---

## Requirements

- **Hub machine**: Python 3.12+, Mosquitto 2.x
- **Device agents**: Linux/macOS/Windows (Go 1.23+ to build), or download a pre-built binary
- **Android**: Termux + `termux-api` package for phone-specific tools (calls, SMS, camera)
- **Claude API key**: Required for NLP routing — [get one here](https://console.anthropic.com)
