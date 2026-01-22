# Orpheus Example Agents

Production-ready example agents demonstrating Orpheus capabilities for AI agent deployment.

---

## Directory Structure

```
examples/
├── basic/               # Getting started examples
├── stateful/            # Session affinity + workspace persistence
├── long-running/        # Multi-step workflows
└── memory-intensive/    # Resource management
```

---

## Basic Examples

**Purpose:** Learn Orpheus fundamentals with simple agents

| Agent | Language | Description | Use Case |
|-------|----------|-------------|----------|
| `calculator-python` | Python | OpenAI API integration | API-based reasoning |
| `calculator-nodejs` | Node.js | Same agent in JavaScript | Language variety |
| `gmail-automation` | Node.js | Email labeling workflow | Real-world automation |

**Start here** if you're new to Orpheus.

---

## Stateful Examples

**Purpose:** Demonstrate workspace persistence and session affinity

### `conversational-memory`
**Framework:** Direct Ollama API
**Model:** Mistral (local)
**Storage:** SQLite in `/workspace`

**What it demonstrates:**
- Session affinity (same user → same worker)
- Conversation history persists across container restarts
- Emotional state tracking over time
- State persistence across restarts

**Example workflow:**
```bash
# Start conversation
orpheus invoke conversational-memory '{"message": "I feel anxious"}' \
  -H "X-Session-ID: user-123"

# Continue
orpheus invoke conversational-memory '{"message": "Things are better"}' \
  -H "X-Session-ID: user-123"

# Kill container
runc kill <container-id>

# Resume - agent STILL remembers everything
orpheus invoke conversational-memory '{"message": "Thank you"}' \
  -H "X-Session-ID: user-123"
```

### `rag-search`
**Framework:** LangChain
**Model:** Mistral (local)
**Vector Store:** FAISS with 80 document chunks

**What it demonstrates:**
- Workspace persistence for vector indices
- Semantic search with embeddings
- Production RAG patterns

---

## Long-Running Examples

**Purpose:** Demonstrate multi-phase workflows with extended timeouts

### `competitive-analysis`
**Framework:** OpenAI + Anthropic SDK
**Models:** GPT-4o (research) + Claude 3.5 Sonnet (synthesis)
**Execution Time:** 5-10 minutes (full workflow)

**What it demonstrates:**
- 6-phase research pipeline
- Progress tracking with real-time logs
- Workspace preservation of intermediate results
- Multi-provider LLM orchestration

**Phases:**
1. Information Gathering (web search)
2. Source Extraction (structured data)
3. Comparative Analysis (feature matrix)
4. Market Intelligence (trend detection)
5. Strategic Synthesis (recommendations)
6. Report Generation (markdown output)

Completes all phases with 10-minute timeout support.

---

## Memory-Intensive Examples

**Purpose:** Demonstrate resource management and OOM detection

### `embedding-cache`
**Framework:** LangChain
**Model:** Mistral (local)
**Memory Pattern:** sentence-transformers (~400MB) + FAISS indices

**What it demonstrates:**
- Real ML memory patterns (not synthetic allocation)
- Memory tracking with psutil
- Configurable memory limits (256MB / 512MB / 1024MB)
- Ready for OOM testing (exit code 137 detection)

**Test scenarios:**
- memory: 512, collections: 2 → SUCCESS (~455MB)
- memory: 512, collections: 3 → OOM
- memory: 256, collections: 1 → OOM

---

## Quick Start

### Deploy an Agent

```bash
# Basic example
cd examples/basic/calculator-python
orpheus deploy .

# Test it
orpheus invoke calculator-python '{"query": "What is 15 times 7?"}'
```

### With Session Affinity

```bash
# Deploy stateful agent
cd examples/stateful/conversational-memory
orpheus deploy .

# Multiple requests with same session
orpheus invoke conversational-memory '{"message": "Hello"}' \
  -H "X-Session-ID: alice"

orpheus invoke conversational-memory '{"message": "How are you?"}' \
  -H "X-Session-ID: alice"
```

### Monitor Execution

```bash
# View logs in real-time
orpheus logs <agent-name> -f

# Check workspace contents
orpheus workspace info <agent-name>

# View execution history
orpheus execlog list <agent-name>
```

---

## Key Capabilities

### State Management
- ✅ State survives container restarts (workspace persistence)
- ✅ No complex volume mounting (workspace automatic)
- ✅ Real-time progress visibility (stdout streaming)

### Execution
- ✅ Long workflows complete (extended timeout support)
- ✅ Session affinity built-in (consistent routing)
- ✅ Simple deployment (single command deployment)
- ✅ Memory management (OCI cgroups enforcement)

### Production-Ready Patterns
- ✅ API-based agents (OpenAI, Anthropic, Gemini compatible)
- ✅ Local model support (Ollama integration)
- ✅ Multi-step workflows (research, analysis, synthesis)
- ✅ Persistent storage (SQLite, FAISS, file-based)
- ✅ Session continuity (conversation history, user context)

---

## Requirements

**For all agents:**
- Orpheus daemon running (`orpheus daemon start` or `orpheus vm start` on Mac)
- Python 3.10+ or Node.js 20+ (depending on runtime)

**For API-based agents:**
- Set environment variables before deployment:
  ```bash
  export OPENAI_API_KEY="sk-..."
  export ANTHROPIC_API_KEY="sk-ant-..."
  orpheus deploy .
  ```

**For local model agents:**
- Ollama installed and running
- Model pulled: `ollama pull mistral`

---

## Agent Comparison

| Agent | Complexity | External API | Setup Time | Best For |
|-------|------------|--------------|------------|----------|
| calculator-python | Simple | OpenAI | 1 min | Learning Orpheus |
| gmail-automation | Simple | Gmail API | 5 min | Automation workflows |
| rag-search | Medium | Ollama (local) | 2 min | Semantic search |
| conversational-memory | Medium | Ollama (local) | 2 min | Chatbots, assistants |
| embedding-cache | Medium | Ollama (local) | 3 min | Memory testing |
| competitive-analysis | Complex | OpenAI + Anthropic | 5 min | Research workflows |

---

## Contributing

When adding new examples:
1. Choose appropriate category (basic, stateful, long-running, memory-intensive)
2. Include comprehensive README.md
3. Add requirements.txt (Python) or package.json (Node.js)
4. Test deployment and execution
5. Document what the agent demonstrates

---

**Updated:** January 15, 2026
