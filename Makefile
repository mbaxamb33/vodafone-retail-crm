.PHONY: dev build test lint

dev:
	@trap 'kill 0' INT TERM EXIT; (cd backend && DEMO_MODE=true go run ./cmd/server) & npm run dev & wait

build:
	npm run build
	cd backend && go build -o /tmp/vodafone-server ./cmd/server

test:
	cd backend && go test -race ./...
	npm test

lint:
	npm run lint
	cd backend && go vet ./...
	@test -z "$$(gofmt -l backend)"
