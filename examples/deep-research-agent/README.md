# Deep Research Agent

A persistent, memory-enabled research assistant that remembers everything across sessions.

## Features

- **Web Research**: Multi-step research on any topic using OpenAI's WebSearchTool
- **Persistent Memory**: Remembers past research using mem0 (survives restarts)
- **Report Generation**: Creates detailed markdown reports saved to workspace
- **Cross-Session Knowledge**: Build a growing knowledge base over time

## Quick Start

### Local Testing (Without Orpheus)

```bash
cd examples/deep-research-agent

# Install dependencies
pip install -r requirements.txt

# Set your API key
export OPENAI_API_KEY='your-key'

# Run interactive mode
python test_local.py

# Or single query
python test_local.py "What are the latest AI developments?"
```

### Deploy to Orpheus

```bash
# Deploy the agent
orpheus deploy ./examples/deep-research-agent

# Invoke it
orpheus invoke deep-research-agent '{"query": "Research quantum computing progress"}'

# With options
orpheus invoke deep-research-agent '{
  "query": "Compare fusion energy approaches",
  "depth": "deep",
  "output_format": "report"
}'
```

## Input Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `query` | string | required | The research topic or question |
| `depth` | string | "standard" | Research depth: "quick", "standard", or "deep" |
| `output_format` | string | "summary" | Output type: "summary" or "report" |
| `session_id` | string | auto | Optional session identifier |

### Depth Levels

- **quick**: 2-3 searches, brief summary (~2 min)
- **standard**: 5-7 searches, detailed summary (~5 min)
- **deep**: 10+ searches, comprehensive analysis (~10 min)

## Output Format

```json
{
  "status": "success",
  "result": {
    "topic": "Quantum computing",
    "prior_knowledge": "From previous sessions...",
    "new_findings": "Latest developments...",
    "key_facts": ["Fact 1", "Fact 2"],
    "sources": ["https://...", "https://..."],
    "confidence": "high",
    "memories_stored": 5,
    "follow_up_questions": ["What about..."]
  },
  "execution_time_seconds": 45.2
}
```

## Memory Persistence

The agent stores knowledge in `~/.orpheus/agents/deep-research-agent/memory/`:

- Facts, findings, and trends from research
- Source credibility information
- User preferences for research style

**Memory survives**:
- Agent restarts
- Daemon restarts
- System reboots

## Reports

Generated reports are saved to `~/.orpheus/agents/deep-research-agent/reports/`:

```
reports/
├── 20260107_143022_Quantum_Computing.md
├── 20260107_150115_AI_Agents_2025.md
└── 20260107_161230_Nuclear_Fusion.md
```

## Example Sessions

### Session 1: Initial Research

```bash
orpheus invoke deep-research-agent '{"query": "What is SpaceX Starship?"}'
```

Response includes web research findings, stored in memory.

### Session 2: Recall + Update

```bash
orpheus invoke deep-research-agent '{"query": "Any updates on Starship since we last checked?"}'
```

Agent recalls previous research, searches for updates, notes what's new.

### Session 3: Cross-Reference

```bash
orpheus invoke deep-research-agent '{"query": "How does Starship compare to Blue Origin New Glenn?"}'
```

Agent uses Starship knowledge from memory, researches New Glenn, compares.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Deep Research Agent                       │
├─────────────────────────────────────────────────────────────┤
│  Framework: OpenAI Agents SDK                               │
│  Model:     gpt-5-mini                                      │
│  Memory:    mem0 + ChromaDB (local)                         │
└─────────────────────────────────────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
  ┌──────────┐     ┌──────────┐     ┌──────────┐
  │ WebSearch│     │  Memory  │     │  Reports │
  │   Tool   │     │  (mem0)  │     │  (files) │
  └──────────┘     └──────────┘     └──────────┘
```

## Dependencies

- `openai>=1.88` - OpenAI API client
- `openai-agents>=0.3.2` - OpenAI Agents SDK
- `mem0ai>=0.1.0` - Memory layer
- `chromadb>=0.4.0` - Vector store for memory

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes | OpenAI API key |

## Troubleshooting

### Memory not persisting

Check that the workspace directory exists and is writable:
```bash
ls -la ~/.orpheus/agents/deep-research-agent/
```

### ChromaDB errors

Try clearing the memory database:
```bash
rm -rf ~/.orpheus/agents/deep-research-agent/memory/chroma_db
```

### Rate limiting

The agent uses `gpt-5-mini` which is cost-effective but may hit rate limits on heavy use. Consider adding delays between requests for deep research.
