.PHONY: help install-py run-hub run-agent build build-all test-py test-go lint clean setup-mosquitto

BASE := $(shell pwd)
DIST := $(BASE)/dist

help:
	@echo ""
	@echo "MCP Mesh — Available targets:"
	@echo ""
	@echo "  Setup"
	@echo "    install-py          Install Python dependencies (venv recommended)"
	@echo "    setup-mosquitto     Configure Mosquitto broker"
	@echo ""
	@echo "  Run"
	@echo "    run-hub             Start the central hub"
	@echo "    run-agent           Start a device agent (reads config/agent.yml)"
	@echo ""
	@echo "  Build (Go agent)"
	@echo "    build               Build agent for current platform -> dist/agent"
	@echo "    build-all           Cross-compile for all platforms"
	@echo ""
	@echo "  Test"
	@echo "    test-py             Run Python tests with coverage"
	@echo "    test-go             Run Go tests"
	@echo "    test                Run all tests"
	@echo ""
	@echo "  Other"
	@echo "    lint                Lint Python (ruff) and Go (vet)"
	@echo "    clean               Remove build artefacts"
	@echo ""

# ── Python ────────────────────────────────────────────────────────────────────

install-py:
	pip install -r requirements-dev.txt

run-hub:
	PYTHONPATH=$(BASE) python hub/main.py

# ── Go (all commands run from agent/ where go.mod lives) ──────────────────────

build:
	@mkdir -p $(DIST)
	cd agent && go build -o $(DIST)/agent .

build-all:
	@mkdir -p $(DIST)
	cd agent && GOOS=linux   GOARCH=amd64  go build -o $(DIST)/agent-linux-amd64  .
	cd agent && GOOS=linux   GOARCH=arm64  go build -o $(DIST)/agent-linux-arm64  .
	cd agent && GOOS=linux   GOARCH=arm    GOARM=7 go build -o $(DIST)/agent-linux-arm .
	cd agent && GOOS=windows GOARCH=amd64  go build -o $(DIST)/agent-windows-amd64.exe .
	cd agent && GOOS=darwin  GOARCH=arm64  go build -o $(DIST)/agent-macos-arm64  .
	@echo ""
	@ls -lh $(DIST)/
	@echo ""

run-agent:
	cd agent && go run . --config ../config/agent.yml

# ── Tests ─────────────────────────────────────────────────────────────────────

test-py:
	PYTHONPATH=$(BASE) pytest tests/ -v --cov=hub --cov=cli \
		--cov-report=term-missing --cov-report=html:htmlcov

test-go:
	cd agent && go test ./... -v -race

test: test-py test-go

# ── Quality ───────────────────────────────────────────────────────────────────

lint:
	ruff check hub/ cli/ tests/
	cd agent && go vet ./...

# ── Mosquitto ─────────────────────────────────────────────────────────────────

setup-mosquitto:
	@echo "Installing Mosquitto..."
	sudo apt-get install -y mosquitto mosquitto-clients
	sudo cp deploy/mosquitto.conf /etc/mosquitto/conf.d/mcp-mesh.conf
	@echo ""
	@echo "Create MQTT credentials:"
	@echo "  sudo mosquitto_passwd -c /etc/mosquitto/passwd meshuser"
	@echo "  sudo systemctl restart mosquitto"
	@echo ""

# ── Cleanup ───────────────────────────────────────────────────────────────────

clean:
	rm -rf dist/ __pycache__ .pytest_cache .coverage htmlcov
	find . -name "*.pyc" -delete
	find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
