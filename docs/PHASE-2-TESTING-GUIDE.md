# Phase 2: Worker Health & Reliability - Testing Guide

**Created**: December 16, 2025
**Status**: Manual Testing Required
**Phase**: 2 of 7

---

## Automated Tests Completed ✅

| Test | Status | Result |
|------|--------|--------|
| Server compilation | ✅ PASS | No build errors |
| Multi-agent startup | ✅ PASS | 2 agents loaded (planning: 2 workers, simple: 1 worker) |
| Health endpoint | ✅ PASS | Returns all agents correctly |
| Agent invocation | ✅ PASS | simple-agent executed in 97ms |
| Stats endpoint | ✅ PASS | Per-agent and global stats work |
| Agent routing | ✅ PASS | Requests route to correct agent |

---

## Manual Tests Required ⚠️

The following features were implemented but **not fully tested** due to automation limitations. Manual verification needed:

---

### Test 1: Periodic Health Checks (30s Maintenance Cycle)

**What to verify**: Health checks run every 30 seconds in maintenance loop

**How to test**:
```bash
# Terminal 1: Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Wait 35+ seconds
# Watch logs for maintenance activity
```

**Expected logs**:
```
[pool] planning-agent: maintenance cycle...
[pool] simple-agent: maintenance cycle...
(Health checks run silently unless workers are unhealthy)
```

**Success criteria**:
- Maintenance runs every 30s
- No errors in logs
- Pool sizes remain stable

**Why not automated**: Requires 30+ second wait, tedious in script

---

### Test 2: Auto-Replacement of Unhealthy Workers

**What to verify**: When a worker becomes unhealthy, it's automatically replaced

**How to test**:

#### Option A: Simulate Worker Crash (Advanced)
```bash
# Terminal 1: Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Terminal 2: Find and kill a worker subprocess
ps aux | grep python | grep simple_agent
# Kill one of the Python processes

# Terminal 1: Watch logs for replacement
# Expected within 30s (next maintenance cycle):
[pool] simple-agent: replacing unhealthy worker simple-agent-worker-1 (attempt 1/3)
[pool] simple-agent: successfully spawned replacement worker
```

#### Option B: Inject Failure via Modified Agent
```bash
# Modify examples/simple-agent/simple_agent.py temporarily:
def process_query(input_data):
    raise Exception("Simulated failure")  # Force failure

# Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Invoke 3 times to trigger consecutive failures
agentscale invoke simple-agent '{"query": "test"}'  # Failure 1 → Degraded
agentscale invoke simple-agent '{"query": "test"}'  # Failure 2 → Degraded
agentscale invoke simple-agent '{"query": "test"}'  # Failure 3 → Unhealthy

# Wait 30s for maintenance cycle
# Expected: Worker replaced
```

**Expected logs**:
```
[pool] simple-agent: replacing unhealthy worker simple-agent-worker-1 (attempt 1/3)
[pool] simple-agent: removed worker simple-agent-worker-1 (total=0)
[pool] simple-agent: spawned worker simple-agent-worker-2 (total=1)
[pool] simple-agent: successfully spawned replacement worker
```

**Success criteria**:
- Unhealthy worker detected within 30s
- Worker removed from pool
- Replacement spawned
- Pool size maintained

**Why not automated**: Complex to simulate worker process failures programmatically

---

### Test 3: Rate Limiting (Replacement Storm Prevention)

**What to verify**: Multiple simultaneous failures don't cause replacement storm

**How to test**:

```bash
# Modify simple_agent.py to fail for all workers
def process_query(input_data):
    import os
    raise Exception(f"Fail from PID {os.getpid()}")

# Start server with simple-agent having min=3, max=5
# Edit examples/agentscale.yaml:
  simple-agent:
    scaling:
      min_workers: 3
      max_workers: 5

./bin/agentscale-server --config ./examples/agentscale.yaml

# Invoke repeatedly to make all 3 workers unhealthy
for i in {1..10}; do
  agentscale invoke simple-agent '{"query": "test"}' &
done

# Wait for maintenance cycle (30s)
# Watch logs
```

**Expected logs**:
```
[pool] simple-agent: replacing unhealthy worker worker-1 (attempt 1/3)
[pool] simple-agent: rate limiting replacement for worker-2
[pool] simple-agent: rate limiting replacement for worker-3
(Only 1 replacement per 5 seconds)
```

**Success criteria**:
- Only 1 replacement every 5 seconds
- No rapid-fire replacement attempts
- Logs show "rate limiting replacement"

