# Detect Go binary: prefer system go, fall back to toolchain in module cache
GO ?= $(shell which go 2>/dev/null || echo /Users/parham/Projects/GO/Samples/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.darwin-arm64/bin/go)

.PHONY: dev infra stop logs migrate seed tunnel run-server build-server build-web lint-server lint-web test

# ── Local dev ──────────────────────────────────────────────────────────────────
infra:
	docker compose up -d postgres redis

stop:
	docker compose down

logs:
	docker compose logs -f

# ── Server (Go) ────────────────────────────────────────────────────────────────
migrate:
	cd server && $(GO) run ./cmd/migrate

seed:
	cd server && $(GO) run ./cmd/seed

# ── Cashback DB SSH tunnel (local only) ────────────────────────────────────────
# Forwards stg-offline-01 (172.16.63.99):5432 to 127.0.0.1:55432 so the server
# can reach the staging cashback DB from a machine outside the internal network.
# Idempotent: does nothing if the tunnel is already listening.
tunnel:
	@if nc -z -w 2 127.0.0.1 55432 2>/dev/null; then \
		echo "cashback SSH tunnel already up (127.0.0.1:55432)"; \
	else \
		echo "starting cashback SSH tunnel -> 172.16.63.99:5432"; \
		(nohup ssh -o ConnectTimeout=10 -o ExitOnForwardFailure=yes \
			-N -L 55432:172.16.63.99:5432 stg-offline-01 \
			>/tmp/nexus-ssh-tunnel.log 2>&1 &); \
		sleep 3; \
		if nc -z -w 2 127.0.0.1 55432 2>/dev/null; then \
			echo "tunnel up"; \
		else \
			echo "tunnel failed — see /tmp/nexus-ssh-tunnel.log"; \
			exit 1; \
		fi; \
	fi

run-server: tunnel
	cd server && $(GO) run ./cmd/nexus

build-server:
	cd server && $(GO) build -o ../bin/nexus ./cmd/nexus

lint-server:
	cd server && golangci-lint run ./...

test-server:
	cd server && $(GO) test ./... -v

# ── Web (Next.js) ──────────────────────────────────────────────────────────────
install-web:
	cd web && npm install --legacy-peer-deps

run-web:
	cd web && npm run dev

build-web:
	cd web && npm run build

lint-web:
	cd web && npm run lint

# ── Full local dev (requires tmux or run in separate terminals) ────────────────
dev: infra
	@echo ""
	@echo "Infrastructure started. Run these in separate terminals:"
	@echo "  make run-server"
	@echo "  make run-web"
	@echo ""

# ── Setup from scratch ─────────────────────────────────────────────────────────
setup:
	@cp -n .env.example .env 2>/dev/null || true
	@cd server && $(GO) mod tidy
	@cd web && npm install --legacy-peer-deps
	@make infra
	@sleep 3
	@make migrate
	@make seed
	@echo ""
	@echo "✓ Nexus is ready. Run 'make run-server' in one terminal and 'make run-web' in another."
