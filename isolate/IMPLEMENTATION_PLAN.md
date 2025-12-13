# Isolation Layer - Implementation Plan

**Goal**: Build a standalone namespace isolation system that can run any process in a secure, resource-limited container.

**Language**: Go (leverages syscall package for Linux namespaces)

**Platform**: Linux only (namespaces are Linux kernel feature)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      CLI Entry Point                        │
│                     (main.go)                               │
│                                                             │
│   ./isolate run --memory=512M --cpu=0.5 "python script.py"  │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Container Manager                        │
│                   (container.go)                            │
│                                                             │
│   - Orchestrates the full lifecycle                         │
│   - Coordinates namespace + cgroups + rootfs                │
│   - Handles cleanup on exit                                 │
└─────────────────────────────────────────────────────────────┘
                            │
            ┌───────────────┼───────────────┐
            ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│   Namespace     │ │    Cgroups      │ │    Rootfs       │
│ (namespace.go)  │ │  (cgroups.go)   │ │  (rootfs.go)    │
│                 │ │                 │ │                 │
│ - PID namespace │ │ - Memory limit  │ │ - pivot_root    │
│ - Mount namespace│ │ - CPU limit    │ │ - Mount /proc   │
│ - UTS namespace │ │ - PID limit     │ │ - Mount /dev    │
│ - Clone flags   │ │ - cgroups v2    │ │ - Bind mounts   │
└─────────────────┘ └─────────────────┘ └─────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Process Executor                         │
│                    (executor.go)                            │
│                                                             │
│   - Runs inside namespace                                   │
│   - Handles stdin/stdout/stderr                             │
│   - Captures exit code                                      │
└─────────────────────────────────────────────────────────────┘
```

---

## File Structure

```
isolation/
├── cmd/
│   └── isolate/
│       └── main.go           # CLI entry point
├── pkg/
│   ├── container/
│   │   └── container.go      # Container lifecycle manager
│   ├── namespace/
│   │   └── namespace.go      # Linux namespace setup
│   ├── cgroups/
│   │   └── cgroups.go        # Resource limits (cgroups v2)
│   ├── rootfs/
│   │   └── rootfs.go         # Filesystem isolation
│   └── executor/
│       └── executor.go       # Process execution
├── rootfs/                   # Minimal filesystem (downloaded/built)
│   └── README.md             # Instructions for rootfs setup
├── go.mod
├── go.sum
└── IMPLEMENTATION_PLAN.md    # This file
```

---

## Implementation Tasks

### Phase 1: Foundation

#### Task 1.1: Project Setup
**What**: Initialize Go module, create directory structure
**Files**: go.mod, directory structure
**Acceptance**:
- `go mod init` works
- All directories exist
- Basic main.go compiles

#### Task 1.2: CLI Scaffolding
**What**: Basic CLI that parses arguments
**Files**: cmd/isolate/main.go
**Acceptance**:
- `./isolate run "echo hello"` parses command
- `--memory`, `--cpu` flags parse correctly
- Help text shows usage

---

### Phase 2: Namespace Isolation

#### Task 2.1: Basic Namespace Creation
**What**: Create process with new PID, UTS, Mount namespaces
**Files**: pkg/namespace/namespace.go
**Depends on**: Task 1.2
**Technique**:
- Use `syscall.SysProcAttr` with `Cloneflags`
- `CLONE_NEWPID` - New PID namespace
- `CLONE_NEWUTS` - New hostname
- `CLONE_NEWNS` - New mount namespace
**Acceptance**:
- Child process sees PID 1 (itself)
- Child has different hostname than host
- Child mounts don't affect host

#### Task 2.2: Re-exec Pattern (Liz Rice approach)
**What**: Implement the `/proc/self/exe` re-exec pattern
**Files**: pkg/namespace/namespace.go, cmd/isolate/main.go
**Depends on**: Task 2.1
**Technique**:
- Parent calls `run` → creates namespaces
- Executes `/proc/self/exe child` inside namespace
- Child (`child` subcommand) does actual setup
**Why**:
- Namespace setup must happen AFTER clone but BEFORE exec
- Re-exec pattern makes this clean
**Acceptance**:
- `./isolate run` triggers `./isolate child` inside namespace
- Child process is PID 1 in its namespace

---

### Phase 3: Filesystem Isolation

#### Task 3.1: Rootfs Preparation
**What**: Create/download minimal filesystem
**Files**: rootfs/README.md, script or documentation
**Options**:
- Option A: Download Alpine minirootfs (~3MB)
- Option B: Build custom rootfs with just /bin, /lib
- Option C: Use debootstrap for Debian minimal
**Recommendation**: Start with Alpine minirootfs (smallest, easiest)
**Acceptance**:
- rootfs/ directory has working /bin/sh
- Can manually chroot into it

#### Task 3.2: pivot_root Implementation
**What**: Change root filesystem to our rootfs
**Files**: pkg/rootfs/rootfs.go
**Depends on**: Task 2.2, Task 3.1
**Technique**:
- Mount rootfs as new root
- `pivot_root` to switch
- Unmount old root
- `chdir("/")` to new root
**Acceptance**:
- Process can't see host filesystem
- Process sees rootfs as /
- `/etc/passwd` shows rootfs version, not host

#### Task 3.3: Essential Mounts
**What**: Mount /proc, /dev inside namespace
**Files**: pkg/rootfs/rootfs.go
**Depends on**: Task 3.2
**Technique**:
- `mount("proc", "/proc", "proc", 0, "")`
- `mount("tmpfs", "/dev", "tmpfs", 0, "")`
- Create essential /dev nodes (null, zero, random)
**Acceptance**:
- `ps` works inside container (needs /proc)
- `/dev/null` works inside container

---

### Phase 4: Resource Limits (Cgroups)

#### Task 4.1: Cgroups v2 Setup
**What**: Create cgroup for container
**Files**: pkg/cgroups/cgroups.go
**Depends on**: Task 2.1
**Technique**:
- Create dir under `/sys/fs/cgroup/`
- Write PID to `cgroup.procs`
- Container process now in cgroup
**Acceptance**:
- Container PID appears in cgroup.procs
- Cgroup exists while container runs
- Cgroup cleaned up after exit

#### Task 4.2: Memory Limit
**What**: Enforce memory limit
**Files**: pkg/cgroups/cgroups.go
**Depends on**: Task 4.1
**Technique**:
- Write limit to `memory.max`
- Write limit to `memory.high` (soft limit, for warnings)
**Acceptance**:
- Container with 100MB limit dies when allocating 150MB
- Host unaffected by container OOM

#### Task 4.3: CPU Limit
**What**: Enforce CPU limit
**Files**: pkg/cgroups/cgroups.go
**Depends on**: Task 4.1
**Technique**:
- Write to `cpu.max` (format: "quota period")
- Example: "50000 100000" = 50% CPU
**Acceptance**:
- Container with 50% CPU limit shows ~50% in top
- Infinite loop in container doesn't starve host

#### Task 4.4: Process Limit (Fork Bomb Protection)
**What**: Limit number of processes
**Files**: pkg/cgroups/cgroups.go
**Depends on**: Task 4.1
**Technique**:
- Write to `pids.max`
**Acceptance**:
- Fork bomb in container fails
- Container can't spawn more than N processes

---

### Phase 5: Process Execution

#### Task 5.1: Basic Command Execution
**What**: Run arbitrary command inside container
**Files**: pkg/executor/executor.go
**Depends on**: Phase 2, Phase 3
**Technique**:
- After namespace/rootfs setup, exec the command
- `syscall.Exec` replaces process with command
**Acceptance**:
- `./isolate run "echo hello"` prints "hello"
- `./isolate run "ls /"` shows rootfs contents

#### Task 5.2: Stdin/Stdout Handling
**What**: Pipe stdin/stdout properly
**Files**: pkg/executor/executor.go
**Depends on**: Task 5.1
**Technique**:
- Connect parent's stdin to child's stdin
- Capture child's stdout/stderr
**Acceptance**:
- Can pipe input: `echo "test" | ./isolate run "cat"`
- Output from container reaches parent

#### Task 5.3: Exit Code Handling
**What**: Capture and return exit code
**Files**: pkg/executor/executor.go
**Depends on**: Task 5.2
**Technique**:
- Wait for child process
- Extract exit code from wait status
**Acceptance**:
- `./isolate run "exit 42"` → exit code 42
- OOM kill → distinguishable exit code

---

### Phase 6: Container Lifecycle Manager

#### Task 6.1: Container Orchestrator
**What**: Tie everything together
**Files**: pkg/container/container.go
**Depends on**: All previous tasks
**Technique**:
- Sequence: Create cgroup → Spawn with namespaces → Setup rootfs → Execute
- Handle errors at each stage
- Cleanup on failure or completion
**Acceptance**:
- Single `container.Run()` call does everything
- Partial failures clean up properly

#### Task 6.2: Cleanup Handler
**What**: Ensure cleanup on any exit
**Files**: pkg/container/container.go
**Depends on**: Task 6.1
**Technique**:
- Defer cleanup functions
- Handle SIGTERM/SIGINT
- Remove cgroup directory
- Unmount filesystems
**Acceptance**:
- Ctrl+C cleans up properly
- No orphaned cgroups after runs
- No mounted filesystems left behind

---

### Phase 7: Testing & Validation

#### Task 7.1: Basic Tests
**What**: Unit tests for each component
**Files**: *_test.go files
**Tests**:
- Namespace created with correct flags
- Cgroup limits written correctly
- Rootfs mounted properly

#### Task 7.2: Integration Tests
**What**: End-to-end container tests
**Files**: Integration test file
**Tests**:
- Run simple command, verify output
- Run memory hog, verify OOM
- Run fork bomb, verify limit
- Try to escape, verify isolation

#### Task 7.3: Security Validation
**What**: Verify isolation boundaries
**Tests**:
- Container can't see host processes (`ps aux`)
- Container can't read host files (`cat /etc/shadow`)
- Container can't affect host network (later)
- Container dies on resource exhaustion

---

## Task Dependency Graph

```
Phase 1: Foundation
    1.1 Project Setup
         │
         ▼
    1.2 CLI Scaffolding
         │
         ▼