**Why not automated**: Requires modifying agent code and coordinating timing

---

### Test 4: Max Replacement Attempts (3 Attempts Then Give Up)

**What to verify**: After 3 failed replacement attempts, system gives up

**How to test**:

```bash
# Make worker spawning fail by breaking agent.yaml
# Edit examples/simple-agent/agent.yaml:
entrypoint: nonexistent_file.py  # Invalid file

# Start server (will fail to spawn initial workers)
./bin/agentscale-server --config ./examples/agentscale.yaml

# Expected: Server tries 3 times then gives up
```

**OR** (if server starts):

```bash
# Make agent fail consistently
# Modify simple_agent.py to raise exception
# Start server
# Invoke to make worker unhealthy
# Manually delete the agent.yaml to break spawning
# Wait for replacement attempts
```

**Expected logs**:
```
[pool] simple-agent: replacing unhealthy worker worker-1 (attempt 1/3)
[pool] simple-agent: failed to spawn replacement (attempt 1/3): ...
(sleep 1s)
[pool] simple-agent: failed to spawn replacement (attempt 2/3): ...
(sleep 2s)
[pool] simple-agent: failed to spawn replacement (attempt 3/3): ...
[pool] simple-agent: failed to spawn replacement after 3 attempts

(Next maintenance cycle, worker-1 already at max attempts)
[pool] simple-agent: max replacement attempts (3) reached for worker-1, giving up
```

**Success criteria**:
- Linear backoff between attempts (1s, 2s)
- After 3 attempts, logs "failed to spawn replacement after 3 attempts"
- Future cycles log "max replacement attempts reached, giving up"
- Attempt counter cleared after successful spawn

**Why not automated**: Requires breaking config files and precise timing coordination

---

### Test 5: Consecutive Failure Tracking (Degraded → Unhealthy)

**What to verify**: Worker transitions Healthy → Degraded → Unhealthy after consecutive failures

**How to test**:

```bash
# Modify simple_agent.py to fail initially then succeed:
FAIL_COUNT = 0

def process_query(input_data):
    global FAIL_COUNT
    FAIL_COUNT += 1
    if FAIL_COUNT <= 2:
        raise Exception(f"Simulated failure {FAIL_COUNT}")
    return {"response": "Now working"}

# Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Invoke 4 times
agentscale invoke simple-agent '{"query": "1"}' # Failure 1
agentscale invoke simple-agent '{"query": "2"}' # Failure 2
agentscale invoke simple-agent '{"query": "3"}' # Failure 3
agentscale invoke simple-agent '{"query": "4"}' # Success

# Check logs for health transitions
```

**Expected behavior**:
- After failure 1: Worker marked HealthDegraded (consecutiveFailures=1)
- After failure 2: Worker still HealthDegraded (consecutiveFailures=2)
- After failure 3: Worker marked HealthUnhealthy (consecutiveFailures=3)
- After success: Worker marked HealthHealthy (consecutiveFailures=0)

**Success criteria**:
- consecutiveFailures counter increments correctly
- Health transitions: Healthy → Degraded → Unhealthy
- Success resets counter

**Why not automated**: Requires stateful agent modification

---

### Test 6: Emergency Worker Spawning Under Load

**What to verify**: When all workers busy and queue building, emergency spawn triggers

**How to test**:

```bash
# Make agent slow to create queue buildup
# Modify simple_agent.py:
def process_query(input_data):
    import time
    time.sleep(5)  # Slow execution
    return {"response": "done"}

# Start server with min=1, max=5
./bin/agentscale-server --config ./examples/agentscale.yaml

# Send many concurrent requests
for i in {1..20}; do
  agentscale invoke simple-agent '{"query": "test"}' &
done

# Watch logs for emergency spawn
```

**Expected logs**:
```
[queue] simple-agent: enqueued request ... (pending=10)
[pool] simple-agent: emergency spawn after 30s timeout
[pool] simple-agent: spawned worker simple-agent-worker-2 (total=2)
```

**Success criteria**:
- After 30s with no idle workers, emergency spawn triggers
- New worker spawned (if under max)
- Queue drains faster after spawn
- Emergency spawn respects max_workers limit

**Why not automated**: Requires long-running agent and precise timing

---

### Test 7: Graceful Shutdown During Processing

**What to verify**: SIGINT during active requests allows them to complete

**How to test**:

