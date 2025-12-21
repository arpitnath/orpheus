# Agent-Native Architecture

> AgentScale is Agent-Native infrastructure - built from scratch for AI agents, not containers adapted.

## Executive Summary

AgentScale takes a fundamentally different approach to running AI agents compared to traditional container orchestration. We call this **Agent-Native** architecture.

**Key Insight:** AI agents are not web services. They have different resource patterns, different failure modes, and different scaling characteristics. Infrastructure designed for microservices doesn't fit.

### What Makes AgentScale Agent-Native

| Characteristic | Container-Native | Agent-Native (AgentScale) |
|----------------|------------------|---------------------------|
| **Scaling trigger** | CPU/memory usage | Queue depth |
| **Memory handling** | Hard OOM kill | Graceful swap/degradation |
| **Timeout model** | Total execution time | Activity-based (detect stuck vs thinking) |
| **Isolation** | Per-pod/container | Per-agent rootfs |
| **Workload assumption** | Fast, predictable | Long-running, variable |

---

## The Problem with Container-Native Approaches

### Memory: Hard Kill is Wrong for Agents

**Docker/Kubernetes approach:**
```
Agent uses 257MB → Limit is 256MB → OOM KILLED → Task lost
```

**Why this fails for agents:**
- Loading large context windows causes memory spikes
- Processing documents requires temporary buffers
- ML inference has peak memory moments
- All these are **normal agent behavior**, not bugs

**Result:** Agents get killed for doing their job.

### Timeout: Total Time is Wrong for Agents

**AWS Lambda / Cloud Functions approach:**
```
Agent runs for 61 seconds → Limit is 60 seconds → KILLED → Task incomplete
```

**Why this fails for agents:**
- Research tasks genuinely take 5-10 minutes
- LLM API calls have variable latency
- Multi-step reasoning requires thinking time
- Agent is making progress, just not "fast enough"

**Result:** Agents get killed for thinking.

### Autoscaling: CPU-Based is Wrong for Agents

**Kubernetes HPA approach:**
```
CPU at 20% → Don't scale
Meanwhile: 50 requests waiting in queue, agents blocked on API calls
```

**Why this fails for agents:**
- Agents spend 90% of time waiting on external APIs
- CPU usage doesn't reflect actual load
- Queue builds up while HPA sees "low utilization"
- Response latency explodes

**Result:** Users wait while infrastructure thinks everything is fine.

---

## Agent-Native Memory Management

### Design: Graceful Degradation with Swap

Instead of hard killing agents that exceed memory limits, AgentScale uses a two-tier system:

```yaml
# agent.yaml
memory:
  target: 256      # MB - Fast performance tier (RAM only)
  limit: 512       # MB - Hard cap (with swap enabled)
  swap_enabled: true
```

### Behavior

```
┌─────────────────────────────────────────────────────────────┐
│ Memory Usage                                                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  0-256 MB     │████████████████│  Fast (pure RAM)           │
│               │                │                             │
│  256-512 MB   │▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒│  Slower (swapping to disk) │
│               │                │                             │
│  >512 MB      │░░░░░░░░░░░░░░░░│  Kill (hard limit exceeded) │
│               │                │                             │
└─────────────────────────────────────────────────────────────┘
```

### How It Works

**Implementation uses cgroups v2:**
- `memory.high` (soft limit) = target value (256MB)
- `memory.max` (hard limit) = limit value (512MB)
- Kernel automatically swaps pages when exceeding `memory.high`
- Process continues running, just slower
- Kill only when exceeding `memory.max`

**User Experience:**

| Scenario | Container-Native | Agent-Native |
|----------|------------------|--------------|
| Agent loads 300MB context | OOM KILLED | Continues (swapping) |
| Agent processes large file | OOM KILLED | Continues (slower) |
| Agent has memory leak to 600MB | OOM KILLED | OOM KILLED |

### Why This Matters

**Real-world agent patterns:**
1. **Context loading:** Agent receives large document → temporary 400MB spike → processes → drops to 150MB
2. **RAG retrieval:** Agent fetches 1000 chunks → peaks at 350MB → filters → drops to 200MB
3. **ML inference:** Model loads → 500MB peak → inference → unloads → 100MB steady

All of these would **fail** with hard 256MB limit. All **succeed** with graceful degradation.

---

## Agent-Native Timeout Management

### Design: Activity-Based Timeout

Instead of killing agents after N seconds regardless of progress, AgentScale tracks activity:

```yaml
# agent.yaml
timeout:
  idle: 60         # seconds - No activity for 60s → stuck, kill
  max: 600         # seconds - Absolute maximum (safety cap)
```

### Activity Detection

**What counts as "activity":**
- stdout/stderr output (agent is communicating)
- Tool call events (agent is working)
- File operations (agent is processing)
- Heartbeat signals (agent is alive)

