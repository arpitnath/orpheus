# Embedding Cache Agent

This agent demonstrates **memory-intensive workloads** and **warm-start performance**. It loads a heavy embedding model (e.g., SentenceTransformers) into memory once during startup and reuses it for all subsequent requests.

## How it Works

1.  **Startup:** Loads the model (simulated 2GB model) into global memory. This happens only once when the worker starts.
2.  **Request:** Receives text input.
3.  **Processing:** Generates vector embeddings instantly using the pre-loaded model.
4.  **Output:** Returns the vector array.

## Key Features

*   **High Memory Config:** Uses `memory: 4096` (4GB) to accommodate the model.
*   **Zero Cold Starts:** Subsequent requests are millisecond-fast because the model stays in memory.
*   **Singleton Pattern:** Uses a global variable to cache the model across requests.

## Usage

```bash
orpheus run embedding-cache '{"text": "Hello world"}'
```
