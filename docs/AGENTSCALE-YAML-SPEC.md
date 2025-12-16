# AgentScale YAML Configuration Specification

**Version**: v0.1.0
**Last Updated**: December 16, 2025

---

## Overview

The `agentscale.yaml` file is the server deployment configuration that defines:
- Server settings (port, isolation defaults)
- Agent deployments (path, scaling policies)
- Per-agent customization

This is distinct from `agent.yaml` (agent code configuration).

---

## File Structure

```yaml
# Server configuration
server:
  port: <int>
  autoscaler_interval: <duration>
  isolation:
    enabled: <bool>
    type: <string>
    defaults:
      memory_limit: <string>
      timeout: <duration>

# Agent deployments
agents:
  <agent-id>:
    path: <string>
    scaling:
      min_workers: <int>
      max_workers: <int>
      target_utilization: <float>
      scale_up_threshold: <float>
      scale_down_threshold: <float>
      scale_up_delay: <duration>
      scale_down_delay: <duration>
      queue_size: <int>
    isolation:  # optional override
      memory_limit: <string>
      timeout: <duration>
```

---

## Field Reference

### `server` (required)

Top-level server configuration.

#### `server.port` (required)
- **Type**: `int`
- **Description**: HTTP server port
- **Example**: `8080`
- **Valid range**: 1024-65535

#### `server.autoscaler_interval` (optional)
- **Type**: `duration`
- **Description**: How often autoscaler checks and scales
- **Default**: `10s`
- **Example**: `5s`, `15s`, `1m`
- **Recommended**:
  - Fast scaling: `5s-10s`
  - Conservative: `15s-30s`

#### `server.isolation` (optional)

Isolation settings applied to all agents (unless overridden).

##### `server.isolation.enabled` (optional)
- **Type**: `bool`
- **Description**: Enable isolation (namespaces/VM)
- **Default**: `true`
- **Note**: Set `false` for development/debugging

##### `server.isolation.type` (optional)
- **Type**: `string`
- **Description**: Isolation type
- **Values**: `auto`, `namespace`, `vm`, `none`
- **Default**: `auto` (auto-detect based on OS)
- **Note**:
  - Linux: `namespace` (pid, net, mount)
  - macOS: `vm` (Virtualization.framework)
  - `none` disables isolation

##### `server.isolation.defaults` (optional)

Default resource limits for all agents.

###### `server.isolation.defaults.memory_limit` (optional)
- **Type**: `string`
- **Description**: Memory limit per worker
- **Format**: `<number><unit>` where unit is `mb` or `gb`
- **Default**: `512mb`
- **Examples**: `256mb`, `1gb`, `2gb`

###### `server.isolation.defaults.timeout` (optional)
- **Type**: `duration`
- **Description**: Execution timeout per request
- **Default**: `300s` (5 minutes)
- **Examples**: `60s`, `5m`, `10m`
- **Note**: Long-running agents may need higher values

---

### `agents` (required)

Map of agent deployments, keyed by agent ID.

#### Agent ID
- **Format**: Alphanumeric with hyphens, kebab-case
- **Examples**: `planning-agent`, `simple-agent`, `analytics-v2`
- **Requirements**:
  - Must be unique
  - Used in API: `/invoke?agent=<agent-id>`
  - Lowercase recommended

---

### `agents.<agent-id>` (required)

Configuration for a single agent deployment.

#### `agents.<agent-id>.path` (required)
- **Type**: `string`
- **Description**: Path to agent directory (contains `agent.yaml`)
- **Examples**:
  - `./examples/planning-agent`
  - `/opt/agents/analytics`
  - `../my-agents/simple-agent`
- **Note**: Can be absolute or relative to config file location

#### `agents.<agent-id>.scaling` (required)

Scaling policy for this agent.

##### `agents.<agent-id>.scaling.min_workers` (required)
- **Type**: `int`
- **Description**: Minimum workers (always running)
- **Minimum**: `1`
- **Recommended**:
  - Dev/testing: `1`
  - Production (cold start sensitive): `2-3`

##### `agents.<agent-id>.scaling.max_workers` (required)
- **Type**: `int`
- **Description**: Maximum workers (scale up limit)
- **Minimum**: Must be >= `min_workers`
- **Recommended**:
  - Light load: `3-5`
  - Medium load: `10-20`
  - High load: `50-100`
- **Note**: System resources permitting

##### `agents.<agent-id>.scaling.target_utilization` (required)
- **Type**: `float`
- **Description**: Target tasks per worker (ideal state)
- **Recommended**: `1.5-3.0`
- **Interpretation**:
  - `1.0`: One task per worker
  - `2.0`: Two tasks per worker (moderate queue)
  - `3.0`: Three tasks per worker (more aggressive queueing)
- **Note**: Higher = more tolerant of queueing

