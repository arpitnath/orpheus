# Scaling Configuration Guide

AgentScale automatically scales your agents based on request load using queue-depth based autoscaling. This guide explains how scaling works and how to customize it.

---

## Default Behavior

**No configuration needed!** AgentScale uses sensible defaults that work for most use cases:

```
min_workers: 1         # Starts with 1 worker
max_workers: 10        # Scales up to 10 workers under load
queue_size: 50         # Buffers up to 50 pending requests
scale_up_threshold: 3.0   # Scales up when queue > 3× workers
scale_down_threshold: 0.5 # Scales down when queue < 0.5× workers
```

**How it works:**
1. Deploy agent → 1 worker starts immediately
2. Send requests → Queue fills up
3. Autoscaler monitors: `utilization = (pending + processing) / workers`
4. When utilization > 3.0 → Spawns more workers
5. When utilization < 0.5 → Removes idle workers
6. Runs every 5 seconds

---

## Custom Scaling Configuration

For advanced use cases, add a `scaling:` section to your `agent.yaml`:

### Minimal Example (Development)

```yaml
# agent.yaml
name: my-dev-agent
runtime: python3
module: agent
entrypoint: handler

# Keep resources low for development
scaling:
  max_workers: 3
```

**Result:** Starts with 1 worker, scales up to max 3.

---

### Balanced Example (Production)

```yaml
scaling:
  min_workers: 2        # Always keep 2 workers warm
  max_workers: 20       # Scale up to 20 under load
  queue_size: 100       # Buffer 100 requests before rejecting
```

**Result:** Pre-warmed with 2 workers, handles bursts up to 20 workers.

---

### Aggressive Example (High-Traffic API)

```yaml
scaling:
  min_workers: 5        # Start with 5 workers
  max_workers: 50       # Scale aggressively to 50
  queue_size: 200       # Large buffer
  scale_up_threshold: 2.0   # Scale up faster (at 2× vs default 3×)
  scale_up_delay: 10s   # Quick scale-up
```

**Result:** High baseline capacity, responsive scaling for traffic spikes.

---

### Conservative Example (Cost-Optimized)

```yaml
scaling:
  min_workers: 0        # Scale to zero when idle
  max_workers: 5        # Limited max capacity
  scale_down_delay: 30s # Scale down quickly to save resources
```

**Result:** Minimal cost when idle, scales up on demand.

---

## Configuration Parameters

### Worker Limits

**`min_workers`** (default: 1)
- Minimum workers to keep running
- Range: 0-50 (OSS limit)
- Set to 0 to scale to zero when idle
- Set higher for pre-warmed capacity

**`max_workers`** (default: 10)
- Maximum workers allowed
- Range: 1-100 (OSS limit)
- Prevents resource exhaustion
- Higher = more concurrent capacity

---

### Scaling Thresholds

**`scale_up_threshold`** (default: 3.0)
- Scale up when `(pending + processing) / workers > threshold`
- Range: 0.1-50.0
- Lower = more aggressive scaling (scales up sooner)
- Higher = more conservative (tolerates higher queue)

**`scale_down_threshold`** (default: 0.5)
- Scale down when `(pending + processing) / workers < threshold`
- Range: 0.0-10.0
- Must be < scale_up_threshold
- Lower = keeps workers longer

---

### Scaling Delays

**`scale_up_delay`** (default: 15s)
- Minimum time between scale-up operations
- Range: 1s-10m
- Prevents rapid oscillation
- Lower = faster response to load spikes

**`scale_down_delay`** (default: 1m)
- Minimum time between scale-down operations
- Range: 1s-30m
- Prevents premature worker termination
- Higher = keeps capacity longer

**`idle_timeout`** (default: 10m)
- How long idle workers stay alive
- Applies when scaled above min_workers
- Saves resources by terminating unused workers

---

### Queue Settings

**`queue_size`** (default: 50)
- Maximum pending requests before rejecting new ones
- Range: 1-1000 (OSS limit)
- Higher = more buffering during bursts
- Lower = faster failure (503) when overloaded

---

## Use Cases

### Development Environment

**Goals:** Minimal resources, fast startup

```yaml
scaling:
  max_workers: 3
  queue_size: 10
```

---

### Production API (Balanced)

**Goals:** Handle bursts, cost-efficient

```yaml
scaling:
  min_workers: 2
  max_workers: 20
  queue_size: 100
  scale_up_delay: 15s
  scale_down_delay: 2m
```

---

### High-Traffic Service

**Goals:** Maximum throughput, low latency

```yaml
scaling:
  min_workers: 10
  max_workers: 50
  queue_size: 200
  scale_up_threshold: 2.0
  scale_up_delay: 10s
```

---

### Background Jobs (Cost-Optimized)

**Goals:** Minimize cost, tolerate latency

```yaml
scaling:
  min_workers: 0
  max_workers: 5
  queue_size: 100
  scale_down_delay: 30s
```

