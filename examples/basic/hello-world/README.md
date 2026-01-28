# Hello World Example

The simplest possible Orpheus agent. No dependencies, no API keys, just pure Python.

## Quick Start (60 seconds)

```bash
# Deploy the agent
orpheus deploy examples/basic/hello-world

# Run it
orpheus run hello-world '{"name": "Alice"}'
```

Expected output:
```json
{
  "results": ["Hello, Alice! (message #1)"],
  "timestamp": 1706342400.123,
  "operation": "greet"
}
```

## Usage Examples

### Simple Greeting

```bash
orpheus run hello-world '{"name": "Orpheus"}'
```

### Multiple Greetings

```bash
orpheus run hello-world '{"name": "Team", "count": 3}'
```

### Calculator

```bash
orpheus run hello-world '{
  "operation": "calculate",
  "a": 42,
  "b": 58,
  "op": "add"
}'
```

Expected output:
```json
{
  "results": ["42 add 58 = 100"]
}
```

### Echo (Debug)

```bash
orpheus run hello-world '{
  "operation": "echo",
  "debug": true,
  "test": "data"
}'
```

## Test Autoscaling

Send multiple requests concurrently to see autoscaling in action:

```bash
# Send 10 concurrent requests
for i in {1..10}; do
  orpheus run hello-world "{\"name\": \"User$i\"}" &
done
wait

# Check pool status
orpheus status hello-world
```

You should see the worker count scale up from 1 to multiple workers.

## Configuration

The `agent.yaml` shows minimal configuration:

```yaml
name: hello-world
runtime: python3     # Python 3.x runtime
module: agent        # Python module name (agent.py)
entrypoint: handler  # Function to call
memory: 128          # Memory limit (MB)
timeout: 30          # Execution timeout (seconds)

scaling:
  min_workers: 1     # Always keep 1 worker warm
  max_workers: 5     # Scale up to 5 workers under load
```

## No External Dependencies

This example intentionally has:
- ❌ No `requirements.txt` (uses only Python stdlib)
- ❌ No API keys required
- ❌ No external services needed
- ❌ No database connections
- ✅ Works out of the box

## Next Steps

After hello-world works, try:
- `examples/basic/calculator-python` - OpenAI integration
- `examples/basic/conversational-memory` - Stateful agents
- `examples/basic/rag-search` - Vector search with persistent workspace

## Troubleshooting

### "Agent not found"

Make sure you deployed first:
```bash
orpheus deploy examples/basic/hello-world
```

### "Connection refused"

Start the daemon:
```bash
orpheusd --tcp-bind 0.0.0.0:7777
```

### "Queue is full"

Increase queue size in agent.yaml:
```yaml
scaling:
  queue_size: 50  # Default is 10
```

## Learn More

- [Security Model](../../../SECURITY.md)
- [Capacity Planning](../../../docs/CAPACITY_PLANNING.md)
- [Troubleshooting Guide](../../../docs/TROUBLESHOOTING.md)
