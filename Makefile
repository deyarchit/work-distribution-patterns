SKILL_NAME = update-codemaps
BASE_URL ?= http://localhost:8080

## Generate protobuf code for Pattern 4
gen-proto:
	@echo "==> Generating protobuf code for Pattern 4"
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		patterns/p04/proto/work.proto

## Run Pattern 1 locally (no Docker)
run-p1:
	go run ./patterns/p01/cmd/server/

## Run Pattern 2 with Docker Compose (1 manager + 1 API + 3 workers)
run-p2:
	docker compose -f patterns/p02/docker-compose.yml up --build

stop-p2:
	docker compose -f patterns/p02/docker-compose.yml down -v

## Run Pattern 3 with Docker Compose (1 manager + 1 API + 3 workers)
run-p3:
	docker compose -f patterns/p03/docker-compose.yml up --build

stop-p3:
	docker compose -f patterns/p03/docker-compose.yml down -v

## Run Pattern 4 with Docker Compose (1 manager + 1 API + 3 workers)
run-p4:
	docker compose -f patterns/p04/docker-compose.yml up --build

stop-p4:
	docker compose -f patterns/p04/docker-compose.yml down -v

## Run Pattern 5 with Docker Compose (1 manager + 3 APIs + 3 workers + NATS + postgres + nginx)
run-p5:
	docker compose -f patterns/p05/docker-compose.yml up --build

stop-p5:
	docker compose -f patterns/p05/docker-compose.yml down -v

## Run Pattern 6 with Docker Compose (1 manager + 3 APIs + 3 workers + broker + nginx)
## Use BROKER=nats (default) or BROKER=kafka
BROKER ?= nats
run-p6:
	@echo "Starting Pattern 6 with broker: $(BROKER)"
	docker compose -p p06-$(BROKER) -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.$(BROKER).yml up --build

stop-p6:
	@echo "Stopping Pattern 6 with broker: $(BROKER)"
	docker compose -p p06-$(BROKER) -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.$(BROKER).yml down -v

## Run in-process integration tests for all patterns (generates cover.out + cover.html)
test:
	go test ./patterns/... -timeout 300s -coverprofile=coverage.txt -coverpkg=./patterns/...,./shared/...
	go tool cover -html=coverage.txt -o coverage.html

## Run E2E tests against BASE_URL (default http://localhost:8080)
test-e2e:
	go clean -testcache && BASE_URL=$(BASE_URL) go test ./tests/e2e/... -v -timeout 120s

## Run load test against BASE_URL
test-load:
	go run ./tests/load/load.go -url $(BASE_URL)

## Build all binaries
build-all:
	go build -o bin/p1-server    ./patterns/p01/cmd/server
	go build -o bin/p2-api       ./patterns/p02/cmd/api
	go build -o bin/p2-manager   ./patterns/p02/cmd/manager
	go build -o bin/p2-worker    ./patterns/p02/cmd/worker
	go build -o bin/p3-api       ./patterns/p03/cmd/api
	go build -o bin/p3-manager   ./patterns/p03/cmd/manager
	go build -o bin/p3-worker    ./patterns/p03/cmd/worker
	go build -o bin/p4-api       ./patterns/p04/cmd/api
	go build -o bin/p4-manager   ./patterns/p04/cmd/manager
	go build -o bin/p4-worker    ./patterns/p04/cmd/worker
	go build -o bin/p5-api       ./patterns/p05/cmd/api
	go build -o bin/p5-manager   ./patterns/p05/cmd/manager
	go build -o bin/p5-worker    ./patterns/p05/cmd/worker
	go build -o bin/p06-api      ./patterns/p06/cmd/api
	go build -o bin/p06-manager  ./patterns/p06/cmd/manager
	go build -o bin/p06-worker   ./patterns/p06/cmd/worker

## Build all binaries and validate all six patterns end-to-end via Docker Compose
test-integration: build-all
	@echo "==> [1/6] Pattern 1: Local-Channels"
	@{ \
	  ./bin/p1-server & \
	  until curl -sf http://localhost:8080/tasks > /dev/null 2>&1; do sleep 1; done; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  pkill -f "bin/p1-server" 2>/dev/null || true; \
	  exit $$RC; \
	}
	@echo "==> [2/6] Pattern 2: Pull-REST"
	@echo "    Building containers..."
	@{ \
	  docker compose -f patterns/p02/docker-compose.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/p02/docker-compose.yml down -v > /dev/null 2>&1; \
	  exit $$RC; \
	}
	@echo "==> [3/6] Pattern 3: Push-WebSocket"
	@echo "    Building containers..."
	@{ \
	  docker compose -f patterns/p03/docker-compose.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/p03/docker-compose.yml down -v > /dev/null 2>&1; \
	  exit $$RC; \
	}
	@echo "==> [4/6] Pattern 4: Streaming-gRPC"
	@echo "    Building containers..."
	@{ \
	  docker compose -f patterns/p04/docker-compose.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/p04/docker-compose.yml down -v > /dev/null 2>&1; \
	  exit $$RC; \
	}
	@echo "==> [5/6] Pattern 5: Brokered-NATS"
	@echo "    Building containers..."
	@{ \
	  docker compose -f patterns/p05/docker-compose.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -f patterns/p05/docker-compose.yml down -v > /dev/null 2>&1; \
	  exit $$RC; \
	}
	@echo "==> [6/7] Pattern 6: Cloud-Agnostic (NATS)"
	@echo "    Building containers..."
	@{ \
	  docker compose -p p06-nats -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.nats.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -p p06-nats -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.nats.yml down -v > /dev/null 2>&1; \
	  exit $$RC; \
	}
	@echo "==> [7/7] Pattern 6: Cloud-Agnostic (Kafka)"
	@echo "    Building containers..."
	@{ \
	  docker compose -p p06-kafka -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.kafka.yml up --build -d --wait --quiet-pull > .docker_build.log 2>&1 || { cat .docker_build.log; rm .docker_build.log; exit 1; }; \
	  rm .docker_build.log; \
	  BASE_URL=$(BASE_URL) $(MAKE) test-e2e; RC=$$?; \
	  docker compose -p p06-kafka -f patterns/p06/docker-compose.base.yml -f patterns/p06/docker-compose.kafka.yml down -v > /dev/null 2>&1; \
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

## Update codemaps
update-codemaps:
	@echo "Running repomix to generate static codemap"
	npx repomix@latest
	@echo "Launching Claude Code to run skill: $(SKILL_NAME)..."
	claude -p "run the $(SKILL_NAME) skill" --model "haiku" --allowedTools "Bash,Read,Edit,Write" --output-format stream-json --verbose --include-partial-messages | \
  jq -rj 'select(.type == "stream_event" and .event.delta.type? == "text_delta") | .event.delta.text'
	@echo "Updated codemaps"


## Prepare for pull request
pr: tidy lint fmt test

.PHONY: *