**What doesn't count:**
- CPU usage (meaningless - waiting on API)
- Memory allocation (not related to progress)

### Behavior

```
┌─────────────────────────────────────────────────────────────┐
│ Time: 0s          30s         60s         90s        120s   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Scenario A: Agent outputting every 10s                       │
│ ──●──●──●──●──●──●──●──●──●──●──●──●──→ COMPLETES (2min)    │
│                                                              │
│ Scenario B: Agent stuck (no output)                          │
│ ────────────────────────────────────╳ KILLED at 60s (idle)  │
│                                                              │
│ Scenario C: Agent running 15 minutes                         │
│ ●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●╳ KILLED at 10min (max)  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### How It Works

**Implementation:**
1. Executor pipes stdout/stderr through activity monitor
2. Each output resets `lastActivity` timestamp
3. Background goroutine checks every 5 seconds:
   - If `now - lastActivity > idle_timeout` → kill (stuck)
   - If `now - startTime > max_timeout` → kill (too long)
4. Agent producing output? Keep running.

**User Experience:**

| Scenario | Container-Native | Agent-Native |
|----------|------------------|--------------|
| 5-min research with updates every 30s | KILLED at 60s | Completes |
| Infinite loop (no output) | Runs until max timeout | Killed at 60s |
| 15-min task with constant output | KILLED at 60s | Killed at 10min (max) |

### Why This Matters

**Real-world agent patterns:**
1. **Deep research:** Agent searches 10 sources, synthesizes → 4 minutes, outputs progress → completes
2. **Stuck agent:** Agent hits API error, retries forever, no output → killed in 60s
3. **Complex analysis:** Agent processes spreadsheet, outputs row-by-row → 3 minutes → completes

All of these behave **correctly** with activity-based timeout.

---

## Agent-Native Autoscaling

### Design: Queue-Depth Based

Instead of scaling based on CPU (meaningless for agents), AgentScale scales based on queue depth:

```yaml
# agentscale.yaml
agents:
  research-agent:
    scaling:
      min_workers: 1
      max_workers: 10
      target_utilization: 2.0    # Target 2 items per worker
      scale_up_threshold: 3.0    # Scale up when >3 items per worker
      scale_down_threshold: 0.5  # Scale down when <0.5 items per worker
      queue_size: 100
```

### Why Queue-Based Scaling

**Agent workload characteristics:**
- 90% of time: Waiting on LLM API call (0% CPU)
- 10% of time: Processing response (5% CPU)
- Average CPU: ~5% even when fully utilized

**With CPU-based scaling:**
```
Queue: 50 items waiting
CPU: 5%
HPA decision: "Low CPU, don't scale"
Result: 50 users waiting
```

**With queue-based scaling:**
```
Queue: 50 items waiting
Workers: 5
Items per worker: 10 (above threshold of 3)
Decision: Scale up to 10 workers
Result: Queue drains in parallel
```

---

## Comparison: AgentScale vs Alternatives

| Feature | Docker/K8s | AWS Lambda | Google Cloud Run | AgentScale |
|---------|-----------|-----------|------------------|------------|
| **Primary use case** | Microservices | Short functions | Stateless HTTP | AI agents |
| **Autoscaling metric** | CPU/Memory | Concurrent requests | Request count | Queue depth |
| **Memory model** | Hard OOM kill | Hard OOM kill | Hard OOM kill | Graceful swap |
| **Timeout model** | Total time | Total time (15min max) | Total time (60min) | Activity-based |
| **Isolation level** | Per-pod | Per-function | Per-container | Per-agent rootfs |
| **Cold start** | Seconds | Milliseconds | Seconds | Milliseconds (warm pool) |
| **Long-running support** | Yes | No (15min) | Yes (60min) | Yes (unlimited*) |
| **Designed for** | 2010s microservices | 2015s serverless | 2018s containers | 2024s AI agents |

*With appropriate timeout.max configuration

---

## Why Not Just Use Kubernetes?

We get this question a lot. Here's the honest answer:

### Kubernetes is Container-Native

K8s was designed in 2014 for a different era:
- **Workloads:** Stateless web services, 100ms response times
- **Scaling:** CPU-bound processing
- **Resources:** Predictable, steady-state

### AI Agents are Different

2024 agent workloads:
- **Workloads:** Stateful reasoning, 10-300 second response times
- **Scaling:** I/O-bound (waiting on LLM APIs)
- **Resources:** Spiky, unpredictable

### Specific K8s Limitations for Agents

**1. HPA doesn't understand agent load:**
```yaml
# K8s HPA sees 5% CPU and doesn't scale
# Meanwhile 100 users are waiting in queue
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  metrics:
  - type: Resource
    resource:
      name: cpu  # ← Meaningless for agents
