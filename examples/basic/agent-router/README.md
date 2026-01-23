# Agent Router (MCP Client)

This agent demonstrates **Agent Orchestration** using the **Model Context Protocol (MCP)**. It acts as a router that receives natural language queries and delegates the actual work to other specialized agents.

## How it Works

1.  **The Brain:** Uses Anthropic's Claude (via SDK) to understand user intent.
2.  **The Tools:** It connects to the `calculator-python` agent via its auto-generated MCP endpoint.
3.  **The Flow:**
    *   User asks: "Calculate 25 * 4"
    *   Router Agent sees it has a `calculator` tool.
    *   Router Agent calls `calculator-python` via MCP (`tools/call`).
    *   Calculator returns `100`.
    *   Router Agent responds: "The result is 100."

## Configuration

Requires an `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` and the URL of the target agent.

```yaml
env:
  - ANTHROPIC_API_KEY=sk-...
  - MCP_URL=https://your-ngrok-url/mcp/agents/calculator-python
```

<Note>
For local development, you must use **ngrok** to expose the target agent's MCP endpoint so the LLM provider can reach it.
</Note>