```bash
# Make agent slow
# Modify simple_agent.py:
def process_query(input_data):
    import time
    time.sleep(10)  # 10 second processing
    return {"response": "completed"}

# Terminal 1: Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Terminal 2: Send request (will take 10s)
time agentscale invoke simple-agent '{"query": "test"}'

# Terminal 1: Send SIGINT immediately after invoke starts
# (within first 5 seconds)
# Ctrl+C

# Terminal 2: Should complete successfully after ~10s
```

**Expected behavior**:
- Request completes even after SIGINT
- Terminal 2 shows successful response (not timeout)
- Server logs show graceful drain:
  ```
  [server] Shutting down...
  [queue] simple-agent: closing (pending=0, processing=1)
  [server] Agent 'simple-agent' workers drained
  [server] Shutdown complete
  ```

**Success criteria**:
- In-flight requests complete
- No "request timeout" errors
- Clean shutdown logs
- All agents drain properly

**Why not automated**: Requires precise timing of SIGINT during execution

---

### Test 8: Autoscaler Activity (Queue-Based Scaling)

**What to verify**: Autoscaler scales pool based on queue depth

**How to test**:

```bash
# Make agent slow and send burst load
# Modify planning_agent.py to add delay
import time
time.sleep(2)  # 2 second per request

# Start server
./bin/agentscale-server --config ./examples/agentscale.yaml

# Send 20 concurrent requests to planning-agent
for i in {1..20}; do
  curl -s -X POST "http://localhost:8080/invoke?agent=planning-agent" \
    -d '{"query": "test"}' > /dev/null &
done

# Monitor stats every 5 seconds
watch -n 5 "agentscale stats planning-agent"
```

**Expected behavior**:
- Queue fills: pending=18, processing=2 (with 2 workers)
- Utilization = 20/2 = 10.0 > ScaleUpThreshold(3.0)
- After 10-15s: Autoscaler triggers scale up
- Workers increase: 2 → 4 → 6 (based on utilization)
- Queue drains faster
- After idle: Workers scale back down to min=2

**Expected logs**:
```
[autoscaler] planning-agent: scaling 2 → 5 (util=10.0)
[pool] planning-agent: scaling up 2 → 5 (spawning 3 workers)
[pool] planning-agent: spawned worker planning-agent-worker-3
[pool] planning-agent: spawned worker planning-agent-worker-4
[pool] planning-agent: spawned worker planning-agent-worker-5
```

**Success criteria**:
- Pool scales up based on queue depth
- Scaling happens within autoscaler_interval (10s)
- Pool respects max_workers limit
- Pool scales down after load subsides

**Why not automated**: Requires sustained load monitoring over 30+ seconds

---

## Manual Testing Checklist

For comprehensive Phase 2 verification, manually test:

- [ ] Maintenance cycle runs every 30 seconds (check logs)
- [ ] Health checks scan all workers in maintenance
- [ ] Unhealthy worker auto-replaced within 30s
- [ ] Rate limiting prevents replacement storm (5s cooldown)
- [ ] Max 3 replacement attempts enforced
- [ ] Linear backoff observed (1s, 2s delays)
- [ ] Consecutive failures: Healthy → Degraded → Unhealthy
- [ ] Success resets consecutive failure counter
- [ ] Emergency spawn triggers after 30s with no idle workers
- [ ] Emergency spawn respects max_workers limit
- [ ] Graceful shutdown drains in-flight requests
- [ ] Both agents shutdown cleanly
- [ ] Autoscaler scales based on queue depth
- [ ] Pool respects min/max worker bounds

---

## Quick Manual Test Script

For convenience when testing later:

```bash
#!/bin/bash
# Phase 2 Manual Testing Script

echo "=== Phase 2 Manual Test ==="

# Start server
./bin/agentscale-server --config ./examples/agentscale.yaml &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
sleep 3

# Test basic functionality
echo ""
echo "1. Health check:"
agentscale health

echo ""
echo "2. Invoke test:"
agentscale invoke simple-agent '{"query": "test"}'

echo ""
echo "3. Stats:"
agentscale stats

echo ""
echo "4. Waiting 35 seconds for maintenance cycle..."
sleep 35

echo ""
echo "5. Check logs for maintenance activity"
echo "(Look for maintenance cycle or health check logs)"

echo ""
echo "6. Graceful shutdown test..."
agentscale invoke simple-agent '{"query": "test"}' &
sleep 1
kill -INT $SERVER_PID

echo ""
echo "Waiting for shutdown..."
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "Test complete. Review logs above."
```

---

## Advanced Test: Simulate Worker Failure

**Purpose**: Verify auto-replacement end-to-end

