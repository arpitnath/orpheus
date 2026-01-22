# RAG Agent

A Retrieval-Augmented Generation agent demonstrating queue-depth based autoscaling for LLM workloads.

## What This Demonstrates

**Queue-depth autoscaling for LLM workloads:**
- LLM inference is I/O-bound (waiting on network)
- Scales based on actual user demand (queue depth)
- More jobs waiting = more workers needed
- Efficient resource utilization for I/O-bound tasks

## Architecture

```
Question → FAISS Retrieval (~30ms) → Ollama/Mistral (~4-5s) → Answer
                 ↓                           ↓
            CPU: negligible            CPU: negligible
                    (Both are I/O-bound, not CPU-bound)
```

## Setup

### 1. Prerequisites

**Ollama with Mistral:**
```bash
# Install Ollama (https://ollama.ai)
ollama pull mistral
ollama serve  # Keep running
```

### 2. Install Dependencies

```bash
cd examples/rag-agent
./setup.sh
```

### 3. Prepare FAISS Index

Option A: Copy from blog experiment
```bash
cp -r /path/to/rag-queue-autoscale/data/faiss_index ./data/
```

Option B: Create sample index (see indexer below)

### 4. Deploy to Orpheus

```bash
orpheus deploy ./examples/rag-agent
```

## Usage

```bash
# Ask a question
orpheus invoke rag-agent '{"question": "What is machine learning?"}'

# Response includes timing breakdown
{
  "status": "success",
  "answer": "Machine learning is...",
  "timing": {
    "retrieval_ms": 28,
    "llm_ms": 4200,
    "total_ms": 4228
  }
}
```

## Scaling Configuration

From `agent.yaml`:

```yaml
scaling:
  min_workers: 1
  max_workers: 5
  target_utilization: 1.5
  scale_up_threshold: 2.0   # Scale up when queue > 2x workers
  scale_down_threshold: 0.3 # Scale down when queue < 0.3x workers
```

This means:
- 5 jobs queued with 1 worker → scale to 2 workers
- 10 jobs queued with 2 workers → scale to 4 workers
- Queue empty for 60s → scale down

## Timing Breakdown

Typical response times:

| Stage | Time | CPU Usage |
|-------|------|-----------|
| FAISS retrieval | ~30ms | negligible |
| LLM inference | ~4-5s | negligible |
| **Total** | **~4-5s** | **~3m** |

**99% of time is LLM inference (I/O wait). Queue-depth autoscaling provides efficient scaling.**
