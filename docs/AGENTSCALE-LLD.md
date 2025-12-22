# AgentScale v0.1.0 - Low-Level Design Document

> **Document Type**: Architecture LLD (No Code)
> **Version**: 0.1.0
> **Last Updated**: December 2024

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Package Architecture](#2-package-architecture)
3. [Component Deep Dive](#3-component-deep-dive)
4. [Execution Flow](#4-execution-flow)
5. [Isolation Architecture](#5-isolation-architecture)
6. [Multi-Agent Server](#6-multi-agent-server)
7. [Agent-Native Features](#7-agent-native-features)
8. [Configuration System](#8-configuration-system)
9. [Data Contracts](#9-data-contracts)
10. [File Reference Map](#10-file-reference-map)

---

## 1. System Overview

### 1.1 What is AgentScale?

AgentScale is a distributed execution platform for AI agents, designed as **"Kubernetes for AI Agents"** but with agent-native primitives rather than container-native ones.

### 1.2 Core Philosophy

| Traditional (K8s/Docker) | Agent-Native (AgentScale) |
|--------------------------|---------------------------|
| OOM Kill on memory exceed | Graceful degradation with swap |
| Fixed timeout | Activity-based timeout (idle detection) |
| CPU-based autoscaling | Queue-depth autoscaling |
| Container orchestration | Agent execution lifecycle |

### 1.3 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        USER INTERFACE                            │
├─────────────────────────────────────────────────────────────────┤
│  agentscale CLI (Python)     │    HTTP API (Go Server)          │
│  - run, deploy, invoke       │    - /invoke, /health, /stats    │
└──────────────┬───────────────┴──────────────┬───────────────────┘
               │                              │
               ▼                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EXECUTION LAYER (Go)                        │
├─────────────────────────────────────────────────────────────────┤
│  Runner          │  Generator       │  Proxy/Executor           │
│  (orchestration) │  (entrypoint)    │  (process mgmt)           │
└──────────────────┴──────────────────┴───────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────────────────┐
│                     ISOLATION LAYER                              │
├─────────────────────────────────────────────────────────────────┤
│  Linux: Namespaces + cgroups v2    │  macOS: VM + vsock         │
│  - PID, Mount, UTS, IPC, Net       │  - Apple Virtualization    │
│  - memory.high/max, cpu.max        │  - PUI PUI Linux kernel    │
└─────────────────────────────────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      AGENT RUNTIME                               │
├─────────────────────────────────────────────────────────────────┤
│  _entrypoint.py (generated)  →  User Agent Code (handler)       │
│  stdin: JSON  →  handler()  →  stdout: JSON                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Package Architecture

### 2.1 Directory Structure

```
agentscale/
├── cmd/                          # Go CLI binaries
│   ├── agentscale-runtime/       # Single agent executor
│   └── agentscale-server/        # Multi-agent HTTP server
│
├── pkg/                          # Core Go packages
│   ├── config/                   # Configuration loading & validation
│   ├── generator/                # Python entrypoint generation
│   ├── proxy/                    # Process execution & monitoring
│   ├── runner/                   # Execution orchestration
│   ├── scaling/                  # Queue-based autoscaling
│   └── server/                   # HTTP server & routing
│
├── isolate/                      # Container/VM isolation
│   ├── cmd/isolate/              # Isolation CLI
│   ├── cmd/vsock-agent/          # VM command bridge
│   └── pkg/                      # Isolation primitives
│       ├── cgroups/              # Linux cgroups v2
│       ├── container/            # Container lifecycle
│       ├── rootfs/               # Filesystem isolation
│       └── vm/                   # macOS VM management
│
├── cli/                          # Python CLI
│   └── agentscale/
│       ├── commands/             # CLI commands
│       └── utils/                # Utilities
│
├── bridge/                       # Python runtime bridge
│   └── run_agent.py              # Agent execution wrapper
│
└── examples/                     # Example agents
    └── calculator-agent/
```

### 2.2 Package Dependency Graph

```
                    cmd/agentscale-runtime
                           │
                           ▼
                      pkg/runner ◄──────────────────┐
                      /    │    \                   │
                     /     │     \                  │
                    ▼      ▼      ▼                 │
            pkg/config  pkg/generator  pkg/proxy   │
                                          │        │
                                          ▼        │
                                   [isolate binary]│
                                                   │
                    cmd/agentscale-server          │
                           │                       │
                           ▼                       │
                      pkg/server                   │
                      /         \                  │
                     ▼           ▼                 │
              pkg/scaling    pkg/config            │
                   │                               │
                   └───────────────────────────────┘
                        (via AgentWorker)
```

---

## 3. Component Deep Dive

### 3.1 Configuration Package (`pkg/config/`)

**Purpose**: Load, validate, and apply defaults to agent/server configurations.

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `AgentConfig` | `config.go:7-34` | Single agent configuration |
| `ServerConfig` | `server_config.go` | Multi-agent server configuration |
| `AgentDeployment` | `server_config.go` | Per-agent deployment settings |
| `ScalingConfig` | `server_config.go` | Queue-based scaling parameters |

**Key Functions**:

| Function | File | Description |
|----------|------|-------------|
| `Load()` | `loader.go` | Load agent.yaml from directory |
| `LoadServerConfig()` | `loader.go` | Load agentscale.yaml |
| `ApplyDefaults()` | `defaults.go` | Set default values |
| `Validate()` | `validator.go` | Validate configuration |

**Configuration Flow**:
```
agent.yaml → Load() → YAML Parse → ApplyDefaults() → Validate() → *AgentConfig
```

---

### 3.2 Generator Package (`pkg/generator/`)

**Purpose**: Generate Python entrypoint files that bridge user code to the runtime.

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `Generator` | `generator.go` | Entrypoint generator |
| `TemplateData` | `generator.go` | Template rendering context |

**Template Location**: `pkg/generator/templates.go`

**Generated File Structure** (`_entrypoint.py`):
```
1. Import sys, json, asyncio, inspect
2. Import user's module and entrypoint
3. Read JSON from stdin
4. Parse input (optionally via Pydantic InputType)
5. Call handler (detect sync/async automatically)
6. Serialize output to JSON
7. Print to stdout
8. Handle exceptions → error JSON + exit(1)
```

**Generation Flow**:
```
AgentConfig → Generate() → Template Render → Write _entrypoint.py → chmod 755
```

---

### 3.3 Proxy Package (`pkg/proxy/`)

**Purpose**: Execute agent processes with isolation, timeout management, and output capture.

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `Proxy` | `proxy.go` | Execution wrapper with config |
| `ExecuteOptions` | `executor.go:18-47` | Execution parameters |
| `Result` | `result.go:16-38` | Execution result |
| `ActivityMonitor` | `activity.go:15-24` | Idle timeout tracker |
| `Status` | `result.go:7-14` | Result status enum |

**Status Values**:
- `success` - Normal completion
- `error` - Execution error
- `timeout` - Max timeout exceeded
- `idle_timeout` - No activity detected

**Execution Modes**:

| Mode | Function | When Used |
|------|----------|-----------|
| Simple | `runAgentSimple()` | No idle timeout configured |
| Activity-Monitored | `runAgentWithActivityMonitor()` | Idle timeout > 0 |

**Execution Flow (Activity-Monitored)**:
```
RunAgent()
    │
    ├── IdleTimeout > 0? ─────► runAgentWithActivityMonitor()
    │                                    │
    │                                    ├── Create ActivityMonitor
    │                                    ├── Create stdout/stderr pipes
    │                                    ├── cmd.Start()
    │                                    ├── Launch monitoring goroutines
    │                                    ├── Start timeout watcher
    │                                    │
    │                                    └── Select:
    │                                        ├── doneChan → Normal result
    │                                        ├── timeoutChan → Kill + timeout result
    │                                        └── ctx.Done() → Kill + context cancelled
    │
    └── No ──────────────────► runAgentSimple()
                                         │
                                         ├── cmd.Run() (blocking)
                                         └── processResult()
```

---

### 3.4 Runner Package (`pkg/runner/`)

**Purpose**: Orchestrate the complete agent execution lifecycle.

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `Runner` | `runner.go:17-22` | Execution orchestrator |
| `RunOptions` | `runner.go:24-43` | Runtime options |

**Orchestration Flow**:
```
Runner.Run()
    │
    ├── 1. Generator.Generate() → _entrypoint.py
    │
    ├── 2. Determine isolation settings
    │       ├── findIsolateBinary()
    │       └── Set UseIsolate, IsolatePath
    │
    ├── 3. Build ExecuteOptions
    │       ├── Memory: MemoryTarget, MemoryLimit, SwapEnabled
    │       └── Timeout: IdleTimeout, MaxTimeout, ActivityCheck
    │
    ├── 4. Proxy.Execute()
    │
    ├── 5. Cleanup _entrypoint.py (unless KeepEntrypoint)
    │
    └── 6. Return *Result
```

**Isolate Binary Search Order** (ref: `runner.go:129-162`):
1. `./isolate/bin/isolate`
2. `./isolation/bin/isolate`
3. `../isolation/bin/isolate`
4. `~/.agentscale/bin/isolate`
5. `/usr/local/bin/isolate`

---

### 3.5 Scaling Package (`pkg/scaling/`)

**Purpose**: Queue-based autoscaling engine for multi-agent deployments.

**Key Interfaces** (ref: `interfaces.go`):

| Interface | Description |
|-----------|-------------|
| `Worker` | Execute requests, report health/idle |
| `WorkerSpawner` | Create/destroy workers |
| `WorkerPool` | Manage worker lifecycle |
| `Autoscaler` | Coordinate scaling decisions |
| `QueueMetrics` | Report queue depth |

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `BasicAutoscaler` | `autoscaler.go` | Scaling engine |
| `BasicWorkerPool` | `pool.go` | Worker pool manager |
| `RequestQueue` | `queue.go` | FIFO request queue |
| `AgentWorker` | `worker.go` | Runner-backed worker |
| `ScalingPolicy` | `policy.go` | Scaling thresholds |

**Scaling Algorithm**:
```
Every autoscaler_interval (default 5s):
    │
    ├── For each registered pool:
    │       │
    │       ├── pending = queue.PendingCount()
    │       ├── processing = queue.ProcessingCount()
    │       ├── current = pool.CurrentSize()
    │       │
    │       ├── utilization = (pending + processing) / current
    │       │
    │       ├── IF utilization > ScaleUpThreshold (default 3.0)
    │       │       └── Scale UP (respect ScaleUpDelay)
    │       │
    │       └── IF utilization < ScaleDownThreshold (default 0.5)
    │               └── Scale DOWN (respect ScaleDownDelay)
    │
    └── maintenanceLoop() spawns/kills workers to match desired size
```

**Key Insight**: Queue-based, not CPU-based. AI agents spend most time waiting on LLM APIs, making CPU metrics meaningless.

---

### 3.6 Server Package (`pkg/server/`)

**Purpose**: HTTP server orchestrating multiple agents with shared autoscaler.

**Key Types**:

| Type | File | Description |
|------|------|-------------|
| `Server` | `server.go` | HTTP server |
| `AgentInstance` | `server.go` | Per-agent components |

**HTTP Routes**:

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/invoke?agent=<id>` | `handleInvoke` | Execute agent |
| GET | `/health` | `handleHealth` | List all agents |
| GET | `/stats` | `handleStats` | All agent stats |
| GET | `/stats?agent=<id>` | `handleStats` | Specific agent stats |

**Server Architecture**:
```
┌─────────────────────────────────────────────────────────────┐
│                         Server                               │
├─────────────────────────────────────────────────────────────┤
│  Shared: BasicAutoscaler                                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │ AgentInstance 1 │  │ AgentInstance 2 │  ...             │
│  ├─────────────────┤  ├─────────────────┤                  │
│  │ RequestQueue    │  │ RequestQueue    │                  │
│  │ WorkerPool      │  │ WorkerPool      │                  │
│  │ Context         │  │ Context         │                  │
│  │ workerLoop()    │  │ workerLoop()    │                  │
│  └─────────────────┘  └─────────────────┘                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Request Flow**:
```
HTTP POST /invoke?agent=agent1
    │
    ├── Parse JSON body
    ├── Find AgentInstance by ID
    ├── Create Request with response channel
    ├── queue.Enqueue(request)
    │
    │   [Background: workerLoop]
    │       ├── queue.Dequeue()
    │       ├── pool.GetIdleWorker()
    │       ├── worker.Execute(request)
    │       └── request.ResponseChan <- result
    │
    ├── <-request.ResponseChan (blocking)
    └── HTTP Response (JSON)
```

---

## 4. Execution Flow

### 4.1 Single Agent Execution (CLI)

```
┌──────────────────────────────────────────────────────────────────┐
│ User: agentscale run ./my-agent                                  │
│       stdin: {"query": "hello"}                                  │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Python CLI: cli/agentscale/commands/run.py                       │
│ - Find agentscale-runtime binary                                 │
│ - Build command with flags                                       │
│ - Pipe stdin to subprocess                                       │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Go Runtime: cmd/agentscale-runtime/main.go                       │
│ - Parse flags (--memory, --timeout, --no-isolate)                │
│ - config.Load(agentDir)                                          │
│ - runner.RunFromDir()                                            │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Runner: pkg/runner/runner.go                                     │
│ - Generator.Generate() → _entrypoint.py                          │
│ - Find isolate binary                                            │
│ - Build ExecuteOptions                                           │
│ - Proxy.Execute()                                                │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Proxy: pkg/proxy/executor.go                                     │
│ - buildCommand() with isolation flags                            │
│ - Set stdin, capture stdout/stderr                               │
│ - Activity monitoring (if enabled)                               │
│ - Process completion/timeout                                     │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Isolation: isolate run [flags] "python3 _entrypoint.py"          │
│ - Linux: Namespace + cgroups isolation                           │
│ - macOS: VM execution via vsock                                  │
└────────────────────────────────┬─────────────────────────────────┘
                                 │
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│ Agent Runtime: _entrypoint.py                                    │
│ - Read JSON from stdin                                           │
│ - Import and call handler()                                      │
│ - Write JSON to stdout                                           │
└──────────────────────────────────────────────────────────────────┘
```

### 4.2 Data Transformation Pipeline

```
User Input (JSON string)
    │
    ▼
cmd.Stdin = strings.NewReader(input)
    │
    ▼
Python: sys.stdin.read()
    │
    ▼
json.loads() → dict
    │
    ▼
[Optional] InputType.model_validate() → Pydantic model
    │
    ▼
handler(input) → dict/Pydantic
    │
    ▼
[If Pydantic] output.model_dump()
    │
    ▼
json.dumps() → string
    │
    ▼
print() → stdout
    │
    ▼
Go: bytes.Buffer captures stdout
    │
    ▼
json.Unmarshal() → map[string]interface{}
    │
    ▼
Result{Status, Output, Duration, ...}
    │
    ▼
OutputJSON() → JSON string with metadata
```

---

## 5. Isolation Architecture

### 5.1 Linux Isolation

**Location**: `isolate/cmd/isolate/container_linux.go`

**Namespace Stack**:

| Namespace | Flag | Purpose |
|-----------|------|---------|
| PID | `CLONE_NEWPID` | Process sees itself as PID 1 |
| Mount | `CLONE_NEWNS` | Isolated filesystem mounts |
| UTS | `CLONE_NEWUTS` | Custom hostname ("container") |
| IPC | `CLONE_NEWIPC` | Isolated IPC resources |
| Network | `CLONE_NEWNET` | Isolated networking (optional) |

**Re-exec Pattern**:
```
Parent Process
    │
    ├── exec.Command("/proc/self/exe", "child", ...)
    │
    ├── SysProcAttr.Cloneflags = CLONE_NEW*
    │
    ├── cmd.Start() ──────────────────┐
    │                                 │
    │   Child Process (inside namespace)
    │       │
    │       ├── setupPivotRoot() if rootfs provided
    │       │
    │       ├── Mount /proc
    │       │
    │       └── exec.Command("/bin/sh", "-c", command)
    │
    └── cmd.Wait()
```

**Filesystem Isolation** (ref: `container_linux.go:172-216`):
```
1. Make root private: MS_PRIVATE | MS_REC
2. Bind mount new root: MS_BIND | MS_REC
3. Create /.old_root directory
4. syscall.PivotRoot(newRoot, oldRoot)
5. chdir("/")
6. Mount /proc
7. Unmount /.old_root with MNT_DETACH
8. Remove /.old_root
```

### 5.2 Cgroups v2 Resource Limits

**Location**: `isolate/pkg/cgroups/cgroups.go`

**Cgroup Path**: `/sys/fs/cgroup/agentscale/{container-id}/`

**Memory Configuration**:

| File | Purpose | Example |
|------|---------|---------|
| `memory.max` | Hard limit (OOM kill) | `536870912` (512MB) |
| `memory.high` | Soft limit (throttle) | `268435456` (256MB) |
| `memory.swap.max` | Swap allowance | `268435456` (256MB) |

**CPU Configuration**:

| File | Purpose | Example |
|------|---------|---------|
| `cpu.max` | Quota per period | `50000 100000` (50%) |

**Process Limit**:

| File | Purpose | Example |
|------|---------|---------|
| `pids.max` | Max processes | `100` |

### 5.3 macOS VM Isolation

**Location**: `isolate/pkg/vm/vm_darwin.go`

**Architecture**:
```
┌─────────────────────────────────────────────────────┐
│                    Host (macOS)                      │
├─────────────────────────────────────────────────────┤
│  isolate vm start                                   │
│      │                                              │
│      ▼                                              │
│  Apple Virtualization.framework                     │
│      │                                              │
│      ▼                                              │
│  ┌───────────────────────────────────────────────┐ │
│  │              VM (PUI PUI Linux)               │ │
│  ├───────────────────────────────────────────────┤ │
│  │  Kernel: vmlinuz (minimal Linux)             │ │
│  │  Initrd: python-3.10.initrd.gz               │ │
│  │                                               │ │
│  │  Running: vsock-agent (port 1024)            │ │
│  │      │                                        │ │
│  │      ▼                                        │ │
│  │  Receives: {command, stdin, env}              │ │
│  │  Returns:  {stdout, stderr, exit_code}        │ │
│  └───────────────────────────────────────────────┘ │
│                        ▲                            │
│                        │ vsock                      │
│                        ▼                            │
│  isolate run "python3 agent.py"                    │
└─────────────────────────────────────────────────────┘
```

**vsock Protocol** (ref: `isolate/cmd/vsock-agent/main.go`):
- Port: 1024
- Format: 4-byte length prefix + JSON payload
- Max request: 10MB
- Max response: 100MB

---

## 6. Multi-Agent Server

### 6.1 Server Initialization Flow

```
main() [cmd/agentscale-server/main.go]
    │
    ├── config.LoadServerConfig(configPath)
    │
    ├── server.New(serverCfg)
    │       │
    │       ├── Create BasicAutoscaler (shared)
    │       │
    │       └── For each agent in config:
    │               ├── config.Load(agent.AgentDir)
    │               ├── Create RequestQueue
    │               ├── Create BasicWorkerPool
    │               ├── Create AgentWorker spawner
    │               ├── Register with autoscaler
    │               └── Store as AgentInstance
    │
    ├── server.Start()
    │       │
    │       ├── autoscaler.Start()
    │       │
    │       ├── For each agent:
    │       │       ├── Spawn initial workers
    │       │       └── Start workerLoop goroutines
    │       │
    │       └── http.ListenAndServe()
    │
    └── Graceful shutdown on SIGINT/SIGTERM
```

### 6.2 Request Processing Flow

```
HTTP POST /invoke?agent=agent1
         │
         ▼
    ┌────────────────────────────────────────────────────┐
    │              handleInvoke()                         │
    │  1. Parse agent ID from query                      │
    │  2. Parse JSON body                                │
    │  3. Find AgentInstance                             │
    │  4. Create Request{Input, ResponseChan}            │
    │  5. queue.Enqueue(request)                         │
    └────────────────────────────────────────────────────┘
                    │
                    ▼
    ┌────────────────────────────────────────────────────┐
    │              RequestQueue                           │
    │  FIFO queue with pending/processing tracking       │
    └────────────────────────────────────────────────────┘
                    │
                    ▼
    ┌────────────────────────────────────────────────────┐
    │              workerLoop() [goroutine]              │
    │  for {                                             │
    │      request := queue.Dequeue() // blocking        │
    │      worker := pool.GetIdleWorker()                │
    │      result := worker.Execute(request)             │
    │      request.ResponseChan <- result                │
    │      pool.ReturnWorker(worker)                     │
    │  }                                                 │
    └────────────────────────────────────────────────────┘
                    │
                    ▼
    ┌────────────────────────────────────────────────────┐
    │              AgentWorker.Execute()                  │
    │  - Runner.Run() with request input                 │
    │  - Returns *proxy.Result                           │
    └────────────────────────────────────────────────────┘
                    │
                    ▼
    ┌────────────────────────────────────────────────────┐
    │              HTTP Response                          │
    │  <- request.ResponseChan                           │
    │  JSON: {status, output, duration}                  │
    └────────────────────────────────────────────────────┘
```

---

## 7. Agent-Native Features

### 7.1 Graceful Memory Degradation

**Concept**: Instead of OOM killing agents at memory limit, provide a degradation path.

**Memory Tiers**:
```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  0 ──────────► Target ──────────► Limit ──────────► ∞      │
│                                                             │
│  [====== Fast (RAM) ======][=== Slow (Swap) ===][  OOM  ]  │
│                                                             │
│  Example: 0-256MB fast, 256-512MB swap, >512MB kill        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Configuration** (ref: `pkg/config/config.go:18-21`):

| Field | YAML | Default | Description |
|-------|------|---------|-------------|
| `Memory` | `memory` | 256 | Target MB (fast tier) |
| `MemoryLimit` | `memory_limit` | 512 | Hard limit MB |
| `SwapEnabled` | `swap_enabled` | true | Enable swap buffer |

**Cgroups Mapping**:
- `memory.high` = Target (soft limit, triggers throttling)
- `memory.max` = Limit (hard limit, OOM kill)
- `memory.swap.max` = Limit - Target (swap allowance)

### 7.2 Activity-Based Timeout

**Concept**: Kill stuck agents (no output activity) rather than just enforcing max time.

**Configuration** (ref: `pkg/config/config.go:23-26`):

| Field | YAML | Default | Description |
|-------|------|---------|-------------|
| `TimeoutSec` | `timeout` | 300 | Max total time (seconds) |
| `IdleTimeoutSec` | `idle_timeout` | 60 | No activity timeout |
| `ActivityCheckSec` | `activity_check` | 5 | Check interval |

**ActivityMonitor Flow** (ref: `pkg/proxy/activity.go`):
```
┌─────────────────────────────────────────────────────────────┐
│                    ActivityMonitor                           │
├─────────────────────────────────────────────────────────────┤
│  lastActivity: time.Time  ◄─── Reset on each stdout/stderr │
│  startTime: time.Time     ◄─── Process start time          │
│  idleTimeout: 60s                                           │
│  maxTimeout: 300s                                           │
│  checkInterval: 5s                                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Background Ticker (every 5s):                              │
│      │                                                      │
│      ├── Check max timeout: now - startTime > maxTimeout?  │
│      │       └── YES → send "max_timeout" to channel       │
│      │                                                      │
│      └── Check idle: now - lastActivity > idleTimeout?     │
│              └── YES → send "idle_timeout" to channel      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Timeout Results**:
- `StatusTimeout` - Max timeout exceeded
- `StatusIdleTimeout` - No activity for idle period

### 7.3 Queue-Based Autoscaling

**Concept**: Scale on queue depth, not CPU usage (agents wait on LLM APIs).

**Scaling Formula**:
```
utilization = (pending_tasks + processing_tasks) / current_workers

IF utilization > scale_up_threshold (3.0):
    desired = min(current * 1.5, max_workers)

IF utilization < scale_down_threshold (0.5):
    desired = max(current - 1, min_workers)
```

**Configuration** (ref: `pkg/config/server_config.go`):

| Field | Default | Description |
|-------|---------|-------------|
| `min_workers` | 1 | Minimum worker count |
| `max_workers` | 10 | Maximum worker count |
| `scale_up_threshold` | 3.0 | Utilization to scale up |
| `scale_down_threshold` | 0.5 | Utilization to scale down |
| `scale_up_delay` | 10s | Cooldown after scale up |
| `scale_down_delay` | 30s | Cooldown after scale down |

---

## 8. Configuration System

### 8.1 Agent Configuration (`agent.yaml`)

```yaml
# Required
name: my-agent
runtime: python3
module: agent           # Python module (without .py)
entrypoint: handler     # Function name

# Optional - Input/Output
input_type: InputModel  # Pydantic model for validation

# Optional - Environment
env:
  - OPENAI_API_KEY=${OPENAI_API_KEY}

# Optional - Agent-Native Memory
memory: 256             # Target MB (fast tier)
memory_limit: 512       # Hard limit MB
swap_enabled: true      # Graceful degradation

# Optional - Agent-Native Timeout
timeout: 300            # Max total seconds
idle_timeout: 60        # No activity timeout
activity_check: 5       # Check interval
```

### 8.2 Server Configuration (`agentscale.yaml`)

```yaml
server:
  port: 8080
  autoscaler_interval: 5s

  isolation:
    enabled: true
    type: auto          # auto, namespace, vm
    defaults:
      memory_limit: 512mb
      timeout: 300s

agents:
  calculator:
    image: /path/to/calculator-image
    scaling:
      min_workers: 1
      max_workers: 10
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      queue_size: 100

  support-agent:
    image: /path/to/support-image
    scaling:
      min_workers: 2
      max_workers: 20
```

### 8.3 Defaults Application (ref: `pkg/config/defaults.go`)

| Category | Field | Default |
|----------|-------|---------|
| Memory | `Memory` | 256 MB |
| Memory | `MemoryLimit` | 512 MB |
| Memory | `SwapEnabled` | true |
| Timeout | `Timeout` | 300s |
| Timeout | `IdleTimeout` | 60s |
| Timeout | `ActivityCheck` | 5s |
| Runtime | `Runtime` | "python3" |

---

## 9. Data Contracts

### 9.1 Agent I/O Contract

**Input** (stdin):
```json
{
  "query": "user input",
  "context": { ... }
}
```

**Output** (stdout):
```json
{
  "response": "agent output",
  "metadata": { ... }
}
```

**Error Output** (stdout with exit 1):
```json
{
  "status": "error",
  "error": "error message",
  "traceback": "stack trace"
}
```

### 9.2 Result Structure (ref: `pkg/proxy/result.go:16-38`)

```
Result {
    Status:    "success" | "error" | "timeout" | "idle_timeout"
    Output:    map[string]interface{}  // Parsed JSON
    RawOutput: string                  // Raw stdout
    Error:     string                  // Error message
    Stderr:    string                  // stderr output
    ExitCode:  int                     // Process exit code
    Duration:  time.Duration           // Execution time
}
```

### 9.3 HTTP API Contracts

**POST /invoke**:
```
Request:
  Query: agent=<agent-id>
  Body: {"input": "..."}

Response:
  {
    "status": "success",
    "output": { ... },
    "duration": "1.234s"
  }
```

**GET /stats**:
```
Response:
  {
    "agent1": {
      "queue_pending": 5,
      "queue_processing": 2,
      "workers_current": 3,
      "workers_desired": 4
    }
  }
```

---

## 10. File Reference Map

### 10.1 Core Execution

| Component | File |
|-----------|------|
| Runtime CLI | `cmd/agentscale-runtime/main.go` |
| Server CLI | `cmd/agentscale-server/main.go` |
| Runner | `pkg/runner/runner.go` |
| Executor | `pkg/proxy/executor.go` |
| Activity Monitor | `pkg/proxy/activity.go` |
| Result Types | `pkg/proxy/result.go` |
| Generator | `pkg/generator/generator.go` |
| Templates | `pkg/generator/templates.go` |

### 10.2 Configuration

| Component | File |
|-----------|------|
| Agent Config | `pkg/config/config.go` |
| Server Config | `pkg/config/server_config.go` |
| Loader | `pkg/config/loader.go` |
| Defaults | `pkg/config/defaults.go` |
| Validator | `pkg/config/validator.go` |

### 10.3 Scaling

| Component | File |
|-----------|------|
| Interfaces | `pkg/scaling/interfaces.go` |
| Autoscaler | `pkg/scaling/autoscaler.go` |
| Worker Pool | `pkg/scaling/pool.go` |
| Request Queue | `pkg/scaling/queue.go` |
| Agent Worker | `pkg/scaling/worker.go` |

### 10.4 Server

| Component | File |
|-----------|------|
| HTTP Server | `pkg/server/server.go` |
| Handlers | `pkg/server/handlers.go` |

### 10.5 Isolation

| Component | File |
|-----------|------|
| Isolate CLI | `isolate/cmd/isolate/main.go` |
| Linux Container | `isolate/cmd/isolate/container_linux.go` |
| macOS Stub | `isolate/cmd/isolate/container_darwin.go` |
| Cgroups | `isolate/pkg/cgroups/cgroups.go` |
| Rootfs | `isolate/pkg/rootfs/rootfs.go` |
| Container | `isolate/pkg/container/container.go` |
| VM (macOS) | `isolate/pkg/vm/vm_darwin.go` |
| vsock Agent | `isolate/cmd/vsock-agent/main.go` |

### 10.6 CLI (Python)

| Component | File |
|-----------|------|
| Main | `cli/agentscale/main.py` |
| Run Command | `cli/agentscale/commands/run.py` |
| Deploy Command | `cli/agentscale/commands/deploy.py` |
| Client Commands | `cli/agentscale/commands/client.py` |
| Builder | `cli/agentscale/utils/builder.py` |

---

## Summary

AgentScale implements a complete agent execution platform with:

1. **Dual-mode isolation** - Linux namespaces/cgroups OR macOS VM
2. **Agent-native memory** - Graceful degradation instead of OOM kill
3. **Activity-based timeout** - Detect stuck agents by lack of output
4. **Queue-based scaling** - Scale on demand, not CPU
5. **Multi-agent orchestration** - Single server, multiple agents, shared autoscaler

The architecture cleanly separates:
- **CLI layer** (Python) - User interface
- **Execution layer** (Go) - Core runtime
- **Isolation layer** (Go) - Process containment
- **Agent runtime** (Python) - Generated entrypoint + user code
