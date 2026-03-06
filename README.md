<div align="center">
  <a href="https://github.com/deyarchit/work-distribution-patterns/actions/workflows/go-test.yml"><img src="https://github.com/deyarchit/work-distribution-patterns/actions/workflows/go-test.yml/badge.svg" alt="go-test"></a>
  <a href="https://github.com/deyarchit/work-distribution-patterns/actions/workflows/golangci-lint.yml"><img src="https://github.com/deyarchit/work-distribution-patterns/actions/workflows/golangci-lint.yml/badge.svg" alt="golangci-lint"></a>
  <a href="https://codecov.io/gh/deyarchit/work-distribution-patterns"><img src="https://codecov.io/gh/deyarchit/work-distribution-patterns/graph/badge.svg?token=TS6BTA0J9J" alt="codecov"></a>
</div>


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
| **p06: Cloud-PubSub** | N APIs + N workers | gocloud.dev (NATS/Kafka/AWS) | Multi-Cloud Event-Driven |

All patterns expose an **identical HTTP API** and **identical HTMX frontend**. Only the internal dispatch mechanism and layering changes.

## Pattern Diagrams

### P1: Local-Channels (Single Process)
**Single process:** API, Manager, and Workers run as goroutines in one process.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["📦 Single Process"]
        subgraph L2a["Shared API Layer"]
            API["API Server<br/>(Echo)"]
            Hub["SSE Hub"]
        end

        subgraph L2b["Manager Layer"]
            Manager["Manager"]
        end

        subgraph L2c["Transport Layer ⚡ VARIATION POINT"]
            CD["ChannelDispatcher<br/>(unbuffered chan)"]
        end

        subgraph L2d["Worker Layer"]
            Workers["Worker Goroutines<br/>(pool)"]
        end

        subgraph L2e["Event Layer"]
            Bridge["MemoryBridge"]
        end
    end

    Browser -->|POST /tasks<br/>GET /events| API
    API -->|Submit/Get/List| Manager
    Manager -->|Dispatch| CD
    CD -->|events chan| Workers
    Workers -->|Emit| Bridge
    Bridge -->|Subscribe| Hub
    Hub -->|SSE events| Browser
```

### P2: Pull-REST (REST Polling)
**Separate processes:** API and Manager on different ports. Workers poll Manager for tasks.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["API Process :8080"]
        API["API Server<br/>(Echo)"]
        SSEHub["SSE Hub"]
    end

    subgraph L3["Manager Process :8081"]
        Manager["Manager"]
        subgraph L3a["Transport Layer ⚡ VARIATION POINT"]
            RD["RESTDispatcher<br/>(HTTP handlers)"]
        end
    end

    subgraph L4["Worker Processes :8082+"]
        Workers["Workers<br/>(polling)"]
    end

    subgraph L5["Event Layer"]
        EventBridge["MemoryBridge"]
    end

    Browser -->|POST /tasks<br/>GET /events| API
    API -->|HTTP RemoteTaskManager| Manager
    Manager -->|Task queue| RD
    Workers -->|GET /work/next<br/>POST /work/events| RD
    Manager -->|Emit events| EventBridge
    EventBridge -->|Subscribe| SSEHub
    SSEHub -->|SSE events| Browser
```

### P3: Push-WebSocket (WebSocket Hub)
**Separate processes with persistent connections:** Manager owns WebSocket hub, pushes tasks to workers.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["API Process :8080"]
        API["API Server<br/>(Echo)"]
        SSEHub["SSE Hub"]
    end

    subgraph L3["Manager Process :8081"]
        Manager["Manager"]
        subgraph L3a["Transport Layer ⚡ VARIATION POINT"]
            WD["WebSocketDispatcher<br/>(WS hub)"]
        end
    end

    subgraph L4["Worker Processes :8082+"]
        Workers["Workers ×3<br/>(persistent WS)"]
    end

    subgraph L5["Event Layer"]
        EventBridge["MemoryBridge"]
    end

    Browser -->|POST /tasks<br/>GET /events| API
    API -->|HTTP RemoteTaskManager| Manager
    Manager -->|Dispatch via| WD
    Workers -->|WS /ws/register<br/>receive tasks| WD
    Workers -->|Emit events| Manager
    Manager -->|Emit events| EventBridge
    EventBridge -->|Subscribe| SSEHub
    SSEHub -->|SSE events| Browser
