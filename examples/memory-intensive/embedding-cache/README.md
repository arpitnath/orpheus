# Embedding Cache Agent

**Memory-Intensive ML Workload for OOM Testing**

This agent demonstrates Orpheus's OOM detection and recovery with real ML workloads (not synthetic allocation).

## Purpose

Tests Orpheus ServiceManager's ability to:
- ✅ Detect OOM kills (exit code 137)
- ✅ Apply exponential backoff after OOM (60s+ delays)
- ✅ Auto-replace workers after 3 failures
- ✅ Handle real ML memory patterns (embeddings + FAISS)

## How It Works

**Memory Pattern:**
- Embedding model: ~400MB (sentence-transformers/all-MiniLM-L6-v2)
- FAISS index: ~150MB per collection (10K document chunks)
- Multiple collections = multiplicative memory usage

**Actions:**
- `load`: Load collections into memory (triggers OOM if over limit)
- `search`: Search loaded collections
- `status`: Return memory usage and loaded collections

## Setup

**Pre-built FAISS indices are already included!** No setup script needed.

The agent comes with 3 pre-built collections (collection_1, collection_2, collection_3) in `data/collections/`. These indices are copied from the RAG agent example and use the same embedding model (sentence-transformers/all-MiniLM-L6-v2).

**Deploy the agent directly:**
```bash
cd examples/embedding-cache-agent
orpheus deploy .
```

Note: The `setup_indices.py` script is kept for reference but is not required for normal use.

## Testing Configurations

### Test 1: Baseline (Should Succeed)

**Config:**
```yaml
memory: 1024
env:
  - MAX_COLLECTIONS=1
```

**Expected Memory:** ~550MB (400MB model + 150MB index)

**Test:**
```bash
# Edit agent.yaml: memory: 1024, MAX_COLLECTIONS=1
orpheus undeploy embedding-cache-agent
orpheus deploy .
orpheus invoke embedding-cache-agent '{"action":"load"}'
```

**Expected Result:**
```json
{
  "status": "success",
  "action": "load",
  "collections_loaded": ["collection_1"],
  "memory_mb": 550.0,
  "load_time_sec": 15.2
}
```

### Test 2: Moderate OOM (Should Fail)

**Config:**
```yaml
memory: 512
env:
  - MAX_COLLECTIONS=2
```

**Expected Memory:** ~700MB (400MB model + 2×150MB indices)
**Container Limit:** 512MB
**Result:** **OOM (exit 137)**

**Test:**
```bash
# Edit agent.yaml: memory: 512, MAX_COLLECTIONS=2
orpheus undeploy embedding-cache-agent
orpheus deploy .
orpheus invoke embedding-cache-agent '{"action":"load"}'
```

**Expected Result:**
- Agent process killed by OOM killer
- ServiceManager detects exit code 137
- ExecLog marks request as FAILED
- Worker enters backoff state (60s minimum)

**Verify:**
```bash
# Check ExecLog for FAILED state
orpheus execlog list embedding-cache-agent

# Check daemon logs for OOM detection
orpheus logs embedding-cache-agent | grep -i oom
```

### Test 3: Aggressive OOM (Even 1 Collection Fails)

**Config:**
```yaml
memory: 256
env:
  - MAX_COLLECTIONS=1
```

**Expected Memory:** ~550MB (too large for 256MB limit)
**Result:** **OOM (exit 137)**

### Test 4: Repeated OOM (Backoff Test)

**Test exponential backoff:**

```bash
# Edit agent.yaml: memory: 256, MAX_COLLECTIONS=1
orpheus undeploy embedding-cache-agent
orpheus deploy .

# Invoke 5 times rapidly
for i in {1..5}; do
  echo "Request $i:"
  orpheus invoke embedding-cache-agent '{"action":"load"}'
  sleep 2
done

# Check logs for backoff progression
orpheus logs embedding-cache-agent | grep -i backoff
```

**Expected backoff progression:**
- Request 1: 2s delay
- Request 2: 4s delay
- Request 3: 8s delay
- Request 4: 16s delay
- Request 5: 32s delay
- After 3 OOMs: Worker marked unhealthy → auto-replaced

## Usage Examples

### Load Collections

```bash
orpheus invoke embedding-cache-agent '{"action":"load"}'
```

