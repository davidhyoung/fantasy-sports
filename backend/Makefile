.PHONY: run build test migrate-up migrate-down generate import-nfl project-nfl backtest-nfl autotune-nfl

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

migrate-up:
	@export $$(grep -v '^#' ../.env | xargs) 2>/dev/null; \
	migrate -path migrations -database "$${DATABASE_URL}?sslmode=disable" up

migrate-down:
	@export $$(grep -v '^#' ../.env | xargs) 2>/dev/null; \
	migrate -path migrations -database "$${DATABASE_URL}?sslmode=disable" down

generate:
	sqlc generate

import-nfl:
	go run ./cmd/import $(ARGS)

project-nfl:
	go run ./cmd/projections $(ARGS)

backtest-nfl:
	go run ./cmd/projections -backtest $(ARGS)

autotune-nfl:
	go run ./cmd/projections -autotune $(ARGS)
