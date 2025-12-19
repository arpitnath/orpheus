# AgentScale Image System - Architecture & Implementation Plan

**Version**: v0.1.0
**Created**: December 19, 2025
**Status**: Design Complete - Ready for Implementation
**Type**: High-Level Design (HLD)

---

## Executive Summary

### Current State

AgentScale has multi-agent server with queue-based autoscaling, but containers use host filesystem (partial isolation).

### Target State

True container isolation with AgentScale images - filesystems containing runtime and agent code, isolated from host.

### Approach

**Simple over complex**: Complete image per agent (no layer sharing), declarative YAML specs, auto-detection.

**Effort**: ~11 hours across 5 phases

---

## System Architecture

### Three-Layer Model

**Layer 1: Base Images**
- Immutable runtime filesystems (python-3.10, node-18)
- Shared starting point for all agents
- Stored at: `~/.agentscale/images/`
- Pre-built and shipped with release

**Layer 2: Agent Images**
- Complete filesystems with runtime + dependencies + code
- One per deployed agent
- Stored at: `~/.agentscale/agents/`
- Built at deploy time

**Layer 3: Container Runtime**
- Existing `isolate` binary
- Creates containers from images
- Platform-specific (namespaces on Linux, VM on macOS)

### Component Interaction

**Installation Flow**:
Install AgentScale → Extract base images → Ready to deploy

**Deployment Flow**:
Deploy agent → Build agent image from base → Register with server

**Execution Flow**:
Invoke agent → Worker uses agent image → Container isolation → Execute

---

## Image Types and Structure

### Base Image (Runtime Only)

**Purpose**: Starting point with just the runtime

**Linux Structure**:
Directory at `~/.agentscale/images/python-3.10/`
- Binary: Python 3.10 executable
- Libraries: Python standard library
- System: Minimal Alpine Linux base (busybox, libc)

**macOS Structure**:
Compressed file at `~/.agentscale/images/python-3.10.initrd.gz`
- Binary: Python 3.10 executable
- Libraries: Python standard library
- System: PUI PUI Linux kernel resources
- Agent: vsock-agent for host communication

**Size**: ~40MB per runtime version

**Count**: Ship with one (python-3.10), users can build others

---

### Agent Image (Complete Package)

**Purpose**: Ready-to-execute agent with everything included

**Structure**:
Directory at `~/.agentscale/agents/<agent-id>/`
- runtime/ - Copy of base image Python
- packages/ - Agent's pip dependencies
- agent/ - Agent's complete code directory
- manifest.json - Metadata

**Size**: ~50-150MB depending on dependencies

**Count**: One per deployed agent

**Build**: At deploy time, from base + agent code + dependencies

---

## Image Specification Format

### Declarative YAML

**File naming**: `<runtime>-<version>.yaml` (e.g., `python-3.10.yaml`)

**Required fields**:
- name: Image identifier
- runtime.type: Runtime type (python3, node)
- runtime.version: Specific version (3.10.19)
- base.os: Base OS (alpine)

**Optional fields**:
- dependencies.python: pip packages
- dependencies.system: apk/apt packages
- env: Environment variables
- optimization: Build optimizations

**Purpose**: Declarative specification for building base images

**Used by**: Image builder system (future), currently we build manually

---

## Platform-Specific Considerations

### Linux (Namespace Isolation)

**Mechanism**: pivot_root to custom rootfs

**Steps**:
1. Create namespaces (pid, net, mount, uts, ipc)
2. Mount cgroups (memory, cpu limits)
3. Bind mount rootfs directory
4. pivot_root to rootfs
5. Mount essential filesystems (/proc, /dev)
6. Execute command inside

**Agent image**: Directory with complete filesystem

**Isolation**: Process sees only image filesystem, cannot access host

---

### macOS (VM Isolation)

**Mechanism**: Virtualization.framework with custom initrd

**Steps**:
1. Boot VM with initrd as root filesystem
2. Kernel loads, initrd unpacked to RAM
3. vsock-agent starts
4. Host sends command via vsock
5. Execute command inside VM
6. Return results via vsock

**Agent image**: Compressed initrd file

**Isolation**: Separate OS, complete isolation

---

