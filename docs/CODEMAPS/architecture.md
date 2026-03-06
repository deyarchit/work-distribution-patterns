# Architecture

## Overview

Six patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

| Interface | Methods | Role |
|-----------|---------|------|
| `contracts.TaskManager` | Submit/Get/List | API → Manager |
| `contracts.TaskDispatcher` | Start/Dispatch/ReceiveEvent | Manager-side transport (⚠ variation point) |
| `contracts.TaskConsumer` | Connect/Receive/Emit | Worker-side transport (⚠ variation point) |
| `events.TaskEventBridge` | Publish/Subscribe | Event streaming |
| `store.TaskStore` | Create/Get/List/SetStatus | Persistence |

- `RemoteTaskManager`: HTTP proxy for Submit/Get/List (P2–P6 APIs)
- Errors: `ErrDispatchFull` → 429, `ErrNoWorkers` → 503

## Design Invariants

- **Manager republishes events** — ⚠ skipping breaks P5/P6 (SSE arrives before DB write).
- **API uses `TaskManager` abstraction** — ⚠ never access store directly; manager-local in P2–P6.
- **Tests: `WaitForWorker` waits for completion** — ⚠ ensures worker idle before suite (P3/P4).

## Process Topology

| Pattern | API | Manager | Worker | Transport |
|---------|-----|---------|--------|-----------|
| P1 | single process | same | goroutines | in-process channels |
| P2 | :8080 | :8081 | separate process | REST polling |
| P3 | :8080 | :8081 | separate process | WebSocket push |
| P4 | :8080 | :8081 | separate process | gRPC bidirectional stream |
| P5 | :8080 (×3) | :8081 (×1) | separate process (×3) | NATS JetStream |
| P6 | :8080 (×3) | :8081 (×1) | separate process (×3) | gocloud PubSub (JetStream) |

## Layering

**API** (`shared/api`): HTTP routes, unchanged across patterns.
**Manager** (`shared/manager`): task lifecycle, deadline loop, event routing.
**Transport** (per-pattern): TaskDispatcher + TaskConsumer implementations.

## Data Flow Diagrams

See [details/backend-patterns.md](./details/backend-patterns.md) for wiring details.

### P1: Goroutine Pool
```mermaid
flowchart LR
    Browser -->|POST /tasks| API
    API --> Manager --> CD[ChannelDispatcher]
    CD -->|events chan| Worker
    Worker -->|Emit| MB[MemoryBridge]
    MB -->|pump| Hub[SSE Hub]
    Hub -->|GET /events| Browser
```

### P2: REST Polling
```mermaid
flowchart LR
    Browser -->|POST /tasks| API
    API -->|HTTP| Manager
    Manager --> RD[RESTDispatcher]
    Worker -->|GET /work/next| RD
    Worker -->|POST /work/events| Manager
    Manager --> MB[MemoryBridge]
    MB -->|sse.Client| Hub[API hub]
    Hub -->|GET /events| Browser
```

### P3: WebSocket Hub
```mermaid
flowchart LR
    Browser -->|POST /tasks| API
    API -->|HTTP| Manager
    Manager --> WD[WebSocketDispatcher]
    Worker -->|WS /ws/register| WD
    WD -->|push task| Worker
    Worker -->|emit event| Manager
    Manager -->|sse.Client| Hub[API hub]
    Hub -->|GET /events| Browser
```

### P4: gRPC Bidirectional
```mermaid
flowchart LR
    Browser -->|POST /tasks| API
    API -->|HTTP| Manager
    Manager --> GD[gRPCDispatcher]
    Worker -->|gRPC bidi stream| GD
    GD -->|stream task| Worker
    Worker -->|stream event| Manager
    Manager -->|sse.Client| Hub[API hub]
    Hub -->|GET /events| Browser
```

### P5: NATS + PostgreSQL
```mermaid
flowchart LR
    Browser -->|POST /tasks| API[API ×3]
    API -->|HTTP| Manager
    Manager -->|tasks.new| Worker
    Worker -->|task.events.*| Manager
    Manager --> NB[NATSBridge]
    NB -->|events| API
    API -->|SSE| Browser
    Manager --> PG[(PostgreSQL)]
```

### P6: Cloud PubSub (gocloud)
```mermaid
flowchart LR
    Browser -->|POST /tasks| API[API ×3]
    API -->|HTTP| Manager
    Manager -->|tasks topic| Worker
    Worker -->|events| Manager
    Manager --> CB[CloudBridge]
    CB -->|events| API
    API -->|SSE| Browser
    Manager --> PG[(PostgreSQL)]
```
