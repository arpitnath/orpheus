# Local Research Agent

**Powered by**: Mistral 7B via Ollama
**Purpose**: Demonstrate local LLM with tool calling (no cloud, no API keys)
**Status**: ✅ Working

---

## What This Proves

- ✅ Local model (Mistral 7B) running on Mac
- ✅ Tool calling works (get_current_time, calculate)
- ✅ No internet required after model download
- ✅ No API keys needed
- ✅ Fully self-contained agent

---

## Prerequisites

```bash
# Install Ollama
brew install ollama

# Start Ollama server
brew services start ollama

# Pull Mistral model (4.4GB download)
ollama pull mistral

# Install Python SDK
pip install ollama
```

---

## Run the Agent

```bash
cd examples/local-research-agent
python3 agent.py
```

---

## Test Results

**Query 1**: "What time is it?"
- ✅ Called `get_current_time()` tool
- ✅ Returned: "It is 14:54 on January 9, 2026"

**Query 2**: "Calculate 42 * 137"
- ✅ Called `calculate()` tool
- ✅ Returned: "Result: 5754"

**Query 3**: "What is Python?"
- ⚠️ Described how to use search_docs instead of calling it
- Note: Tool calling reliability varies by query complexity

---

## Available Tools

1. **search_docs(query)** - Search documentation
2. **get_current_time()** - Get current timestamp
3. **calculate(expression)** - Evaluate math expressions

---

## Performance

**Model**: Mistral 7.2B (Q4 quantization)
**Memory**: ~6-8GB when loaded
**Response time**: ~1-3 seconds per query
**Keep-alive**: Model stays in RAM for 5 minutes after last request

---

## Next Steps

**For Orpheus integration:**
1. Abstract model backend (OpenAI OR Ollama)
2. Add warm agent pool (keep Python process + model loaded)
3. Enable session affinity (same user → same warm agent)
4. Deploy to remote (EC2) with same agent.yaml

---

**Status**: Prototype validated ✅ - ready for Orpheus integration
