.PHONY: backend frontend bot install dev clean

# Load env vars
ENV_CMD = source <(grep -v '^\#' .env | sed 's/^/export /')

backend:
	cd backend && $(ENV_CMD) && go run ./cmd/server/

frontend:
	cd frontend && npm run dev

bot:
	cd whatsapp-bot && BACKEND_URL=http://localhost:8080 node bot.js

install:
	cd frontend && npm install
	cd whatsapp-bot && npm install

dev:
	@echo "Run these in separate terminals:"
	@echo "  make backend"
	@echo "  make frontend"
	@echo "  make bot"

clean:
	rm -f backend/reyna.db
	rm -rf backend/drive_storage
	rm -rf whatsapp-bot/auth_state
	@echo "Cleaned database, drive storage, and bot auth."
