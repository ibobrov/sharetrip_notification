.DEFAULT_GOAL := help

EXE :=
ifeq ($(OS),Windows_NT)
EXE := .exe
endif

export GOBIN := $(CURDIR)/.tools

LINT_VERSION := v2.11.3
GOOSE_VERSION := v3.28.0

LINT := $(GOBIN)/golangci-lint$(EXE)
GOOSE := $(GOBIN)/goose$(EXE)

COMPOSE := docker compose --env-file .env -f deploy/docker-compose.yml

.PHONY: help deps tidy fmt lint test build run up down
.PHONY: migrate-create migrate-status migrate-up migrate-down
.PHONY: lint-install goose-install migrations-dir

help:
	@echo Available commands:
	@echo   make deps               - Download modules and install pinned CLI tools
	@echo   make tidy               - Update go.mod/go.sum and remove unused requirements
	@echo   make fmt                - Format Go code
	@echo   make lint               - Run linters
	@echo   make test               - Run all tests
	@echo   make build              - Build bin/notification
	@echo   make run                - Run application
	@echo   make up                 - Start local PostgreSQL and wait for health
	@echo   make down               - Stop PostgreSQL and preserve its volume
	@echo   make migrate-create name=create_notifications - Create SQL migration
	@echo   make migrate-status     - Show migration status
	@echo   make migrate-up         - Apply migrations
	@echo   make migrate-down       - Roll back last migration with possible data loss
	@echo   make lint-install       - Install pinned golangci-lint
	@echo   make goose-install      - Install pinned Goose CLI
	@echo   make migrations-dir     - Create migrations directory
	@echo   make help               - Show available commands

deps: lint-install goose-install
	go mod download

lint-install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION)
	"$(LINT)" version

goose-install:
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	"$(GOOSE)" -version

tidy:
	go mod tidy

fmt:
	go fmt ./...

lint:
	"$(LINT)" run ./...

test:
	go test ./...

build:
	go build -o bin/notification$(EXE) ./cmd/notification

run:
	go run ./cmd/notification

up:
	$(COMPOSE) up -d --wait --wait-timeout 90

down:
	$(COMPOSE) down

migrate-create: migrations-dir
	"$(GOOSE)" -env none -dir migrations create "$(if $(strip $(name)),$(strip $(name)),name)" sql

migrate-status:
	"$(GOOSE)" -env .env -dir migrations status

migrate-up:
	"$(GOOSE)" -env .env -dir migrations up

migrate-down:
	"$(GOOSE)" -env .env -dir migrations down

migrations-dir:
ifeq ($(OS),Windows_NT)
	powershell.exe -NoProfile -Command "New-Item -ItemType Directory -Force -Path 'migrations' | Out-Null"
else
	mkdir -p migrations
endif
