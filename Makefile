export GOCACHE := $(PWD)/backend/.gocache

.PHONY: dev test build backend frontend

dev:
	docker-compose up --build

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm run dev

test:
	cd backend && GOCACHE=$(GOCACHE) go test ./...

build:
	docker-compose build
