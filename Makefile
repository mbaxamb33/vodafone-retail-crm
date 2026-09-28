# Local development database started by `make db-up` (see docker-compose.yml).
DATABASE_URL ?= postgres://crm:crm@127.0.0.1:5434/crm?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)

.PHONY: dev db-up db-down db-reset migrate seed build test lint

dev: db-up
	@trap 'kill 0' INT TERM EXIT; (cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/server) & npm run dev & wait

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

# Deletes all local data, then recreates the schema and demo store.
db-reset:
	docker compose down -v
	$(MAKE) seed

migrate: db-up
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/crmctl migrate

seed: db-up
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/crmctl seed-demo

build:
	npm run build
	cd backend && go build -o bin/server ./cmd/server && go build -o bin/crmctl ./cmd/crmctl

# Database tests run in throwaway schemas; run `make db-up` first locally.
test:
	cd backend && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race ./...
	npm test

lint:
	npm run lint
	cd backend && go vet ./...
	@test -z "$$(gofmt -l backend)"