##### `agents.<agent-id>.scaling.scale_up_threshold` (required)
- **Type**: `float`
- **Description**: Utilization threshold to trigger scale up
- **Must be**: Greater than `target_utilization`
- **Recommended**: `1.2x - 1.5x` of `target_utilization`
- **Examples**:
  - If `target_utilization: 2.0`, use `3.0-3.5`
  - If `target_utilization: 1.5`, use `2.5-3.0`

##### `agents.<agent-id>.scaling.scale_down_threshold` (required)
- **Type**: `float`
- **Description**: Utilization threshold to trigger scale down
- **Must be**: Less than `target_utilization`
- **Recommended**: `0.3-0.5`
- **Note**: Low value = aggressive scale down (save resources)

##### `agents.<agent-id>.scaling.scale_up_delay` (required)
- **Type**: `duration`
- **Description**: Cooldown period after scaling up
- **Recommended**:
  - Fast response: `10s-15s`
  - Conservative: `30s-60s`
- **Purpose**: Prevent oscillation (scaling up/down repeatedly)

##### `agents.<agent-id>.scaling.scale_down_delay` (required)
- **Type**: `duration`
- **Description**: Cooldown period after scaling down
- **Recommended**: `1m-5m` (longer than scale up)
- **Purpose**:
  - Keep workers warm longer
  - Avoid cold start costs

##### `agents.<agent-id>.scaling.queue_size` (required)
- **Type**: `int`
- **Description**: Maximum pending requests in queue
- **Recommended**:
  - Light: `50-100`
  - Medium: `200-500`
  - High: `1000+`
- **Behavior**: When full, `/invoke` returns 503 (Service Unavailable)

---

#### `agents.<agent-id>.isolation` (optional)

Override server isolation defaults for this agent.

##### `agents.<agent-id>.isolation.memory_limit` (optional)
- **Type**: `string`
- **Description**: Override memory limit
- **Examples**: `1gb`, `2gb`
- **Use case**: Agent needs more memory than default

##### `agents.<agent-id>.isolation.timeout` (optional)
- **Type**: `duration`
- **Description**: Override execution timeout
- **Examples**: `10m`, `30m`
- **Use case**: Long-running agent (data processing, etc.)

---

## Complete Example

```yaml
# agentscale.yaml

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
  # Fast-scaling agent for real-time queries
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
      memory_limit: 1gb    # Override: needs more memory
      timeout: 600s        # Override: longer timeout

  # Conservative scaling for simple tasks
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
    # No isolation override - use server defaults

  # Aggressive scaling for high-throughput analytics
  analytics-agent:
    path: ./examples/analytics-agent
    scaling:
      min_workers: 3        # Always warm
      max_workers: 20
      target_utilization: 1.5
      scale_up_threshold: 2.5
      scale_down_threshold: 0.5
      scale_up_delay: 10s
      scale_down_delay: 5m  # Keep workers longer
      queue_size: 500
    isolation:
      memory_limit: 2gb    # Data-intensive
      timeout: 1800s       # 30 min for large datasets
```

---

## Scaling Policy Guidelines

### Conservative (Save Resources)
```yaml
scaling:
  min_workers: 1
  max_workers: 3
  target_utilization: 3.0     # Tolerate more queueing
  scale_up_threshold: 4.5     # Scale up only when really needed
  scale_down_threshold: 0.5
  scale_up_delay: 30s
  scale_down_delay: 2m
  queue_size: 50
```

**Use for**:
- Low-traffic agents
- Development/testing
- Cost-sensitive deployments

---

### Balanced (General Purpose)
```yaml
scaling:
  min_workers: 2
  max_workers: 10
  target_utilization: 2.0
  scale_up_threshold: 3.0
  scale_down_threshold: 0.5
  scale_up_delay: 15s
  scale_down_delay: 1m
  queue_size: 200
```

**Use for**:
- Production agents
- Moderate traffic
- General workloads

---

### Aggressive (Low Latency)
```yaml
scaling:
  min_workers: 3              # Always warm
  max_workers: 20
  target_utilization: 1.5     # Scale proactively
  scale_up_threshold: 2.5     # Quick scale up
  scale_down_threshold: 0.5
  scale_up_delay: 10s         # Fast response
  scale_down_delay: 5m        # Keep workers longer
  queue_size: 500
```

**Use for**:
- User-facing agents
- Real-time applications
- High SLA requirements

---

## Duration Format

All duration fields accept:
- Seconds: `30s`, `60s`
- Minutes: `1m`, `5m`, `30m`
- Hours: `1h`, `2h`
- Combined: `1m30s`, `2h30m`

**Examples**:
- `10s` = 10 seconds
- `1m` = 1 minute = 60 seconds
- `5m30s` = 5 minutes 30 seconds = 330 seconds

