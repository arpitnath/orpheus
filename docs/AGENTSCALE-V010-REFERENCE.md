# AgentScale v0.1.0 "The WTF Launch" - Complete Reference

**Created**: December 15, 2025
**Status**: Phases 1-6 Complete, Phase 7 Pending
**Commits**: `b74a5be` → `fbfc796`

---

## What We Built

Queue-based autoscaling for AI agents - the core differentiator that makes AgentScale unique.

**Before (MVP v0.0.x):**
```
CLI → Runner → Single execution → Exit
```

**After (v0.1.0):**
```
HTTP Request → Queue → WorkerPool → Runner → Agent
                 ↑           ↑
           QueueMetrics  Autoscaler
              (queue-based scaling)
```

---

## The Innovation: Queue-Based Scaling

Traditional autoscaling (CPU/memory) doesn't work for AI agents because they're I/O bound waiting for LLM APIs.

**Our approach:**
```
utilization = (pending_tasks + processing_tasks) / current_workers

if utilization > ScaleUpThreshold → scale UP
if utilization < ScaleDownThreshold → scale DOWN
```

This is **predictive** not reactive - we know exactly how much work is queued.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AGENTSCALE v0.1.0                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────────────┐               │
│  │  Python CLI     │   HTTP       │  Go Server              │               │
│  │  (client only)  │ ────────────▶│  (runs independently)   │               │
│  └─────────────────┘              └───────────┬─────────────┘               │
│                                               │                             │
│  Commands:                                    ▼                             │
│  • agentscale invoke              ┌─────────────────────────┐               │
│  • agentscale status              │     Request Queue       │               │
│  • agentscale health              │  (QueueMetrics impl)    │               │
│                                   └───────────┬─────────────┘               │
│                                               │                             │
│                                   ┌───────────▼─────────────┐               │
│                    Autoscaler ──▶ │     Worker Pool         │               │
│                    (monitors)     │  (GetIdleWorker blocks) │               │
│                                   └───────────┬─────────────┘               │
│                                               │                             │
│                                   ┌───────────▼─────────────┐               │
│                                   │   Existing MVP Pipeline │               │
│                                   │ Runner → Generator →    │               │
│                                   │ Proxy → Isolate → Agent │               │
│                                   └─────────────────────────┘               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase Summary

| Phase | Commit | What We Built |
|-------|--------|---------------|
| 1 | `b74a5be` | Interfaces, types, tier configs |
| 2 | `50e0444` | Worker pool with spawner |
| 3 | `22e88e3` | Queue-based autoscaler |
| 4 | `e8f7a6d` | Request queue with QueueMetrics |
| 5 | `d17833c` | HTTP server orchestrating all components |
| 6 | `e8bd193` | Python CLI (HTTP client) + Go server binary |
| 7 | Pending | Integration testing |

---

## Phase 1: Interfaces & Core Types

**Commit**: `b74a5be`
**Goal**: Define contracts all components depend on

### Files Created

| File | Purpose |
|------|---------|
| `pkg/scaling/interfaces.go` | Worker, WorkerPool, Autoscaler, QueueMetrics interfaces |
| `pkg/scaling/types.go` | Result, HealthStatus, PoolStats |
| `pkg/scaling/config.go` | TierScalingConfig, ScalingPolicy, tier defaults |

### Key Interfaces

```
Worker
├── ID() string
├── AgentID() string
├── IsIdle() bool
├── Execute(ctx, input) → (Result, error)
├── Shutdown(ctx) error
└── Health() HealthStatus

WorkerPool
├── AgentID() string
├── Size() int
├── DesiredSize() int
├── SetDesiredSize(size)
├── GetIdleWorker(ctx) → (Worker, error)  // BLOCKS
├── ReturnWorker(worker)
├── Shutdown(ctx) error
└── GetStats() PoolStats

Autoscaler
├── Start(ctx) error
├── Stop() error
├── RegisterPool(agentID, pool, policy)
└── RegisterQueueMetrics(agentID, metrics)

QueueMetrics
├── PendingTasks() int
├── ProcessingTasks() int
└── QueueLength() int
```

### Tier Configuration

| Tier | MinWorkers | MaxWorkers | ScaleUpDelay | ScaleDownDelay | QueueSize |
|------|------------|------------|--------------|----------------|-----------|
| free | 1 | 2 | 30s | 2min | 50 |
| pro | 1 | 10 | 15s | 1min | 200 |
| teams | 2 | 50 | 10s | 30s | 500 |

**Reference**: `agentscale/pkg/scaling/config.go:15-55`

---

## Phase 2: Worker Pool Implementation

**Commit**: `50e0444`
**Goal**: Manage dynamic pool of workers

### Files Created

| File | Purpose |
|------|---------|
| `pkg/scaling/worker_pool.go` | BasicWorkerPool with maintenance loop |
| `pkg/scaling/worker.go` | AgentWorker, AgentSpawner (wraps existing Runner) |

### How It Works

