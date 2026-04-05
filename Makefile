APP ?= app

.PHONY: up down seed test swagger loadtest

up:
	docker compose up -d --build

down:
	docker compose down

seed:
	docker compose exec -T db psql -U meeting_user -d meeting_booking < deploy/postgres/seed.sql

test:
	go test ./... -cover

swagger:
	go run github.com/swaggo/swag/cmd/swag@latest init -g cmd/server/main.go -o docs

loadtest:
	go run ./loadtest/loadtest.go
