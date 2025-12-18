# AgentScale v0.1.0 Development Reference

**Created**: December 18, 2025
**Status**: Complete Implementation Reference
**Purpose**: Single source of truth for v0.1.0 architecture and implementation

---

## Table of Contents

1. [Current Status](#current-status)
2. [Architecture Overview](#architecture-overview)
3. [Phase 1: Multi-Agent Hosting](#phase-1-multi-agent-hosting)
4. [Phase 2: Worker Health & Auto-Healing](#phase-2-worker-health--auto-healing)
5. [Phase 3: Observability (Pending)](#phase-3-observability-pending)
6. [Key Files Reference](#key-files-reference)
7. [Configuration Reference](#configuration-reference)
8. [Testing Guide](#testing-guide)

---

## Current Status

### Completed Phases ✅

| Phase | Commit | What We Built |
|-------|--------|---------------|
| Multi-Agent Refactor Phase 1 | `1cc99d8` | Multi-agent config & server |
| Multi-Agent Refactor Phase 2 | `afab7f7` | Worker health & auto-replacement |

### Pending Work ⏳

| Phase | Effort | What It Adds |
|-------|--------|--------------|
| Phase 3: Observability | ~2 hours | Scaling history, enhanced stats (optional) |
| Image System | ~10 hours | True container isolation with base images |
| Testing & Polish | ~3 hours | Integration tests, docs |

---

## Architecture Overview

### Vision: Multi-Agent Hosting Platform

**AgentScale** = Docker + Kubernetes for AI Agents

```
┌─────────────────────────────────────────────────────────────────┐
│                    AGENTSCALE v0.1.0                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Python CLI (HTTP Client)                                       │
│  • agentscale invoke <agent-id> '{...}'                         │
│  • agentscale stats [agent-id]                                  │
│  • agentscale health                                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Go Server (Multi-Agent Orchestrator)                     │   │
│  │                                                           │   │
│  │  HTTP → Router → AgentInstance                           │   │
│  │                                                           │   │
│  │  Agent A:              Agent B:                           │   │
│  │  ┌──────────────┐      ┌──────────────┐                 │   │
│  │  │Queue (200)   │      │Queue (50)    │                 │   │
│  │  └──────┬───────┘      └──────┬───────┘                 │   │
│  │         │                     │                          │   │
│  │  ┌──────▼───────┐      ┌──────▼───────┐                 │   │
│  │  │Pool (2-10)   │      │Pool (1-5)    │                 │   │
│  │  └──────┬───────┘      └──────┬───────┘                 │   │
│  │         │                     │                          │   │
│  │  [Workers: W1 W2]      [Workers: W1]                     │   │
│  │  Each wraps:                                             │   │
│  │  Runner → Generator → Proxy → Isolate → Agent           │   │
│  │                                                           │   │
│  │  Shared Autoscaler (monitors all pools)                  │   │
│  │  Queue-based scaling (pending+processing/workers)        │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Core Innovation: Queue-Based Autoscaling

**Traditional** (K8s HPA):
```
CPU > 80% → Scale up (reactive, wrong for AI agents)
```

**AgentScale**:
```
utilization = (pending + processing) / workers
if utilization > threshold → Scale up (proactive, business-aware)
```

**Why better**:
- AI agents are I/O bound (waiting for LLM APIs)
- Queue depth = actual demand
- Scales before saturation, not after

---

## Phase 1: Multi-Agent Hosting

**Commit**: `1cc99d8`
**Goal**: Server hosts multiple agents with per-agent scaling

### Configuration Format

**File**: `agentscale.yaml`

```yaml
server:
  port: 8080
  autoscaler_interval: 10s
  isolation:
    enabled: true
    type: auto
    defaults:
      memory_limit: 512mb
      timeout: 300s

agents:
  planning-agent:
    path: ./examples/planning-agent
    scaling:
      min_workers: 2
      max_workers: 10
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      scale_up_delay: 15s
      scale_down_delay: 1m
      queue_size: 200
    isolation:
      memory_limit: 1gb
      timeout: 600s

  simple-agent:
    path: ./examples/simple-agent
    scaling:
      min_workers: 1
      max_workers: 5
      target_utilization: 3.0
      scale_up_threshold: 4.0
      scale_down_threshold: 0.5
      scale_up_delay: 30s
      scale_down_delay: 2m
      queue_size: 50
```

### Server Architecture

**File**: `pkg/server/server.go`

```go
type Server struct {
    serverCfg  *config.ServerConfig
    instances  map[string]*AgentInstance  // Multi-agent
    autoscaler *scaling.BasicAutoscaler   // Shared
    mu         sync.RWMutex
    // ...
}

type AgentInstance struct {
    agentID      string
    cfg          *config.AgentConfig
    queue        *scaling.RequestQueue
    pool         *scaling.BasicWorkerPool
    workerCtx    context.Context
    workerWg     sync.WaitGroup
}
```

### HTTP API

| Endpoint | Purpose |
|----------|---------|
| `POST /invoke?agent=<id>` | Execute agent |
| `GET /health` | List all agents |
| `GET /stats` | All agent stats |
| `GET /stats?agent=<id>` | Specific agent stats |

### Key Changes

1. **Multi-agent support**: Server manages map of AgentInstances
2. **Per-agent queues**: Independent request queues
3. **Per-agent pools**: Independent worker pools
4. **Shared autoscaler**: Monitors all pools
5. **HTTP routing**: Query param `?agent=<id>` routes to correct instance

**Files created**:
- `pkg/config/server.go` - ServerConfig structs
- `pkg/config/server_loader.go` - YAML loader

**Files modified**:
- `pkg/server/server.go` - Multi-agent support
- `pkg/server/handlers.go` - Agent routing
- `cmd/agentscale-server/main.go` - --config flag
- `cli/agentscale/commands/client.py` - agent_id parameter

---

## Phase 2: Worker Health & Auto-Healing

**Commit**: `afab7f7`
**Goal**: Auto-healing, reliability improvements

### Features Added

**Periodic Health Checks**:
- Maintenance loop checks all workers every 30s
- Detects unhealthy workers proactively
- Triggers auto-replacement

**Auto-Replacement**:
```go
func (p *BasicWorkerPool) replaceUnhealthyWorker(worker Worker) {
    // Rate limiting: 5s cooldown
    // Max 3 attempts per worker
    // Linear backoff (1s, 2s, 3s)
    // Spawns replacement
}
```

**Degraded State Handling**:
- Track consecutive failures
- 1-2 failures → HealthDegraded
- 3 failures → HealthUnhealthy
- Success → Reset to HealthHealthy

**Emergency Spawning**:
- Already existed, preserved
- 30s timeout in GetIdleWorker
- Prevents deadlock

### Key Changes

**File**: `pkg/scaling/worker_pool.go`
- Added: `performHealthCheck()` method
- Added: `replaceUnhealthyWorker()` with rate limiting
- Added: Replacement tracking (attempts map)

**File**: `pkg/scaling/worker.go`
- Added: `consecutiveFailures` atomic counter
- Modified: `Execute()` tracks failures, sets degraded state

**Testing**: Manual verification guide in `docs/PHASE-2-TESTING-GUIDE.md`

---

## Phase 3: Observability (Pending)

**Status**: Not implemented (optional for v0.1.0)
**Effort**: ~2 hours

### What It Would Add

**Scaling History**:
- Track every scaling event (timestamp, reason, utilization)
- API: `GET /scaling/history?agent=<id>`
- CLI: `agentscale scaling-history <agent-id>`

**Enhanced Stats**:
- Worker breakdown (idle, busy, degraded)
- Request metrics (total, processed, failed)
- Utilization over time

**Deferred**: Can add in v0.2 based on user feedback

---

## Key Files Reference

### Core Packages

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `pkg/config/` | Configuration | server.go, server_loader.go, config.go |
| `pkg/scaling/` | Autoscaling | worker_pool.go, worker.go, autoscaler.go, queue.go |
| `pkg/server/` | HTTP Server | server.go, handlers.go |
| `pkg/runner/` | Execution | runner.go |
| `pkg/proxy/` | Process execution | executor.go |
| `isolate/` | Container runtime | Full isolation system |

### Binaries

| Binary | Source | Purpose |
|--------|--------|---------|
| `bin/agentscale-runtime` | `cmd/agentscale-runtime/` | Single execution |
| `bin/agentscale-server` | `cmd/agentscale-server/` | Multi-agent server |
| `isolate/bin/isolate` | `isolate/cmd/isolate/` | Container runtime |

### CLI

| Command | Implementation | Purpose |
|---------|----------------|---------|
| `agentscale run` | `cli/commands/run.py` | Single execution (subprocess) |
| `agentscale invoke` | `cli/commands/client.py` | HTTP to server |
| `agentscale stats` | `cli/commands/client.py` | Server stats |
| `agentscale health` | `cli/commands/client.py` | Health check |

---

## Configuration Reference

### agent.yaml (Per-Agent Code Config)

```yaml
name: my-agent
runtime: python3
runtime_version: "3.10"    # Optional
module: my_agent
entrypoint: main

memory: 512
timeout: 120

env:
  - "OPENAI_API_KEY=sk-..."
```

### agentscale.yaml (Server Deployment Config)

```yaml
server:
  port: 8080
  autoscaler_interval: 10s
  isolation:
    enabled: true
    type: auto
    defaults:
      memory_limit: 512mb
      timeout: 300s

agents:
  <agent-id>:
    path: <path-to-agent-directory>
    scaling:
      min_workers: 1
      max_workers: 10
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      scale_up_delay: 15s
      scale_down_delay: 1m
      queue_size: 200
    isolation:            # Optional overrides
      memory_limit: 1gb
      timeout: 600s
```

### Scaling Policy Guidelines

**Conservative**:
```yaml
min_workers: 1
max_workers: 5
target_utilization: 3.0
scale_up_threshold: 4.0
scale_up_delay: 30s
queue_size: 50
```

**Balanced**:
```yaml
min_workers: 2
max_workers: 10
target_utilization: 2.0
scale_up_threshold: 3.0
scale_up_delay: 15s
queue_size: 200
```

**Aggressive**:
```yaml
min_workers: 3
max_workers: 20
target_utilization: 1.5
scale_up_threshold: 2.5
scale_up_delay: 10s
queue_size: 500
```

---

## Testing Guide

### Basic Functionality

**Start server**:
```bash
cd agentscale
./bin/agentscale-server --config ./examples/agentscale.yaml
```

**Health check**:
```bash
agentscale health
# ✓ Server healthy - 2 agents:
#   - planning-agent: planning-agent (2 workers)
#   - simple-agent: simple-agent (1 workers)
```

**Invoke agents**:
```bash
agentscale invoke simple-agent '{"query": "hello"}'
agentscale invoke planning-agent '{"input_as_text": "test"}'
```

**Check stats**:
```bash
agentscale stats                    # All agents
agentscale stats planning-agent     # Specific agent
```

### Load Testing

**Trigger autoscaling**:
```bash
# Send 20 concurrent requests
for i in {1..20}; do
  agentscale invoke simple-agent '{"query": "test"}' &
done
wait

# Check if scaled up
agentscale stats simple-agent
```

**Watch server logs** for:
```
[autoscaler] simple-agent: scaling 1 → 3 (util=5.2)
[pool] simple-agent: spawned worker simple-agent-worker-2
```

### Manual Health Testing

See: `docs/PHASE-2-TESTING-GUIDE.md` for detailed health check scenarios

---

## Commit History

```
afab7f7  feat(scaling): Phase 2 - Worker health checks and auto-replacement
1cc99d8  feat(server): Phase 1 - Multi-agent hosting with per-agent autoscaling
fbfc796  chore(cli): Update egg-info metadata for Phase 6 changes
e8bd193  feat(cli): Phase 6 - Add HTTP client commands and Go server binary
d17833c  feat(server): Phase 5 - Add HTTP server with scaling orchestration
e8f7a6d  feat(scaling): Phase 4 - Add request queue with QueueMetrics
22e88e3  feat(scaling): Phase 3 - Add queue-based autoscaler
50e0444  feat(scaling): Phase 2 - Add worker pool implementation
b74a5be  feat(scaling): Phase 1 - Add scaling interfaces and core types
```

---

## Quick Commands

```bash
# Build
cd agentscale
go build -o bin/agentscale-server ./cmd/agentscale-server

# Install CLI
cd cli && pip install -e .

# Run server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Test
agentscale health
agentscale invoke simple-agent '{"query": "test"}'
agentscale stats
```

---

## Critical Architectural Decisions

### 1. Multi-Agent Architecture
- One server manages multiple agents
- Each agent: independent queue + pool
- Shared autoscaler monitors all
- HTTP routing by agent ID

### 2. Queue-Based Scaling
- Utilization = (pending + processing) / workers
- Proactive scaling (before saturation)
- Per-agent policies (min, max, thresholds)

### 3. Worker Health
- Periodic checks (30s maintenance)
- Auto-replacement with rate limiting (5s cooldown)
- Max 3 attempts per worker
- Degraded state tracking

### 4. Graceful Shutdown
- Per-agent queue closing
- Worker loop draining
- Clean pool shutdown
- 30s timeout per phase

### 5. Configuration Design
- Declarative YAML (agentscale.yaml)
- Per-agent scaling policies
- Global defaults + per-agent overrides
- Auto-detection from agent.yaml

---

## Known Issues & Gaps

### Current Limitations

**Isolation** (Critical):
- ✅ Namespace isolation (pid, net, mount)
- ❌ **Uses host filesystem** (not using --rootfs)
- ❌ **No Python in container images**
- ⚠️ Falls back on macOS (VM has no Python)

**Status**: Container runtime exists, images need to be built

**Fix Required**: Image system implementation (~10 hours)

### Platform-Specific

**Linux**:
- Namespace isolation works
- Uses host Python (not ideal)
- Needs custom rootfs with Python

**macOS**:
- VM boots successfully
- vsock communication works
- Needs Python in VM initrd
- Currently falls back to direct execution

---

## Next Steps

### Immediate (Image System)
1. Build python-3.10 base image (Linux + macOS)
2. Implement `agentscale deploy` command
3. Update executor to use --rootfs
4. Remove fallback warnings
5. Test true isolation

### After Images
1. Integration testing
2. Documentation updates
3. Example agents
4. Release preparation

### v0.2 Planning
- Cloud deployment API
- Image registry
- Node.js support
- Advanced features

---

## Related Documentation

**Internal Discussion**:
- `/internal_discussion/final_disc/01-AGENTSCALE-IMAGES-ARCHITECTURE.md` - Image system design
- `/internal_discussion/final_disc/02-V0.1.0-SCOPE-AND-DECISIONS.md` - Locked scope
- `/internal_discussion/loved_response_3.md` - OSS vs Cloud strategy

**Previous Plans** (Superseded):
- ~~v0.1.0-IMPLEMENTATION-PLAN.md~~ - Original single-agent plan
- ~~v0.1.0-MULTI-AGENT-REFACTOR.md~~ - Refactor plan (completed)
- ~~PHASE-*.md~~ - Individual phase docs (consolidated here)

---

**This is the single source of truth for v0.1.0 development.**