---

## Memory Format

Memory limit fields accept:
- Megabytes: `256mb`, `512mb`, `1024mb`
- Gigabytes: `1gb`, `2gb`, `4gb`

**Examples**:
- `512mb` = 512 megabytes
- `1gb` = 1 gigabyte = 1024 megabytes
- `2gb` = 2 gigabytes = 2048 megabytes

---

## Validation Rules

### Server Level
1. `server.port` must be 1024-65535
2. `server.autoscaler_interval` must be >= 1s
3. If `isolation.enabled = false`, `isolation.type` ignored

### Agent Level
1. `agent-id` must be unique across all agents
2. `agent-id` must match `^[a-z0-9-]+$` (alphanumeric + hyphen)
3. `path` must point to directory containing `agent.yaml`
4. `max_workers` must be >= `min_workers`
5. `scale_up_threshold` must be > `target_utilization`
6. `scale_down_threshold` must be < `target_utilization`
7. `queue_size` must be > 0

### Scaling Math
```
Utilization = (pending + processing) / current_workers

Scale up when:
  utilization > scale_up_threshold
  AND time_since_last_scale >= scale_up_delay

Scale down when:
  utilization < scale_down_threshold
  AND time_since_last_scale >= scale_down_delay

Target size:
  ceil((pending + processing) / target_utilization)
  Bounded by [min_workers, max_workers]
```

---

## Common Patterns

### Development Setup
```yaml
server:
  port: 8080
  isolation:
    enabled: false    # No isolation for debugging

agents:
  my-agent:
    path: ./my-agent
    scaling:
      min_workers: 1
      max_workers: 2
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      scale_up_delay: 30s
      scale_down_delay: 2m
      queue_size: 10    # Small for testing
```

### Production Setup
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
  # Multiple agents with varying policies
  agent-a:
    path: ./agents/agent-a
    scaling:
      min_workers: 2
      max_workers: 15
      # ... balanced settings

  agent-b:
    path: ./agents/agent-b
    scaling:
      min_workers: 3
      max_workers: 25
      # ... aggressive settings
```

---

## Migration from v0.0.x

### Before (Single-Agent)
```bash
./bin/agentscale-server --agent ./planning-agent --port 8080 --tier pro
```

### After (Multi-Agent)
```yaml
# agentscale.yaml
server:
  port: 8080

agents:
  planning-agent:
    path: ./planning-agent
    scaling:
      min_workers: 1
      max_workers: 10
      target_utilization: 2.0
      scale_up_threshold: 3.0
      scale_down_threshold: 0.5
      scale_up_delay: 15s
      scale_down_delay: 1m
      queue_size: 200
```

```bash
./bin/agentscale-server --config agentscale.yaml
```

**Notes**:
- `--tier` flag removed (define exact values in config)
- Can now deploy multiple agents in same config
- More fine-grained control per agent

---

## Best Practices

### 1. Start Conservative, Scale Up
Begin with low `max_workers` and increase based on actual load.

### 2. Match Timeouts to Workload
- Quick tasks: `30s-60s`
- LLM calls: `5m-10m`
- Data processing: `15m-30m`

### 3. Monitor and Adjust
Use `agentscale scaling-history` to see if scaling policies work well.

### 4. Use Descriptive Agent IDs
- Good: `planning-agent`, `analytics-v2`, `content-classifier`
- Bad: `agent1`, `a`, `test`

### 5. Isolation Overrides Sparingly
Only override when agent truly needs different limits.

### 6. Keep Scale-Down Delay > Scale-Up Delay
Prevents rapid scaling oscillation.

---

## Troubleshooting

### Issue: Agent not scaling up
**Check**:
- Is utilization > `scale_up_threshold`?
- Has `scale_up_delay` elapsed since last scale?
- Is pool at `max_workers` already?

### Issue: Queue filling up (503 errors)
**Solutions**:
- Increase `max_workers`
- Increase `queue_size`
- Lower `scale_up_threshold` (scale sooner)
- Decrease `scale_up_delay` (scale faster)

### Issue: Workers scaling down too quickly
**Solutions**:
- Increase `scale_down_delay`
- Lower `scale_down_threshold`
- Increase `min_workers` (keep warm)

### Issue: High memory usage
**Solutions**:
- Decrease `max_workers`
- Lower `memory_limit` per worker
- Check for agent memory leaks

---

## Future Additions (v0.2+)

Planned features for future versions:
- Preset policies: `conservative`, `balanced`, `aggressive`
- Environment-specific overrides: `dev`, `staging`, `prod`
- Global limits: `max_total_workers` across all agents
- Cost optimization: `cost_per_worker`, optimize for cost
- Predictive scaling: ML-based anticipation

---

**This specification is for v0.1.0 and subject to change.**