Output:
```json
{
  "status": "success",
  "action": "load",
  "collections_loaded": ["collection_1", "collection_2"],
  "memory_mb": 687.3,
  "load_time_sec": 18.5,
  "max_collections": 2,
  "embedding_model_loaded": true,
  "load_times": {
    "embedding_model": 12.3,
    "collection_collection_1": 3.1,
    "collection_collection_2": 3.1
  }
}
```

### Search Collection

```bash
orpheus invoke embedding-cache-agent '{
  "action": "search",
  "collection": "collection_1",
  "query": "machine learning",
  "top_k": 3
}'
```

Output:
```json
{
  "status": "success",
  "action": "search",
  "collection": "collection_1",
  "query": "machine learning",
  "results": [
    {
      "content": "Machine learning is a subset of artificial intelligence...",
      "metadata": {"collection": "collection_1", "doc_id": 1}
    }
  ],
  "retrieval_ms": 45,
  "memory_mb": 687.3
}
```

### Get Status

```bash
orpheus invoke embedding-cache-agent '{"action":"status"}'
```

Output:
```json
{
  "status": "success",
  "action": "status",
  "memory_mb": 687.3,
  "max_collections": 2,
  "collections_loaded": ["collection_1", "collection_2"],
  "embedding_model_loaded": true,
  "load_times": {
    "embedding_model": 12.3,
    "collection_collection_1": 3.1,
    "collection_collection_2": 3.1
  }
}
```

## Memory Tracking

The agent uses `psutil` to track actual RSS memory usage:

```python
process = psutil.Process()
memory_mb = process.memory_info().rss / 1024 / 1024
```

Memory is logged before/after each load:
```
[cache] Loading embedding model: sentence-transformers/all-MiniLM-L6-v2
[cache] This will use approximately 400MB of memory...
[cache] Embedding model loaded: 398.2MB in 12.3s (total: 412.5MB)
[cache] Loading collection 'collection_1' from .../faiss_index
[cache] This will use approximately 150MB of memory...
[cache] Collection 'collection_1' loaded: 147.8MB in 3.1s (total: 560.3MB)
```

## OOM Validation Checklist

When testing OOM scenarios, verify:

- [ ] Exit code 137 captured by ServiceManager
- [ ] ExecLog marks request as FAILED (not COMPLETED)
- [ ] Daemon logs show: `[service-manager] OOM detected for model, applying aggressive backoff`
- [ ] Backoff delays increase exponentially: 2s → 4s → 8s → 16s → 32s → 60s (capped)
- [ ] Worker marked unhealthy after 3 consecutive OOMs
- [ ] New worker spawned automatically
- [ ] Memory logging shows actual usage (not estimates)

## Technical Details

**Framework:** LangChain
**Embeddings:** sentence-transformers/all-MiniLM-L6-v2 (~400MB)
**Vector Store:** FAISS (in-memory)
**Model:** Mistral (via Orpheus ServiceManager)

**Pre-built Indices:**
- 3 collections included (collection_1, collection_2, collection_3)
- Copied from the rag-agent example (same embedding model)
- Each index: ~160KB (index.faiss + index.pkl)
- Works out-of-the-box on Mac without building sentence-transformers

**Technical Details:**
- Uses FAISS + HuggingFaceEmbeddings pattern
- ServiceManager injects MODEL_URL automatically
- Compatible with same embedding model as RAG agent

**Real ML workload characteristics:**
- Authentic ML memory patterns from sentence-transformers
- Production use case: caching multiple document collections
- Realistic memory usage from embeddings and FAISS indices
- Tests actual overhead from NumPy arrays and model weights

## Troubleshooting

**"Collection not found"**
- Pre-built indices should be in `data/collections/{collection_1,collection_2,collection_3}/faiss_index/`
- Verify the directory structure exists: `ls -la data/collections/`
- Each collection should have `index.faiss` and `index.pkl` files

**"Model server not available"**
- Ensure Ollama is running: `ollama list`
- Check MODEL_URL in logs

**OOM not triggering**
- Verify memory limit in agent.yaml
- Increase MAX_COLLECTIONS to load more indices
- Check actual memory usage with `orpheus logs`

**Backoff not working**
- Check daemon logs for OOM detection
- Verify exit code 137 in ExecLog
- Ensure ServiceManager is configured correctly
