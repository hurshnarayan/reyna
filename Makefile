.PHONY: backend frontend install dev fresh clean help

# ── .env loading ──
# Strips comments and blank lines, then exports each KEY=VALUE.
# Runs in /bin/sh regardless of the user's login shell (fish, zsh, bash).
# No `include .env` (Make's parser chokes on comments/blanks/spaces).
ENV_FILE := $(wildcard .env)
ifneq ($(ENV_FILE),)
  ENV_EXPORT := $(shell grep -v '^\#' .env | grep -v '^$$' | sed 's/^/export /' | tr '\n' ';')
endif

# ── Targets ──

help: ## Show this help
	@awk '/^[a-zA-Z_-]+:.*## /{split($$0,a,":.*## ");printf "  \033[36mmake %-10s\033[0m %s\n",a[1],a[2]}' $(MAKEFILE_LIST)

backend: ## Start the Go backend on :8080
	@$(ENV_EXPORT) go run ./cmd/server/

frontend: ## Start the React dev server on :5173
	cd web && npm run dev

install: ## Install frontend dependencies
	cd web && npm install

dev: ## Print instructions for running both services
	@echo ""
	@echo "  Run these in 2 separate terminals:"
	@echo ""
	@echo "    make backend     # Go API on :8080"
	@echo "    make frontend    # React on :5173"
	@echo ""
	@echo "  Or for a fresh start (clean DB + backend):"
	@echo ""
	@echo "    make fresh"
	@echo ""

fresh: ## Clean DB + start backend (your daily dev command)
	rm -f reyna.db reyna.db-shm reyna.db-wal
	@$(ENV_EXPORT) go run ./cmd/server/

clean: ## Remove database and drive storage
	rm -f reyna.db reyna.db-shm reyna.db-wal
	rm -rf drive_storage
	@echo "Cleaned database and drive storage."

build: ## Build the backend binary
	@$(ENV_EXPORT) go build -o reyna-server ./cmd/server/
	@echo "Built: ./reyna-server"
