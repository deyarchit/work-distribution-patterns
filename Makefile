.PHONY: run-p1 run-p2 run-p3 test-e2e test-load build-all tidy stop-p2 stop-p3

BASE_URL ?= http://localhost:8080

## Run Pattern 1 locally (no Docker)
run-p1:
	go run ./patterns/01-goroutine-pool/cmd/server/

## Run Pattern 2 with Docker Compose (1 API + 3 workers)
run-p2:
	docker compose -f patterns/02-websocket-hub/docker-compose.yml up --build

stop-p2:
	docker compose -f patterns/02-websocket-hub/docker-compose.yml down

## Run Pattern 3 with Docker Compose (3 APIs + 3 workers + NATS + nginx)
run-p3:
	docker compose -f patterns/03-nats-jetstream/docker-compose.yml up --build

stop-p3:
	docker compose -f patterns/03-nats-jetstream/docker-compose.yml down

## Run E2E tests against BASE_URL (default http://localhost:8080)
test-e2e:
	BASE_URL=$(BASE_URL) go test ./tests/e2e/... -v -timeout 120s

## Run load test against BASE_URL
test-load:
	go run ./tests/load/load.go -url $(BASE_URL)

## Build all five binaries
build-all:
	go build -o bin/p1-server  ./patterns/01-goroutine-pool/cmd/server
	go build -o bin/p2-api     ./patterns/02-websocket-hub/cmd/api
	go build -o bin/p2-worker  ./patterns/02-websocket-hub/cmd/worker
	go build -o bin/p3-api     ./patterns/03-nats-jetstream/cmd/api
	go build -o bin/p3-worker  ./patterns/03-nats-jetstream/cmd/worker

## Tidy modules
tidy:
	go mod tidy
