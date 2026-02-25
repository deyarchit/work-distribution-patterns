# Work Distribution Patterns

A project exploring various work-distribution patterns with progressively increasing scalability and decoupling.

## Patterns

| Pattern | Topology | Communication Style | Full-Stack Layering |
|---|---|---|---|
| **p01: Local-Channels** | Single process | In-process channels | Embedded Monolith |
| **p02: Pull-REST** | 1 API + N workers | HTTP Long-polling | Tiered Remote Polling |
| **p03: Push-WebSocket** | 1 API + N workers | Persistent WebSockets | Tiered Remote Push |
| **p04: Streaming-gRPC** | 1 API + N workers | gRPC Bidirectional Streams | Tiered Remote Streaming |
| **p05: Brokered-NATS** | N APIs + N workers | NATS + PostgreSQL | Distributed Event-Driven |

All patterns expose an **identical HTTP API** and **identical HTMX frontend**. Only the internal dispatch mechanism and layering changes.

## Prerequisites

### Runtime Dependencies (Required)

All patterns require:
- **Go 1.25+**
- **Docker** and **Docker Compose** (for patterns 2-5)

> **Note:** Pattern 4 uses gRPC, but the generated protobuf code is **already checked into the repository** (`patterns/p04/proto/*.pb.go`). You do **not** need to install protoc or any code generators unless you plan to modify the `.proto` file itself.

### Development Dependencies (Optional)

**Only needed if modifying `patterns/p04/proto/work.proto`:**

- **protoc** (Protocol Buffers compiler)
- **protoc-gen-go** (Go protobuf code generator)
- **protoc-gen-go-grpc** (Go gRPC code generator)

#### Installing Protobuf Toolchain (Development Only)

**On macOS (via Homebrew):**
```bash
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

**Verify Installation:**
```bash
protoc --version        # Should show libprotoc 3.x or higher
which protoc-gen-go     # Should be in $GOPATH/bin or ~/go/bin
which protoc-gen-go-grpc
```

**Note:** Ensure `$GOPATH/bin` (or `~/go/bin`) is in your `$PATH`:
```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

#### Regenerating Protobuf Code

After modifying `patterns/p04/proto/work.proto`:
```bash
make gen-proto
```

## Quick Start

### Pattern 1: Local-Channels (no Docker needed)

```bash
make run-p1
# open http://localhost:8080
```

### Pattern 2: Pull-REST (Docker)

```bash
make run-p2
# open http://localhost:8080
```

### Pattern 3: Push-WebSocket (Docker)

```bash
make run-p3
# open http://localhost:8080
```

### Pattern 4: Streaming-gRPC (Docker)

```bash
make run-p4
# open http://localhost:8080
```

### Pattern 5: Brokered-NATS (Docker)

```bash
make run-p5
# open http://localhost:8080
```

## Architecture

```
Browser
  │  HTTP POST /tasks, GET /tasks, GET /tasks/{id}
  │  SSE  GET /events
  ▼
┌─────────────────────────────────────────────────────┐
│  shared/api  (HTTP handlers + SSE — 100% shared)    │
│  POST /tasks → manager.Submit(task)                 │
│  GET  /events → sse.Hub.Subscribe()                 │
└──────────────────┬──────────────────────────────────┘
                   │ contracts.TaskDispatcher interface
        ┌──────────┴─────────────────────────────────┐
        │                                            │
      p01                                 p02 / p03 / p04 / p05
  ChannelDispatcher              REST/WS/gRPC/NATSDispatcher
  (in-process)                    (routes to external workers)
```

## Testing

```bash
# E2E tests (requires a running server)
BASE_URL=http://localhost:8080 make test-e2e

# Load test
BASE_URL=http://localhost:8080 make test-load

# Build all binaries
make build-all

# Run all patterns end-to-end
make test-all
```

## Project Structure

```
shared/
├── api/          All HTTP handlers (shared across all patterns)
├── manager/      Unified task lifecycle management
├── contracts/    Interfaces (TaskDispatcher, TaskConsumer, TaskManager)
├── models/       Task, Stage, TaskEvent data types
├── executor/     Stage runner (worker-side logic)
├── store/        Persistence (Memory, PostgreSQL)
├── events/       Event Bus (Memory, NATS)
├── client/       Remote proxy for API-to-Manager communication
├── sse/          SSE hub — broadcaster to browser connections
└── templates/    Embedded HTMX frontend

patterns/
├── p01/          Local-Channels: Bounded goroutine pool (in-process)
├── p02/          Pull-REST: REST-based worker polling
├── p03/          Push-WebSocket: WebSocket dispatch to external workers
├── p04/          Streaming-gRPC: gRPC bidirectional streams with protobuf
└── p05/          Brokered-NATS: NATS JetStream (queue) + PostgreSQL (store)
```
