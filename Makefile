.PHONY: run test test-race test-short cover build lint

run: ## Run the API locally (uses environment variables from .env)
	go run ./cmd/api

test: ## Run the full test suite
	go test ./...

test-race: ## Run the full test suite with the race detector
	go test -race ./...

test-short: ## Run only fast tests (skips DynamoDB integration tests)
	go test -short ./...

cover: ## Run tests with coverage report
	go test ./... -cover

build: ## Build the API binary
	go build -o bin/api ./cmd/api

lint: ## Run go vet as a lightweight static check
	go vet ./...
