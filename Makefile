.PHONY: run-p1 run-p2 stop-p2 run-p3 stop-p3 run-p4 stop-p4 test-e2e test-load test-all build-all tidy lint fmt

BASE_URL ?= http://localhost:8080

## Run Pattern 1 locally (no Docker)
run-p1:
	go run ./patterns/01-goroutine-pool/cmd/server/

## Run Pattern 2 with Docker Compose (1 manager + 1 API + 3 workers)
run-p2:
	docker compose -f patterns/02-rest-polling/docker-compose.yml up --build

stop-p2:
	docker compose -f patterns/02-rest-polling/docker-compose.yml down

## Run Pattern 3 with Docker Compose (1 manager + 1 API + 3 workers)
run-p3:
	docker compose -f patterns/03-websocket-hub/docker-compose.yml up --build

stop-p3:
	docker compose -f patterns/03-websocket-hub/docker-compose.yml down

## Run Pattern 4 with Docker Compose (1 manager + 3 APIs + 3 workers + NATS + postgres + nginx)
run-p4:
	docker compose -f patterns/04-queue-and-store/docker-compose.yml up --build

stop-p4:
	docker compose -f patterns/04-queue-and-store/docker-compose.yml down

## Run E2E tests against BASE_URL (default http://localhost:8080)
test-e2e:
	go clean -testcache && BASE_URL=$(BASE_URL) go test ./tests/e2e/... -v -timeout 120s

## Run load test against BASE_URL
test-load:
	go run ./tests/load/load.go -url $(BASE_URL)

## Build all binaries
build-all:
	go build -o bin/p1-server    ./patterns/01-goroutine-pool/cmd/server
	go build -o bin/p2-api       ./patterns/02-rest-polling/cmd/api
	go build -o bin/p2-manager   ./patterns/02-rest-polling/cmd/manager
	go build -o bin/p2-worker    ./patterns/02-rest-polling/cmd/worker
	go build -o bin/p3-api       ./patterns/03-websocket-hub/cmd/api
	go build -o bin/p3-manager   ./patterns/03-websocket-hub/cmd/manager
	go build -o bin/p3-worker    ./patterns/03-websocket-hub/cmd/worker
	go build -o bin/p4-api       ./patterns/04-queue-and-store/cmd/api
	go build -o bin/p4-manager   ./patterns/04-queue-and-store/cmd/manager
	go build -o bin/p4-worker    ./patterns/04-queue-and-store/cmd/worker

## Build all binaries and validate all four patterns end-to-end
test-all: build-all
	@echo "==> [1/4] Pattern 1: goroutine-pool"
	@{ \
	  ./bin/p1-server & \
	  until curl -sf http://localhost:8080/tasks > /dev/null 2>&1; do sleep 1; done; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  pkill -f "bin/p1-server" 2>/dev/null || true; \
	  exit $$RC; \
	}
	@echo "==> [2/4] Pattern 2: rest-polling"
	@{ \
	  docker compose -f patterns/02-rest-polling/docker-compose.yml up --build -d --wait && \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/02-rest-polling/docker-compose.yml down; \
	  exit $$RC; \
	}
	@echo "==> [3/4] Pattern 3: websocket-hub"
	@{ \
	  docker compose -f patterns/03-websocket-hub/docker-compose.yml up --build -d --wait && \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/03-websocket-hub/docker-compose.yml down; \
	  exit $$RC; \
	}
	@echo "==> [4/4] Pattern 4: queue-and-store"
	@{ \
	  docker compose -f patterns/04-queue-and-store/docker-compose.yml up --build -d --wait && \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/04-queue-and-store/docker-compose.yml down; \
	  exit $$RC; \
	}

## Tidy modules
tidy:
	go mod tidy

## Lint with golangci-lint
lint:
	golangci-lint run ./...

## Format with golangci-lint (auto-fix formatting)
fmt:
	golangci-lint fmt ./...


## Update repomap
update-repomap:
	npx repomix@latest