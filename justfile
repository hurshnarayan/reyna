set dotenv-load := true
set shell := ["bash", "-uc"]

# Reyna (Web + Baileys Bot) task runner

default:
    @just --list

# Check environment toolchain, configuration, and services
doctor:
    #!/usr/bin/env bash
    ok() { printf "  \033[32mok\033[0m    %s\n" "$1"; }
    no() { printf "  \033[31mFAIL\033[0m  %s\n" "$1"; }
    echo "Toolchain:"
    command -v go >/dev/null && ok "Go $(go version | awk '{print $3}')" || no "Go"
    command -v node >/dev/null && ok "Node $(node -v)" || no "Node"
    command -v npm >/dev/null && ok "npm $(npm -v)" || no "npm"
    command -v just >/dev/null && ok "just $(just --version | awk '{print $2}')" || no "just"
    echo "Configuration:"
    [ -f .env ] && ok ".env exists" || no ".env missing (run: just env-init)"
    [ -n "${DEVICE_TOKEN:-}" ] && ok "DEVICE_TOKEN is set" || no "DEVICE_TOKEN is empty in .env"
    [ -d web/node_modules ] && ok "web/node_modules installed" || no "web deps missing (run: just install)"
    [ -d whatsapp-bot/node_modules ] && ok "whatsapp-bot/node_modules installed" || no "bot deps missing (run: just install)"
    echo "Status:"
    curl -sf -m 2 http://localhost:8080/api/health >/dev/null && ok "Backend running on :8080" || echo "  info  Backend not running (run: just backend)"
    curl -sf -m 2 http://localhost:5173 >/dev/null && ok "Frontend running on :5173" || echo "  info  Frontend dev server not running (run: just frontend)"

# Initialize .env with generated secrets if not present
env-init:
    #!/usr/bin/env bash
    if [ -f .env ]; then
        echo ".env already exists."
    else
        sed -e "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" \
            -e "s|^DEVICE_TOKEN=.*|DEVICE_TOKEN=$(openssl rand -hex 32)|" \
            .env.example > .env
        echo "Created .env with generated JWT_SECRET and DEVICE_TOKEN."
    fi

# Install dependencies for web frontend and WhatsApp bot
install:
    cd web && npm install
    cd whatsapp-bot && npm install

# Build backend binary and frontend static assets
build:
    go build -o reyna-server ./cmd/server/
    cd web && npm run build

# Start Go backend server on :8080
backend:
    go run ./cmd/server/

# Start React Vite frontend dev server on :5173
frontend:
    cd web && npm run dev

# Start WhatsApp bot (Baileys)
bot:
    cd whatsapp-bot && node bot.js

# Instant fuzzy document retrieval via fzf (Raycast-style)
find *args:
    ./scripts/reyna-find {{args}}

# Wipe SQLite database and start fresh backend
fresh:
    rm -f reyna.db reyna.db-shm reyna.db-wal
    go run ./cmd/server/

# Clean database, drive local storage, and bot auth state
clean:
    rm -f reyna.db reyna.db-shm reyna.db-wal
    rm -rf drive_storage
    rm -rf whatsapp-bot/auth_state
    @echo "Cleaned database, drive storage, and bot auth state."

# Print instructions for running all services
dev:
    @echo ""
    @echo "Reyna consists of 3 services. Run these in separate terminals:"
    @echo ""
    @echo "  Terminal 1: just backend    # Go API on http://localhost:8080"
    @echo "  Terminal 2: just frontend   # React dev server on http://localhost:5173"
    @echo "  Terminal 3: just bot        # WhatsApp Baileys bot"
    @echo ""
