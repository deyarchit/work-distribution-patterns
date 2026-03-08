# Architecture

## Overview

Seven patterns demonstrating different work distribution topologies and deployment models. P1–P6 share the same HTTP API surface and HTMX frontend. P7 adds edge worker bootstrap with zero redeployments when infrastructure changes.

## Shared Interfaces

| Interface | Methods | Role |
|-----------|---------|------|
| `contracts.TaskManager` | Submit/Get/List | API → Manager |
| `contracts.TaskDispatcher` | Start/Dispatch/ReceiveEvent | Manager-side transport (⚠ variation point) |
| `contracts.TaskConsumer` | Connect/Receive/Emit | Worker-side transport (⚠ variation point) |
| `events.TaskEventBridge` | Publish/Subscribe | Event streaming |
| `store.TaskStore` | Create/Get/List/SetStatus | Persistence |
- `p07.bootstrap.BootstrapResponse`: Worker runtime discovery
- `p07.bootstrap.Tokener`: HMAC token vending
- `p07.bootstrap.RevocationList`: Per-device revocation
- `RemoteTaskManager`: HTTP proxy (P2–P7 APIs)
- Errors: `ErrDispatchFull` → 429, `ErrNoWorkers` → 503

## Design Invariants

- **Manager republishes events** — ⚠ skipping breaks SSE (arrives before DB write).
- **API uses `TaskManager` abstraction** — ⚠ never direct store access.
- **Tests: `WaitForWorker` waits for completion** — ⚠ worker idle before suite.
- **P7: zero hardcoded broker config** — ⚠ all discovery via bootstrap handshake.

## Process Topology

| Pattern | API | Manager | Worker | Transport |
|---------|-----|---------|--------|-----------|
| P1 | single process | same | goroutines | in-process channels |
| P2 | :8080 | :8081 | separate process | REST polling |
| P3 | :8080 | :8081 | separate process | WebSocket push |
| P4 | :8080 | :8081 | separate process | gRPC bidirectional stream |
| P5 | :8080 (×3) | :8081 (×3) | separate process (×3) | NATS JetStream |
| P6 | :8080 (×3) | :8081 (×3) | separate process (×3) | gocloud PubSub (JetStream) |
| P7 | :8080 | :8081 + :8083 mTLS | separate process | mTLS bootstrap → gocloud PubSub |

## Layering

**API** (`shared/api`): HTTP routes, unchanged across patterns.
**Manager** (`shared/manager`): task lifecycle, deadline loop, event routing.
**Transport** (per-pattern): TaskDispatcher + TaskConsumer implementations.

## Data Flow

See [details/architecture-dataflow.md](./details/architecture-dataflow.md) for per-pattern diagrams.
