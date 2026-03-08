<!-- Load when: Understanding pattern-specific data flows or transport mechanics -->

# Data Flow Diagrams

## P1: Goroutine Pool
```mermaid
flowchart LR
    Browser -->|POST /tasks| API
    API --> Manager --> CD[ChannelDispatcher]
    CD -->|events chan| Worker
    Worker -->|Emit| MB[MemoryBridge]
    MB -->|pump| Hub[SSE Hub]
    Hub -->|GET /events| Browser
```

## P2: REST Polling
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

## P3: WebSocket Hub
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

## P4: gRPC Bidirectional
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

## P5: NATS + PostgreSQL
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

## P6: Cloud PubSub (gocloud)
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

## P7: Bootstrap-Driven Edge Worker
```mermaid
flowchart LR
    Edge -->|mTLS /bootstrap| Bootstrap[Manager:8083]
    Bootstrap -->|BrokerURL + Token| Edge
    Edge -->|Token to Broker| Broker
    Broker -->|tasks| Edge
    Edge -->|events| Manager[Manager:8081]
    Manager -->|HTTP /tasks| API
    Browser -->|GET /events| API
    Manager --> PG[(PostgreSQL)]
```