```

### P4: Streaming-gRPC (gRPC Bidirectional)
**Separate processes with bidirectional streams:** Manager runs dual listeners (HTTP + gRPC) for high-performance streaming.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["API Process :8080"]
        API["API Server<br/>(Echo)"]
        SSEHub["SSE Hub"]
    end

    subgraph L3["Manager Process"]
        subgraph L3a[":8081 HTTP"]
            Manager["Manager"]
        end
        subgraph L3b[":9090 gRPC + Transport Layer ⚡ VARIATION POINT"]
            GD["gRPCDispatcher<br/>(bidirectional stream)"]
        end
    end

    subgraph L4["Worker Processes :8082+"]
        Workers["Workers ×N<br/>(gRPC stream)"]
    end

    subgraph L5["Event Layer"]
        EventBridge["MemoryBridge"]
    end

    Browser -->|POST /tasks<br/>GET /events| API
    API -->|HTTP RemoteTaskManager| Manager
    Manager -->|Dispatch via| GD
    Workers -->|gRPC bidirectional<br/>stream| GD
    Workers -->|Emit events| Manager
    Manager -->|Emit events| EventBridge
    EventBridge -->|Subscribe| SSEHub
    SSEHub -->|SSE events| Browser
```

### P5: Brokered-NATS (Distributed Event-Driven)
**Horizontally scaled:** Multiple API replicas, NATS JetStream for queuing, PostgreSQL for durability.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["API Layer :8080 ×N"]
        APIs["API Replicas<br/>(Echo)"]
        SSEHub["SSE Hub"]
    end

    subgraph L3["Manager Process :8081"]
        Manager["Manager<br/>(lifecycle, deadline)"]
    end

    subgraph L4["Transport Layer ⚡ VARIATION POINT"]
        NATS["🔥 NATS JetStream<br/>(durable queue)"]
    end

    subgraph L5["Worker Layer :8082+ ×N"]
        Workers["Workers<br/>(queue-subscribe)"]
    end

    subgraph L6["Persistence Layer"]
        EventBridge["NATSBridge<br/>(event routing)"]
        PG["🗄️ PostgreSQL<br/>(task state)"]
    end

    Browser -->|POST /tasks<br/>GET /events| APIs
    APIs -->|HTTP RemoteTaskManager| Manager
    Manager -->|Publish tasks.new| NATS
    Workers -->|Subscribe tasks.new| NATS
    Workers -->|Emit task.events.*| Manager
    Manager -->|Persist| PG
    Manager -->|Publish events| EventBridge
    EventBridge -->|Subscribe| SSEHub
    SSEHub -->|SSE events| Browser