Phase 2: Namespaces ─────────────────────────┐
    2.1 Basic Namespace                      │
         │                                   │
         ▼                                   │
    2.2 Re-exec Pattern                      │
         │                                   │
         ├───────────────┬───────────────────┤
         ▼               ▼                   │
Phase 3: Rootfs    Phase 4: Cgroups          │
    3.1 Rootfs Prep     4.1 Cgroup Setup     │
         │                   │               │
         ▼                   ├───────┬───────┤
    3.2 pivot_root          ▼       ▼       ▼
         │              4.2 Mem  4.3 CPU  4.4 PIDs
         ▼                   │       │       │
    3.3 Essential Mounts     └───────┴───────┘
         │                           │
         └───────────────┬───────────┘
                         ▼
Phase 5: Execution
    5.1 Basic Command Exec
         │
         ▼
    5.2 Stdin/Stdout
         │
         ▼
    5.3 Exit Code
         │
         ▼
Phase 6: Container Manager
    6.1 Orchestrator
         │
         ▼
    6.2 Cleanup Handler
         │
         ▼
Phase 7: Testing
    7.1 Unit Tests
         │
         ▼
    7.2 Integration Tests
         │
         ▼
    7.3 Security Validation
```

---

## Success Criteria (POC Complete)

**Must have:**
- [ ] Can run `./isolate run "echo hello"` and get "hello"
- [ ] Process is PID 1 inside container
- [ ] Process can't see host filesystem
- [ ] Process can't see host processes
- [ ] Memory limit enforced (OOM kills container, not host)
- [ ] CPU limit enforced
- [ ] Clean cleanup (no orphaned cgroups/mounts)

**Nice to have:**
- [ ] Python works inside container (rootfs has Python)
- [ ] Multiple containers can run simultaneously
- [ ] Container startup < 50ms

---

## Technical Notes

### Why Re-exec Pattern?

The challenge: namespace setup happens at `clone()` time, but rootfs setup needs to happen AFTER we're in the namespace.

```
Wrong approach:
    parent: setup rootfs → clone with namespaces
    Problem: rootfs setup happens in parent's namespace!