```

**2. Pod isolation is too coarse:**
```yaml
# All agents in pod share filesystem
# One agent's bad dependency affects others
# No true multi-tenancy
```

**3. OOM behavior is harsh:**
```yaml
# Agent loads 257MB context, limit is 256MB
# K8s: Kill pod, restart, lose all context
resources:
  limits:
    memory: "256Mi"  # ← Hard kill, no graceful degradation
```

**4. Liveness probes don't understand "thinking":**
```yaml
# Agent is deep in research, not responding to health checks
# K8s: "Pod unhealthy, kill it"
livenessProbe:
  httpGet:
    path: /health
  periodSeconds: 10  # ← Agent busy thinking, misses probe
```

### When to Use What

| Use Case | Best Choice |
|----------|-------------|
| Web APIs, microservices | Kubernetes |
| Short functions (<15min) | AWS Lambda |
| Containerized services | Cloud Run |
| AI agents, long-running reasoning | AgentScale |

---

## Configuration Reference

### Per-Agent Configuration (agent.yaml)

```yaml
name: my-agent
runtime: python3
module: agent
entrypoint: handler

# Agent-Native Memory Management
memory:
  target: 256        # MB - Fast performance tier (RAM only)
  limit: 512         # MB - Hard limit (with swap)
  swap_enabled: true # Enable graceful degradation

# Agent-Native Timeout Management
timeout:
  idle: 60           # seconds - Kill if no activity
  max: 600           # seconds - Absolute maximum
```

### Server-Level Defaults (agentscale.yaml)

```yaml
isolation:
  enabled: true

  # Default memory settings for all agents
  memory:
    default_target: 256
    default_limit: 512
    swap_enabled: true

  # Default timeout settings for all agents
  timeout:
    default_idle: 60
    default_max: 300

agents:
  research-agent:
    image: research-agent-v1
    scaling:
      min_workers: 1
      max_workers: 10
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      queue_size: 100

    # Override defaults for this agent
    memory:
      target: 512
      limit: 1024
    timeout:
      idle: 120
      max: 1800  # 30 minutes for deep research
```

---

## Implementation Status

### Current (v0.1.0)

| Feature | Status | Notes |
|---------|--------|-------|
| Memory hard limit | ✅ Implemented | cgroups v2 memory.max |
| Total timeout | ✅ Implemented | context.WithTimeout |
| Queue-based autoscaling | ✅ Implemented | Per-agent queues |
| Per-agent rootfs isolation | ✅ Implemented | pivot_root to agent image |
| Graceful memory swap | ⏳ Planned | memory.high + swap |
| Activity-based timeout | ⏳ Planned | stdout/stderr monitoring |

### Implementation Notes

**Graceful memory swap (planned):**
- Modify `isolate/pkg/cgroups/cgroups.go`
- Add `memory.high` (soft limit) in addition to `memory.max`
- Configure swap space in container
- Track swap usage for metrics

**Activity-based timeout (planned):**
- Modify `pkg/runner/runner.go`
- Add stdout/stderr activity monitor
- Track `lastActivity` timestamp
- Kill on idle timeout, not just total timeout

### Files to Modify

```
agentscale/
├── isolate/pkg/cgroups/cgroups.go     # Add swap support
├── pkg/config/config.go               # New memory/timeout structs
├── pkg/config/server.go               # Server-level defaults
├── pkg/runner/runner.go               # Activity monitoring
└── pkg/proxy/executor.go              # Idle timeout detection
```

---

## Future Roadmap

### v0.2.0 - Observability

- Memory monitoring and metrics (current usage, swap usage)
- Warning at 80% memory usage
- Auto-suggest optimal memory limits based on historical usage
- Timeout monitoring (time to complete, idle time)

### v0.3.0 - Endpoint-Specific Timeouts

- HTTP sync endpoint: Total timeout (current behavior)
- SSE streaming endpoint: Idle timeout (no events = dead)
- MCP protocol: Per-turn timeout (per message, not session)
- Heartbeat mechanism for long-running agents

### v0.4.0 - Advanced Resource Management

- Dynamic memory limits (adjust based on load)
- Memory pressure detection and backoff
- Prioritized queue processing
- Resource quotas per tenant

---

## Summary

AgentScale's Agent-Native architecture represents a fundamental rethinking of infrastructure for AI workloads:

| Traditional Approach | Agent-Native Approach |
|---------------------|----------------------|
| Kill on memory spike | Swap and continue |
| Kill on timeout | Kill only if stuck |
| Scale on CPU | Scale on queue depth |
| Per-pod isolation | Per-agent isolation |

This isn't incremental improvement - it's recognizing that AI agents are a new category of workload that deserves purpose-built infrastructure.

**The bottom line:** AgentScale is built for how AI agents actually behave, not how we wish they behaved.
