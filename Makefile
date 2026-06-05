.PHONY: help init build test check run sync-env docker-build up down logs deploy-check http-test live-test push tidy clean

GREEN=\033[0;32m
NC=\033[0m

BIN=./bin/k-ai
CONFIG?=./config/config.yaml

help: ## Show make targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "$(GREEN)%-18s$(NC) %s\n", $$1, $$2}'

init: ## Init dev env (.env, override compose, dirs)
	@echo "$(GREEN)Initializing k-ai dev environment...$(NC)"
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@if [ ! -f docker-compose.override.yml ]; then \
		cp docker-compose.override.example.yml docker-compose.override.yml; \
	fi
	@mkdir -p data bin
	@chmod +x scripts/*.sh 2>/dev/null || true
	@echo "$(GREEN)Ready. Edit .env then run 'make run' or 'make up'.$(NC)"

sync-env: ## Import provider keys from OpenCode/Hermes auth files
	@./scripts/sync-env-from-runtimes.sh

tidy:
	go mod tidy

build:
	go build -o $(BIN) ./cmd/k-ai

test:
	go test ./...

check: tidy build test ## Quality gate (static)

run: build ## Run native server
	@set -a; [ -f .env ] && . ./.env; set +a; \
	export K_AI_BASE_URL="$${K_AI_BASE_URL:-http://127.0.0.1:$${K_AI_PORT:-8080}}"; \
	$(BIN) -config $(CONFIG)

docker-build:
	docker compose build

up: ## Docker compose up (dev/prod)
	docker compose up -d --build

down: ## Docker compose down
	docker compose down

logs: ## Follow container logs
	docker compose logs -f k-ai

deploy-check: docker-build ## Verify Docker image builds
	@echo "$(GREEN)Docker image built successfully$(NC)"

http-test: ## curl smoke tests (server must run)
	@./scripts/http-test.sh

live-test: ## Full live smoke (starts server on :18080)
	@./scripts/live-test.sh

realtime-test: ## Realtime matrix (server must run on :18090 or set K_AI_BASE_URL)
	@./scripts/realtime-matrix.sh

push: ## Push to gitlab remote
	@echo "--> Pushing to gitlab..."
	git push gitlab --all
	git push gitlab --tags

clean:
	rm -rf bin data/*.db