```
BasicWorkerPool
├── workers map[string]Worker     // All workers by ID
├── idleWorkers chan Worker       // Available workers (buffered)
├── desiredSize atomic.Int32      // Target size (set by autoscaler)
└── mu sync.RWMutex               // Protects workers map

GetIdleWorker(ctx):
1. Try to get from idleWorkers channel
2. If empty and Size < MaxWorkers → spawn new
3. Block until worker available or ctx done

ReturnWorker(worker):
1. Check if pool shrinking (Size > DesiredSize) → terminate
2. Otherwise put back in idleWorkers channel
```

### Key Decision

Workers wrap the existing MVP Runner - no reimplementation needed:
```
AgentWorker.Execute() → runner.Run() → existing pipeline
```

**Reference**: `agentscale/pkg/scaling/worker.go:56-75`

---

## Phase 3: Autoscaler Implementation

**Commit**: `22e88e3`
**Goal**: Make scaling decisions based on queue metrics

### File Created

| File | Purpose |
|------|---------|
| `pkg/scaling/autoscaler.go` | BasicAutoscaler with scaling loop |

### Scaling Algorithm

```go
func GetScalingDecision(agentID string) int {
    pending := metrics.PendingTasks()
    processing := metrics.ProcessingTasks()
    current := pool.Size()

    utilization := (pending + processing) / current

    if utilization > ScaleUpThreshold {
        target := ceil((pending + processing) / TargetUtilization)
        return min(target, MaxWorkers)
    }

    if utilization < ScaleDownThreshold {
        target := ceil((pending + processing) / TargetUtilization)
        return max(target, MinWorkers)
    }

    return current  // No change
}
```

### Scaling Loop

- Runs every 5 seconds
- Checks cooldown delays before scaling
- Logs scaling decisions

**Reference**: `agentscale/pkg/scaling/autoscaler.go:89-130`

---

## Phase 4: Request Queue Implementation

**Commit**: `e8f7a6d`
**Goal**: Queue that provides metrics to autoscaler

### File Created

| File | Purpose |
|------|---------|
| `pkg/scaling/queue.go` | RequestQueue implementing QueueMetrics |

### Request/Response Types

```
Request
├── ID string
├── Input []byte
├── Context context.Context
├── ResponseCh chan *Response
├── QueuedAt time.Time
└── StartedAt *time.Time

Response
├── Result *Result
├── Error error
└── Duration time.Duration
```

### Queue Operations

| Method | Behavior |
|--------|----------|
| `Enqueue(ctx, req)` | Add to pending, returns ErrQueueFull/ErrQueueClosed |
| `Dequeue(ctx)` | Block until available, move to processing |
| `Complete(reqID)` | Remove from processing after execution |
| `Close()` | Stop accepting new requests |
| `Drain()` | Return all pending (for shutdown) |

### QueueMetrics Implementation

```go
PendingTasks()    → atomic.LoadInt32(&pendingCount)
ProcessingTasks() → atomic.LoadInt32(&processingCount)
QueueLength()     → PendingTasks() + ProcessingTasks()
```

**Reference**: `agentscale/pkg/scaling/queue.go:70-85`

---

## Phase 5: HTTP Server Implementation

**Commit**: `d17833c`
**Goal**: Orchestrate all components with HTTP endpoints

### Files Created

| File | Purpose |
|------|---------|
| `pkg/server/server.go` | Server struct, lifecycle, worker loop |
| `pkg/server/handlers.go` | HTTP handlers for /invoke, /health, /stats |

### Server Orchestration

```
Server.New(cfg, addr, tier):
1. Get tier configuration
2. Create RequestQueue (tier-based size)
3. Create AgentSpawner (wraps config)
4. Create WorkerPool (spawner + policy)
5. Create Autoscaler
6. Setup HTTP routes

Server.Start():
1. Start autoscaler
2. Register pool and queue metrics
3. Start worker loop goroutines
4. Start HTTP listener
```

### Worker Loop

```go
func workerLoop() {
    for {
        req := queue.Dequeue(ctx)        // Block for request
        worker := pool.GetIdleWorker()   // Block for worker
        result := worker.Execute(input)
        pool.ReturnWorker(worker)
        req.ResponseCh <- Response{Result: result}
        queue.Complete(req.ID)
    }
}
```

### HTTP Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/invoke` | Execute agent, wait for response |
| GET | `/health` | Health check (agent_id, tier) |
| GET | `/stats` | Queue/pool statistics |

### Response Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad request |
| 503 | Queue full / shutting down |
| 504 | Request timeout |

**Reference**: `agentscale/pkg/server/server.go:91-164`

---

## Phase 6: CLI Commands + Server Binary

**Commit**: `e8bd193`
**Goal**: Python CLI as HTTP client, Go server binary

### Files Created

| File | Purpose |
|------|---------|
| `cmd/agentscale-server/main.go` | Go server entry point |
| `cli/agentscale/commands/client.py` | HTTP client commands |

### Go Server Binary

```bash
./bin/agentscale-server --agent ./my-agent --port 8080 --tier pro
```

Flags:
- `--agent` - Path to agent directory (required)
- `--port` - Server port (default: 8080)
- `--tier` - Scaling tier: free/pro/teams (default: pro)

**Reference**: `agentscale/cmd/agentscale-server/main.go`

