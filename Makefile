.PHONY: all dev backend frontend install build clean reset-data help

# ── Config ────────────────────────────────────────────────────────────────────
BACKEND_DIR  := backend
FRONTEND_DIR := frontend
BINARY       := ticket-store

PORT    ?= 8080
HOST    ?=
REPO    ?= ./ticket-data
CORS    ?= http://localhost:5173,http://localhost:4173,http://localhost:3000
VERBOSE ?= false

# ── Default ───────────────────────────────────────────────────────────────────
all: help

# ── Dev ───────────────────────────────────────────────────────────────────────
dev: install
	@echo ""
	@echo "  Starting Ticket Store"
	@echo "  Backend  → http://$(or $(HOST),localhost):$(PORT)"
	@echo "  Frontend → http://localhost:5173"
	@echo ""
	@trap 'kill 0' SIGINT; \
		$(MAKE) --no-print-directory _backend & \
		$(MAKE) --no-print-directory _frontend & \
		wait

_backend:
	@if [ -f "./$(BINARY)" ]; then \
		./$(BINARY) \
			-port $(PORT) \
			$(if $(HOST),-host $(HOST)) \
			-repo $(REPO) \
			-cors "$(CORS)" \
			$(if $(filter true,$(VERBOSE)),-verbose); \
	else \
		echo "  (tip: run 'make backend-build' first for a stable Git merge driver path)"; \
		cd $(BACKEND_DIR) && go run main.go \
			-port $(PORT) \
			$(if $(HOST),-host $(HOST)) \
			-repo ../$(REPO) \
			-cors "$(CORS)" \
			$(if $(filter true,$(VERBOSE)),-verbose); \
	fi

_frontend:
	cd $(FRONTEND_DIR) && npm run dev

# ── Backend ───────────────────────────────────────────────────────────────────
backend:
	cd $(BACKEND_DIR) && go mod tidy && go run main.go \
		-port $(PORT) \
		$(if $(HOST),-host $(HOST)) \
		-repo ../$(REPO) \
		-cors "$(CORS)"

backend-build:
	cd $(BACKEND_DIR) && go mod tidy && go build -o ../$(BINARY) .
	@echo "Binary: ./$(BINARY)"
	@echo ""
	@echo "  NOTE: start with './$(BINARY)' (not 'go run') so the Git merge"
	@echo "  driver is registered with a stable, persistent binary path."

backend-run: backend-build
	./$(BINARY) -port $(PORT) -repo $(REPO) -cors "$(CORS)"

# Verify the Git merge driver is wired up correctly in the repo
check-merge-driver:
	@echo "── .gitattributes ──────────────────────────────────────"
	@cat $(REPO)/.gitattributes 2>/dev/null || echo "(not found)"
	@echo ""
	@echo "── .git/config merge section ───────────────────────────"
	@git -C $(REPO) config --list 2>/dev/null | grep "merge.ticket-crdt" || echo "(not configured)"
	@echo ""
	@echo "── test invocation ─────────────────────────────────────"
	@$(BINARY) merge-driver --help 2>&1 | head -3 || true

# ── Frontend ──────────────────────────────────────────────────────────────────
frontend:
	cd $(FRONTEND_DIR) && npm run dev

frontend-build:
	cd $(FRONTEND_DIR) && npm run build
	@echo "Production build: $(FRONTEND_DIR)/dist/"

frontend-preview: frontend-build
	cd $(FRONTEND_DIR) && npm run preview

# ── Install ───────────────────────────────────────────────────────────────────
install: _go-tidy _npm-install

_go-tidy:
	cd $(BACKEND_DIR) && go mod tidy

_npm-install:
	cd $(FRONTEND_DIR) && npm install

# ── Build all ─────────────────────────────────────────────────────────────────
build: backend-build frontend-build

# ── Cleanup ───────────────────────────────────────────────────────────────────
clean:
	rm -f $(BINARY)
	rm -rf $(FRONTEND_DIR)/dist
	cd $(BACKEND_DIR) && go clean

reset-data:
	@read -p "Delete all ticket data in '$(REPO)'? [y/N] " ans; \
		[ "$$ans" = "y" ] && rm -rf $(REPO) && echo "Cleared." || echo "Aborted."

# ── Help ──────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  Ticket Store"
	@echo ""
	@echo "  Usage:"
	@echo "    make dev                    Start backend + frontend (watches for changes)"
	@echo "    make backend                Run backend only (go run)"
	@echo "    make backend-build          Compile ./$(BINARY) binary"
	@echo "    make backend-run            Compile then run binary"
	@echo "    make frontend               Run Vite dev server only"
	@echo "    make frontend-build         Production build  → frontend/dist/"
	@echo "    make install                go mod tidy + npm install"
	@echo "    make build                  Build binary + production frontend"
	@echo "    make clean                  Remove build artefacts"
	@echo "    make reset-data             Delete ticket data (prompts for confirmation)"
	@echo "    make check-merge-driver     Show merge driver config in the repo"
	@echo ""
	@echo "  Variables:"
	@echo "    PORT=$(PORT)         Backend HTTP port"
	@echo "    HOST=$(HOST)         Bind host (default: all interfaces)"
	@echo "    REPO=$(REPO)  Git repo path"
	@echo "    CORS=$(CORS)"
	@echo "    VERBOSE=$(VERBOSE)      Log every request"
	@echo ""
	@echo "  Examples:"
	@echo "    make dev"
	@echo "    make dev PORT=9090 REPO=/data/tickets"
	@echo "    make dev HOST=0.0.0.0 CORS='https://app.example.com'"
	@echo "    make backend-run PORT=9090 VERBOSE=true"
	@echo "    make backend -- -port 9090 -verbose    (pass flags directly)"
	@echo ""