## Implementation Phases

### Phase 1: Base Image Builder (Linux)

**Goal**: Create python-3.10 base image for Linux

**Tasks**:
1. Download Alpine Linux minirootfs
2. Download Python 3.10 from python-build-standalone (MUSL variant)
3. Extract both to temporary directory
4. Copy Python binaries to Alpine filesystem
5. Set up paths and symlinks
6. Remove unnecessary files (tests, docs)
7. Package as directory
8. Save to `~/.agentscale/images/python-3.10/`
9. Create manifest.json with metadata
10. Verify: Can execute python3 --version inside image

**Deliverable**: Working python-3.10 base image for Linux

**Effort**: ~2 hours

**Success criteria**:
- Image exists at expected location
- Contains python3 binary
- Standard library included
- Can execute inside container

---

### Phase 2: Base Image Builder (macOS)

**Goal**: Create python-3.10 base image for macOS VM

**Tasks**:
1. Locate PUI PUI Linux base initrd
2. Download Python 3.10 from python-build-standalone (MUSL variant)
3. Extract base initrd to temporary directory
4. Copy Python binaries into initrd filesystem
5. Update init script for Python paths
6. Add vsock-agent (already in place)
7. Remove unnecessary files (tests, docs)
8. Repack as compressed initrd
9. Save to `~/.agentscale/images/python-3.10.initrd.gz`
10. Verify: Boot VM, execute python3 --version

**Deliverable**: Working python-3.10 base image for macOS

**Effort**: ~2 hours

**Success criteria**:
- VM boots with new initrd
- Python3 available inside VM
- vsock communication works
- Can execute Python scripts

---

### Phase 3: Deploy Command Implementation

**Goal**: Build complete agent images from base + agent code

**Tasks**:
1. Create new CLI command: `agentscale deploy <agent-path>`
2. Parse agent directory for agent.yaml
3. Read runtime and version from agent.yaml
4. Resolve base image (python-3.10) based on runtime
5. Create agent image directory structure
6. Copy base image to agent image runtime/
7. Detect dependencies file (requirements.txt)
8. Execute pip install into agent image packages/
9. Copy entire agent code directory to agent image agent/
10. Generate manifest.json with metadata
11. Update or create agentscale.yaml entry
12. Validate agent image completeness
13. Return success with image location

**Deliverable**: Working deploy command that builds agent images

**Effort**: ~3 hours

**Success criteria**:
- Deploy creates complete agent image
- All dependencies installed
- All code files copied
- agentscale.yaml updated
- Can list deployed agents

---

### Phase 4: Executor Integration

**Goal**: Wire workers to use agent images with true isolation

**Tasks**:
1. Update AgentWorker to store agent image path
2. Modify AgentSpawner to receive image path
3. Update runner.RunOptions to include image path
4. Modify proxy.Execute to use --rootfs flag
5. Implement image path resolution logic
6. Update error handling for missing images
7. Remove macOS fallback warning code
8. Add validation that image exists before execution
9. Update logging to show image usage
10. Test isolation (verify cannot access host)

**Deliverable**: Workers execute from agent images with true isolation

**Effort**: ~2 hours

**Success criteria**:
- Workers use --rootfs flag
- Execute from agent filesystem
- Cannot access host files
- No fallback warnings
- Both platforms work

---

### Phase 5: Testing & Verification

**Goal**: Verify complete image system works end-to-end

**Tasks**:
1. Test base image build (both platforms)
2. Test deploy command (simple agent)
3. Test deploy command (agent with dependencies)
4. Test deploy command (complex directory structure)
5. Verify isolation (cannot read /etc/passwd from host)
6. Test multi-agent deployment
7. Verify autoscaling still works
8. Test graceful shutdown
9. Performance testing (startup time)
10. Documentation updates

**Deliverable**: Verified, working image system

**Effort**: ~2 hours

**Success criteria**:
- All tests pass
- Isolation verified
- No regressions
- Documentation updated

---

## Architecture Decisions

### Decision 1: Complete Image Per Agent

**Choice**: Each agent gets full copy of runtime + dependencies + code

**Alternative rejected**: Layer sharing, package deduplication

**Rationale**: Simplicity wins, disk is cheap, can optimize later