```

### P6: Cloud-PubSub (Multi-Cloud Event-Driven)
**Cloud-agnostic abstraction:** Broker-agnostic via gocloud.dev (NATS/Kafka/AWS SNS-SQS), same distributed topology as P5.

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        Browser["Browser<br/>(HTTP/SSE)"]
    end

    subgraph L2["API Layer :8080 ×N"]
        APIs["API Replicas<br/>(Echo)"]
        SSEHub["SSE Hub"]
    end

    subgraph L3["Manager Process :8081"]
        Manager["Manager<br/>(lifecycle, deadline)"]
    end

    subgraph L4["Transport Layer ⚡ VARIATION POINT"]
        Broker["☁️ Broker Abstraction<br/>(gocloud.dev)<br/>NATS | Kafka | AWS SNS/SQS"]
    end

    subgraph L5["Worker Layer :8082+ ×N"]
        Workers["Workers<br/>(subscribe)"]
    end

    subgraph L6["Persistence Layer"]
        EventBridge["CloudBridge<br/>(event routing)"]
        PG["🗄️ PostgreSQL<br/>(task state)"]
    end

    Browser -->|POST /tasks<br/>GET /events| APIs
    APIs -->|HTTP RemoteTaskManager| Manager
    Manager -->|Publish tasks topic| Broker
    Workers -->|Subscribe tasks topic| Broker
    Workers -->|Emit events| Manager
    Manager -->|Persist| PG
    Manager -->|Publish events| EventBridge
    EventBridge -->|Subscribe| SSEHub
    SSEHub -->|SSE events| Browser
```

## Prerequisites

### Runtime Dependencies (Required)

All patterns require:
- **Go 1.25+**
- **Docker** and **Docker Compose** (for patterns 2-6)

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

### Pattern 6: Cloud-PubSub (Docker)

Uses `gocloud.dev/pubsub` abstraction. Supports three brokers:

```bash
# NATS JetStream (default)
make run-p6 BROKER=nats

# Kafka
make run-p6 BROKER=kafka

# AWS SNS/SQS (via LocalStack)
make run-p6 BROKER=aws

# open http://localhost:8080
```

## Layered Architecture Overview

All patterns share the same HTTP API and Manager, with **one variation point**: the **Transport Layer** (`contracts.TaskDispatcher` / `contracts.TaskConsumer`).

```mermaid
graph TB
    subgraph L1["🌐 Browser Layer"]
        B["Browser (UI)"]
    end

    subgraph L2["📡 API Layer (100% Shared)"]
        API["shared/api<br/>(HTTP handlers, HTMX)"]
        SSE["shared/sse<br/>(Hub)"]
    end

    subgraph L3["📊 Manager Layer (100% Shared)"]
        MGR["shared/manager<br/>(Task lifecycle, deadlines,<br/>event routing)"]
    end

    subgraph L4["⚡ VARIATION POINT: Transport Layer"]
        P1["P1: ChannelDispatcher<br/>(in-process)"]
        P234["P2/P3/P4:<br/>REST/WS/gRPC"]
        P56["P5/P6:<br/>NATS/CloudPubSub"]
    end

    subgraph L5["👷 Worker Layer (Pluggable)"]
        W["shared/executor<br/>(Stage runner)"]
    end

    subgraph L6["💾 Persistence Layer"]
        STORE["shared/store<br/>(TaskStore interface)<br/>Memory | PostgreSQL"]
        EVENTS["shared/events<br/>(TaskEventBridge)<br/>Memory | NATS | Cloud"]
    end

    B -->|HTTP<br/>POST /tasks<br/>GET /tasks/:id<br/>GET /events| API
    API -->|manager.Submit(task)| MGR
    MGR -->|Dispatch/Receive<br/>contracts.TaskDispatcher| P1
    MGR -->|Dispatch/Receive<br/>contracts.TaskDispatcher| P234
    MGR -->|Dispatch/Receive<br/>contracts.TaskDispatcher| P56
    P1 -->|contracts.TaskConsumer| W
    P234 -->|contracts.TaskConsumer| W
    P56 -->|contracts.TaskConsumer| W
    W -->|Create/Get/List<br/>contracts.TaskStore| STORE
    W -->|Emit events<br/>TaskEventBridge| EVENTS
    EVENTS -->|Subscribe| SSE
    SSE -->|SSE events| B
    MGR -->|Persist state| STORE
    MGR -->|Publish events| EVENTS
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
├── p05/          Brokered-NATS: NATS JetStream (queue) + PostgreSQL (store)
└── p06/          Cloud-PubSub: gocloud.dev abstraction (NATS/Kafka/AWS)
```