---

## Monitoring Scaling Behavior

### Check Current State

```bash
# Get stats for your agent
curl http://localhost:7777/v1/agents/my-agent/stats \
  -H "Authorization: Bearer $API_KEY"
```

**Response:**
```json
{
  "agent_name": "my-agent",
  "queue": {
    "pending": 5,
    "processing": 3,
    "fill_percentage": 16.0
  },
  "pool": {
    "total_workers": 4,
    "idle_workers": 1,
    "busy_workers": 3,
    "desired_size": 4
  },
  "derived": {
    "utilization_percentage": 75.0,
    "requests_per_worker": 2.0,
    "pool_efficiency": "high"
  }
}
```

### Interpret Metrics

**Utilization Percentage:**
- 0-40%: "low" efficiency - consider reducing max_workers
- 40-70%: "medium" - well balanced
- 70%+: "high" - may need more capacity

**Fill Percentage:**
- < 50%: Queue has plenty of headroom
- 50-80%: Moderate queue usage
- > 80%: Queue filling up - may need larger queue_size or more workers

---

## Troubleshooting

### Error: "min_workers exceeds OSS limit"

**Cause:** Tried to set min_workers > 50

**Fix:**
```yaml
scaling:
  min_workers: 50  # OSS max is 50
  max_workers: 100
```

For higher limits, use AgentScale Cloud.

---

### Error: "scale_up_threshold must be > scale_down_threshold"

**Cause:** Invalid threshold configuration

**Fix:**
```yaml
scaling:
  scale_up_threshold: 3.0     # Higher value
  scale_down_threshold: 0.5   # Lower value (must be less than scale_up)
```

---

### Workers not scaling up

**Check:**
1. Queue is actually filling up (`/v1/stats` shows high pending count)
2. Haven't hit max_workers limit
3. Scale_up_delay hasn't elapsed (check last_scale_time in stats)

**Debug:**
```bash
# Watch scaling in real-time
watch -n 1 'curl -s http://localhost:7777/v1/stats?agent=my-agent | jq ".pool, .queue"'
```

---

### Workers not scaling down

**Check:**
1. Queue is actually empty (pending + processing = 0)
2. Scale_down_delay has elapsed (default: 1 minute)
3. Not at min_workers already

**Note:** Workers only scale down to min_workers, not below.

---

## Advanced: Full Configuration

Complete example with all available parameters:

```yaml
name: my-agent
runtime: python3
module: agent
entrypoint: handler
env:
  - OPENAI_API_KEY=${OPENAI_API_KEY}

scaling:
  # Worker bounds
  min_workers: 2                 # Minimum workers (0-50)
  max_workers: 20                # Maximum workers (1-100)

  # Utilization targets
  target_utilization: 2.0        # Ideal tasks per worker
  scale_up_threshold: 3.0        # Scale up when > this
  scale_down_threshold: 0.5      # Scale down when < this

  # Timing
  scale_up_delay: 15s            # Wait between scale-ups (1s-10m)
  scale_down_delay: 1m           # Wait between scale-downs (1s-30m)
  idle_timeout: 10m              # Kill idle workers after this

  # Queue
  queue_size: 100                # Max pending requests (1-1000)
```

---

## Safety Limits (OSS)

To prevent resource exhaustion on self-hosted deployments:

| Parameter | Min | Max | Default |
|-----------|-----|-----|---------|
| min_workers | 0 | 50 | 1 |
| max_workers | 1 | 100 | 10 |
| queue_size | 1 | 1000 | 50 |
| scale_up_delay | 1s | 10m | 15s |
| scale_down_delay | 1s | 30m | 1m |
| scale_up_threshold | 0.1 | 50.0 | 3.0 |
| scale_down_threshold | 0.0 | 10.0 | 0.5 |

**Note:** Cloud deployments may have higher limits for enterprise customers.

---

## FAQ

**Q: Do I need to configure scaling?**
A: No! The defaults work great for most use cases. Only customize if you have specific requirements.

**Q: What's the difference between target_utilization and thresholds?**
A: `target_utilization` is informational (ideal state). `scale_up_threshold` and `scale_down_threshold` trigger actual scaling actions.

**Q: Can I change scaling config without redeploying?**
A: Not in v0.1.0. You must undeploy and redeploy the agent. Runtime updates coming in future versions.

**Q: Why did my invalid config get ignored?**
A: Validation failures fall back to defaults (logged). Check daemon logs for the error message.

**Q: How do I disable autoscaling?**
A: Set `min_workers` and `max_workers` to the same value (e.g., both = 5). Workers won't scale.

---

## Learn More

- Check real-time metrics: `GET /v1/stats`
- Monitor daemon logs for scaling events: `[autoscaler]`, `[pool]`, `[queue]`
- See Architecture Docs: `architecture/05-V0.1.0-FEATURE-SCOPE.md`