**Impact**: More storage used, but simpler implementation

---

### Decision 2: Deploy-Time Packaging

**Choice**: Build complete image when user runs deploy

**Alternative rejected**: Install dependencies at invoke time

**Rationale**: Fast invocation, no latency, predictable

**Impact**: Slower deploy, faster invoke

---

### Decision 3: YAML Declarative Format

**Choice**: Use YAML for image specifications

**Alternative rejected**: Shell scripts, Dockerfile format

**Rationale**: Consistent with existing configs, K8s-like, easy to parse

**Impact**: Need YAML parser and builder, but better UX

---

### Decision 4: Auto-Detection

**Choice**: Resolve image from agent.yaml runtime field

**Alternative rejected**: Manual --image flags

**Rationale**: Better developer experience, less configuration

**Impact**: Need resolution logic, but simpler usage

---

### Decision 5: Local-Only Images (v0.1.0)

**Choice**: No image registry, push/pull for v0.1.0

**Alternative rejected**: Build registry now

**Rationale**: OSS self-hosted doesn't need registry, defer to v0.2

**Impact**: Users build locally, but simpler system

---

## Storage Layout

### Filesystem Organization

**Root**: `~/.agentscale/`

**images/**: Base runtime images
- python-3.10/ (Linux directory)
- python-3.10.initrd.gz (macOS file)
- Future: python-3.11/, node-18/

**agents/**: Complete agent images
- agent-a/ (complete filesystem)
- agent-b/ (complete filesystem)
- Each contains: runtime/ + packages/ + agent/

**cache/**: Build cache
- Downloaded tarballs
- Temporary build artifacts

**bin/**: Binaries
- isolate
- agentscale-server
- agentscale-runtime

---

## Configuration Updates

### agent.yaml (No Change)

Existing format continues to work:
- name, runtime, entrypoint remain same
- Add runtime_version field (optional)

### agentscale.yaml (Minor Change)

**Current**: Points to code directories

**After**: Points to agent images OR auto-resolves

**Backward compatible**: Can support both path (code dir) and image (built image)

---

## Developer Experience

### Before (Current)

Installation:
- Install AgentScale
- Build binaries
- Manual setup

Usage:
- Edit agentscale.yaml manually
- Point to code directories
- Start server
- Partial isolation (warning on macOS)

---

### After (With Images)

Installation:
- Install AgentScale (one command)
- Base images included
- Ready immediately

Usage:
- Run: `agentscale deploy ./my-agent`
- System handles: building image, updating config
- Start server: just works
- True isolation (no warnings)

**Simpler, faster, more professional!**

---

## Risk Assessment

### Technical Risks

**Risk 1**: Python installation in initrd fails
- Mitigation: Use proven python-build-standalone binaries
- Fallback: Use Alpine apk packages

**Risk 2**: Dependency installation fails
- Mitigation: Validate requirements.txt before build
- Fallback: Error with clear message, don't deploy

**Risk 3**: Image size too large
- Mitigation: Strip tests and docs, compress
- Fallback: Accept larger size for v0.1.0

**Risk 4**: Platform differences break builds
- Mitigation: Test both platforms thoroughly
- Fallback: Ship best-effort, fix in patches

---

### Operational Risks

**Risk 1**: Users don't understand deploy flow
- Mitigation: Clear documentation, examples
- Fallback: Video tutorial, better error messages

**Risk 2**: Disk space issues (100 agents × 100MB)
- Mitigation: Document storage requirements
- Fallback: Add cleanup command, add warnings

**Risk 3**: Build failures on user machines
- Mitigation: Pre-built images in release, minimal build requirements
- Fallback: Provide pre-built agent images too

---

## Success Metrics

### Functional

**Must work**:
- Base image builds successfully (both platforms)
- Deploy creates complete agent images
- Containers execute from images (true isolation)
- Cannot access host filesystem
- Autoscaling still works
- Multi-agent still works

### Performance

**Acceptable**:
- Deploy time: < 2 minutes for typical agent
- Container startup: < 500ms
- Image build time: < 5 minutes
- Storage: < 100MB per agent

### Quality

**Requirements**:
- No warnings in logs
- Clear error messages
- Reproducible builds
- Cross-platform compatibility

---

## Rollback Plan

### If Image System Fails

**Fallback**: Current partial isolation continues to work
- Host filesystem still accessible
- System functional, just not fully isolated
- Can fix and release as patch

**No breaking changes**: Deploy command is new, existing flows unchanged

---

## Future Enhancements (Out of Scope)

### v0.2 Candidates

**Image Registry**:
- Central hosting for base images
- Pull/push commands
- Version management

**Layer Optimization**:
- Shared package cache
- Deduplication
- Union mounts

**Multi-Runtime**:
- Node.js support
- Python 3.11, 3.12
- Go runtime

**Advanced Features**:
- Custom base images
- Image variants (ml, web, data)
- Build caching
- Dependency locking

---

## Dependencies and Prerequisites

### Build-Time Dependencies

**For base image builder**:
- curl (download binaries)
- tar/gzip (extract archives)
- cpio (macOS initrd packing)
- Internet connection (download Python)

**For deploy command**:
- pip (install Python packages)
- Base image must exist
- Agent code directory with agent.yaml

### Runtime Dependencies

**For container execution**:
- Linux: Kernel with namespace support (>= 3.8)
- macOS: Virtualization.framework (macOS 11+)
- Sufficient disk space (~100MB per agent)
- Memory for worker pools

---

## Implementation Phases Summary

| Phase | Goal | Tasks | Effort |
|-------|------|-------|--------|
| 1 | Linux base image | Download, install Python, package | 2h |
| 2 | macOS base image | Build initrd with Python | 2h |
| 3 | Deploy command | Build agent images from base | 3h |
| 4 | Executor wiring | Use images with --rootfs | 2h |
| 5 | Testing | Verify isolation and functionality | 2h |

**Total**: ~11 hours

**Critical path**: Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5 (sequential)

---

## Validation Criteria

### Per-Phase Validation

**Phase 1**: Base image exists, Python executable, can run --version

**Phase 2**: VM boots with initrd, Python available, vsock works

**Phase 3**: Deploy creates image, all files present, config updated

**Phase 4**: Worker uses --rootfs, executes from image, host inaccessible

**Phase 5**: All integration tests pass, no regressions

### Final Validation

**System-level**:
- Install fresh → Deploy agent → Start server → Invoke → Success
- No host filesystem access
- No warnings in logs
- Works on Linux and macOS
- Multi-agent tested
- Autoscaling verified

---

## Migration Strategy

### For Existing Deployments

**Current**: agentscale.yaml points to code directories

**After images**: Points to built images

**Migration path**:
1. Run deploy for each existing agent
2. Updates agentscale.yaml automatically
3. Restart server
4. Existing agents now use images

**Backward compatibility**: Support both path (code) and image (built) in config

---

## Distribution Strategy

### What Ships in Release

**Release package includes**:
- agentscale binaries (server, runtime, CLI)
- python-3.10 base image (Linux build)
- python-3.10 base image (macOS build)
- Installation script
- Example agents
- Documentation

**Size**: ~80MB compressed

**Platforms**: Linux (amd64, arm64), macOS (arm64)

### Installation Flow

**User runs**: `curl -sSL install.agentscale.dev | bash`

**Script does**:
1. Detect platform and architecture
2. Download appropriate binary package
3. Extract to `~/.agentscale/`
4. Install CLI (pip install)
5. Verify base image present
6. Add to PATH
7. Print success and next steps

**Result**: User ready to deploy agents immediately

---

## Open Questions (To Resolve Before Implementation)

### Question 1: Config Format

When agent is deployed, update agentscale.yaml to use:
- Option A: `image: ~/.agentscale/agents/my-agent/`
- Option B: Keep `path: ./my-agent`, auto-detect image if exists

**Decision needed**: Which approach?

---

### Question 2: Deploy Behavior

If agent already deployed:
- Option A: Error (already exists)
- Option B: Rebuild (update image)
- Option C: Version it (my-agent-v1, my-agent-v2)

**Decision needed**: How to handle re-deploys?

---

### Question 3: Dependency Updates

If agent's requirements.txt changes:
- Option A: User must re-deploy
- Option B: Auto-detect and rebuild
- Option C: Hot reload (update running workers)

**Decision needed**: How to handle dependency changes?

---

### Question 4: Image Cleanup

Images accumulate over time:
- Option A: Manual (user runs cleanup command)
- Option B: Auto (remove unused after N days)
- Option C: Ask on deploy (remove old version?)

**Decision needed**: Cleanup strategy?

---

### Question 5: Build Cache Location

Download cache for Python binaries:
- Option A: `~/.agentscale/cache/` (persistent)
- Option B: `/tmp/` (ephemeral)
- Option C: Configurable

**Decision needed**: Where to cache?

---

## Key Files to Create

### New Components

**Image Builder**:
- Location: `pkg/imagebuilder/`
- Purpose: Build base images from specs
- Components: Linux builder, macOS builder, downloader

**Deploy Command**:
- Location: `cli/agentscale/commands/deploy.py`
- Purpose: Build agent images and update config
- Integration: Calls image builder, updates YAML

**Image Utilities**:
- Location: `pkg/images/`
- Purpose: Image resolution, validation, listing
- Used by: Executor, deploy command, CLI

### Modified Components

**Executor**:
- File: `pkg/proxy/executor.go`
- Change: Add --rootfs flag with image path
- Remove: macOS fallback warning

**Worker Spawner**:
- File: `pkg/scaling/worker.go`
- Change: Accept image path parameter
- Pass: To runner options

**Config Loader**:
- File: `pkg/config/server_loader.go`
- Change: Support image field in agent deploy
- Validate: Image exists before starting server

---

## Testing Strategy

### Unit Tests

**Base image builder**:
- Download and extraction
- Python installation
- Path setup
- Packaging

**Deploy command**:
- Image building
- Dependency installation
- Code copying
- Config updates

### Integration Tests

**End-to-end**:
- Fresh install → Deploy → Server → Invoke
- Multi-agent deployment
- Both platforms (Linux + macOS)
- With and without dependencies

**Isolation verification**:
- Attempt to read /etc/passwd (should fail)
- Attempt to write to /tmp on host (should fail)
- Verify python3 is from image not host
- Verify cannot see host processes

### Manual Testing

**Scenarios**:
- Complex agent directory (tools/, sub-agents/)
- Large dependencies (torch, tensorflow)
- Multiple agents
- Concurrent execution
- Server restart after deploy

---

## Documentation Updates Required

### New Documentation

**Deploy Guide**:
- How to deploy agents
- What deploy command does
- Troubleshooting deploy errors

**Image System Guide**:
- What are AgentScale images
- Base vs agent images
- Storage locations
- Cleanup procedures

### Updated Documentation

**README**:
- Installation includes base images
- Deploy flow added to quick start
- Architecture diagram updated

**DEVELOPMENT-REFERENCE**:
- Image system section
- Deploy command reference
- Updated architecture diagram

---

## Appendix: Design Alternatives Considered

### Alternative 1: Docker-Style Layers

**Approach**: Use overlay filesystem for layer sharing

**Rejected because**: Too complex for v0.1.0, Linux-specific

---

### Alternative 2: Package Caching

**Approach**: Shared package cache, symlink into images

**Rejected because**: Complex, fragile, premature optimization

---

### Alternative 3: Runtime Package Installation

**Approach**: Install packages at invoke time, cache results

**Rejected because**: Adds latency, unpredictable, cache management issues

---

### Alternative 4: Registry-First

**Approach**: Build registry, pull images on deploy

**Rejected because**: Not needed for self-hosted, adds infrastructure

---

**Chosen approach**: Complete image per agent - simplest that works.

---

## Timeline Estimate

**Phase 1**: 2 hours
**Phase 2**: 2 hours
**Phase 3**: 3 hours
**Phase 4**: 2 hours
**Phase 5**: 2 hours

**Total**: 11 hours (1.5 development days)

**With buffer**: 15 hours (2 days)

---

## Next Steps

1. Review and approve this HLD
2. Resolve open questions
3. Begin Phase 1 implementation
4. Sequential execution through Phase 5
5. Integration testing
6. Documentation
7. Release preparation

---

**This HLD is ready for implementation phase.**