Correct approach (re-exec):
    parent: clone with namespaces, exec "/proc/self/exe child"
    child: (now in namespace) setup rootfs, exec actual command
```

### Cgroups v2 vs v1

- Cgroups v2 is unified hierarchy (simpler)
- Most modern systems (Ubuntu 22.04+, Fedora, etc.) default to v2
- We target v2 only for simplicity
- Detection: check if `/sys/fs/cgroup/cgroup.controllers` exists

### Rootfs Options

**Alpine minirootfs** (Recommended for POC):
- 3MB download
- Has /bin/sh, basic utils
- Easy to add Python later
- Download: https://alpinelinux.org/downloads/ → Mini root filesystem

**Custom minimal** (For production):
- Just what we need
- Smaller attack surface
- More work to build

---

## Questions to Resolve Before Starting

1. **Rootfs strategy**: Download Alpine during build, or expect it pre-installed?
2. **Error handling**: Panic on errors, or return errors up the chain?
3. **Logging**: Silent, verbose flag, or always log?

---

## Estimated Task Breakdown

| Phase | Tasks | Complexity |
|-------|-------|------------|
| Phase 1 | 2 | Low |
| Phase 2 | 2 | Medium |
| Phase 3 | 3 | Medium-High |
| Phase 4 | 4 | Medium |
| Phase 5 | 3 | Low-Medium |
| Phase 6 | 2 | Medium |
| Phase 7 | 3 | Low |
| **Total** | **19 tasks** | |

---

**Ready for review.**
