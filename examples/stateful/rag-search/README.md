# RAG Search Agent

This agent implements a **Retrieval Augmented Generation (RAG)** pipeline. It demonstrates how to manage persistent vector indexes and cache search results using the Workspace.

## How it Works

1.  **Ingestion:** You can send documents to be "indexed".
2.  **Persistence:** The agent saves the document embeddings (simulated) into a vector store located in `/workspace/index/`.
3.  **Search:** When you ask a query, it:
    *   Checks the result cache in `/workspace/cache/`.
    *   If miss, scans the persistent index.
    *   Returns the most relevant document.

## Key Features

*   **Persistent Index:** The vector database lives in `/workspace`, surviving redeploys.
*   **Result Caching:** Expensive search operations are cached to disk to save compute.
*   **Stateful updates:** You can incrementally add documents to the index over time.

## Usage

```bash
# Add a document
orpheus run rag-search '{"action": "index", "doc": "Orpheus uses warm workers."}'

# Search
orpheus run rag-search '{"action": "search", "query": "warm workers"}'
```