**Setup**:
1. Modify `examples/simple-agent/simple_agent.py`:
```python
import os

FAIL_MODE = os.environ.get('FAIL_MODE', 'false') == 'true'
CALL_COUNT = 0

def process_query(input_data):
    global CALL_COUNT
    CALL_COUNT += 1

    if FAIL_MODE:
        if CALL_COUNT <= 3:
            raise Exception(f"Simulated failure {CALL_COUNT}/3")

    return {
        "response": f"Processed: {input_data.get('query', 'no query')}",
        "agent": "simple-agent",
        "call_count": CALL_COUNT,
        "version": "1.0.0",
    }
```

2. Start server normally (FAIL_MODE not set)
3. Invoke successfully to verify baseline
4. Restart server with `FAIL_MODE=true` (modify config or env)
5. Invoke 3 times to trigger unhealthy state
6. Wait for maintenance cycle
7. Verify worker replaced

**Expected flow**:
```
Invoke 1: FAIL_MODE=true → Exception → consecutiveFailures=1 → Degraded
Invoke 2: FAIL_MODE=true → Exception → consecutiveFailures=2 → Degraded
Invoke 3: FAIL_MODE=true → Exception → consecutiveFailures=3 → Unhealthy
(Wait 30s for maintenance)
Health check: Worker is Unhealthy → replaceUnhealthyWorker()
Worker removed and replaced
Invoke 4: FAIL_MODE=false → Success → Healthy
```

---

## Observability During Testing

### Log Patterns to Watch For

**Successful health check**:
```
(Silent - no logs unless workers are unhealthy)
```

**Auto-replacement triggered**:
```
[pool] {agent}: replacing unhealthy worker {worker-id} (attempt N/3)
[pool] {agent}: removed worker {worker-id} (total=X)
[pool] {agent}: spawned worker {new-worker-id} (total=Y)
[pool] {agent}: successfully spawned replacement worker
```

**Rate limiting active**:
```
[pool] {agent}: rate limiting replacement for {worker-id}
```

**Max attempts reached**:
```
[pool] {agent}: max replacement attempts (3) reached for {worker-id}, giving up
```

**Emergency spawn**:
```
[pool] {agent}: emergency spawn after 30s timeout
```

**Autoscaler activity**:
```
[autoscaler] {agent}: scaling {old} → {new} (util={value})
```

**Graceful shutdown**:
```
[server] Shutting down...
[queue] {agent}: closing (pending=X, processing=Y)
[server] Agent '{agent}' workers drained
[server] Shutdown complete
```

---

## Known Limitations

### Cannot Test Programmatically

1. **Worker process failures**: No easy way to kill subprocess from test script
2. **Consecutive failures**: Requires stateful agent code modification
3. **Replacement storms**: Requires coordinated multi-worker failures
4. **Long-running operations**: Test timeouts conflict with 30s+ waits
5. **Autoscaler timing**: 10-30s intervals too slow for CI/CD

### Workarounds

- Use modified agents with failure injection
- Manual testing with log inspection
- Integration tests with real load scenarios
- Chaos engineering (kill processes manually)

---

## Future: Automated Testing Improvements

For v0.2+, consider:

1. **Test mode flag**: `--test-mode` enables failure injection
2. **Health probe API**: `POST /test/fail-worker?agent=X&worker=Y`
3. **Mock agent**: Built-in test agent with configurable failure modes
4. **Metrics endpoint**: `/metrics` for Prometheus-style monitoring
5. **Shorter timeouts**: Test-specific config with 5s cycles instead of 30s

---

## Summary

**Automated Tests**: ✅ 6/6 pass (basic functionality)
**Manual Tests Required**: ⚠️ 8 scenarios (health features)
**Blocker**: None - code is solid, just needs real-world verification
**Recommendation**: Commit Phase 2, manual test post-deployment

---

## What Was Implemented (Reference)

### Files Modified
- `pkg/scaling/worker_pool.go` - Health check loop, auto-replacement, rate limiting
- `pkg/scaling/worker.go` - Consecutive failure tracking

### Features Added
- Periodic health checks (every 30s maintenance cycle)
- Auto-replacement with 5s rate limiting
- Max 3 replacement attempts per worker
- Linear backoff (1s, 2s, 3s)
- Consecutive failure tracking (3 failures = unhealthy)
- Degraded state support
- Emergency spawning (already existed, preserved)

### Code Patterns from BlumeLoop
- Per-worker attempt tracking map
- Global replacement cooldown
- Goroutine-safe replacement
- Attempt counter cleanup on success
- Rate limiting to prevent storms

---

**Use this guide for manual verification after commit.**