### Python CLI Commands

| Command | Endpoint | Purpose |
|---------|----------|---------|
| `agentscale invoke '{"query": "test"}'` | POST /invoke | Execute agent |
| `agentscale status` | GET /stats | Queue/pool stats |
| `agentscale health` | GET /health | Health check |

Options:
- `--server, -s` - Server URL (default: http://localhost:8080)
- `--timeout, -t` - Request timeout (default: 300s)
- `--raw` - Output raw JSON

**Reference**: `agentscale/cli/agentscale/commands/client.py`

### Architecture Decision

**Separation**: Go server runs independently, Python CLI is pure HTTP client.

Why:
- CLI can evolve independently
- Python devs can contribute to CLI
- Server handles all scaling logic
- Clean deployment boundary

---

## File Reference

### Scaling Package (`pkg/scaling/`)

| File | Lines | Purpose |
|------|-------|---------|
| interfaces.go | ~90 | Worker, WorkerPool, Autoscaler, QueueMetrics |
| types.go | ~50 | Result, HealthStatus, PoolStats |
| config.go | ~100 | TierScalingConfig, ScalingPolicy, defaults |
| worker.go | ~130 | AgentWorker, AgentSpawner |
| worker_pool.go | ~280 | BasicWorkerPool |
| autoscaler.go | ~200 | BasicAutoscaler |
| queue.go | ~210 | RequestQueue |

### Server Package (`pkg/server/`)

| File | Lines | Purpose |
|------|-------|---------|
| server.go | ~220 | Server struct, lifecycle, worker loop |
| handlers.go | ~190 | HTTP handlers |

### CLI (`cli/agentscale/`)

| File | Purpose |
|------|---------|
| main.py | Entry point, registers commands |
| commands/run.py | Single execution (subprocess) |
| commands/client.py | HTTP client commands |

### Binaries

| Binary | Source | Purpose |
|--------|--------|---------|
| `bin/agentscale-runtime` | `cmd/agentscale-runtime/` | Single execution |
| `bin/agentscale-server` | `cmd/agentscale-server/` | HTTP server with scaling |

---

## Key Decisions & Learnings

### 1. Worker Reuse Pattern

Workers wrap existing Runner - no reimplementation. Execute() calls runner.Run() internally.

### 2. Blocking vs Non-blocking

- `GetIdleWorker()` blocks until worker available
- `Enqueue()` returns ErrQueueFull if at capacity (no blocking)
- `Dequeue()` blocks until request available

### 3. Atomic Counters for Metrics

Queue uses `atomic.Int32` for pendingCount/processingCount - lock-free metric access for autoscaler.

### 4. Graceful Shutdown Order

```
1. Cancel server context (signals workers)
2. Shutdown HTTP server (drain connections)
3. Close queue (stop new requests)
4. Wait for worker loops to drain
5. Stop autoscaler
6. Shutdown worker pool
```

### 5. CLI Architecture Pivot

Originally planned subprocess wrapper (`agentscale serve` calling Go binary). Changed to pure HTTP client for better separation and Python dev contribution.

---

## Discoveries

### 1. Response Type Mismatch

`InvokeResponse.Output` was initially `string`, but `Result.Output` is `map[string]interface{}`. Fixed by changing to `map[string]any`.

### 2. Tier Queue Sizes

Different tiers need different queue sizes:
- free: 50 (prevents abuse)
- pro: 200 (reasonable workload)
- teams: 500 (high throughput)

### 3. Worker Loop Count

Initially hard-coded. Changed to start with `pool.Size()` workers (respects tier MinWorkers).

---

## What's Left: Phase 7

Integration testing to verify:
1. Server starts and loads config
2. CLI → Server communication works
3. Autoscaling triggers under load
4. Queue limits enforced
5. Graceful shutdown works

---

## Quick Commands

```bash
# Build
go build -o bin/agentscale-server ./cmd/agentscale-server

# Start server
./bin/agentscale-server --agent ./examples/planning-agent --port 8080 --tier pro

# CLI commands
agentscale health
agentscale status
agentscale invoke '{"input": "test"}'

# With custom server
agentscale invoke '{"input": "test"}' -s http://prod:8080
```

---

## Commit History

```
fbfc796 chore(cli): Update egg-info metadata for Phase 6 changes
e8bd193 feat(cli): Phase 6 - Add HTTP client commands and Go server binary
d17833c feat(server): Phase 5 - Add HTTP server with scaling orchestration
e8f7a6d feat(scaling): Phase 4 - Add request queue with QueueMetrics
22e88e3 feat(scaling): Phase 3 - Add queue-based autoscaler
50e0444 feat(scaling): Phase 2 - Add worker pool implementation
b74a5be feat(scaling): Phase 1 - Add scaling interfaces and core types
```

---

## Related Docs

| Doc | Purpose |
|-----|---------|
| `docs/v0.1.0-IMPLEMENTATION-PLAN.md` | Original implementation plan |
| `docs/PHASE-{1-6}-COMPLETE.md` | Individual phase completion notes |
| `MVP-AUDIT.md` | Pre-v0.1.0 MVP audit |
