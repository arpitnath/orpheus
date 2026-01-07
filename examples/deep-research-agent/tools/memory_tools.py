"""
Memory tools for Deep Research Agent using mem0.

Provides persistent memory that survives agent restarts.
Uses local ChromaDB storage in the workspace directory.
"""

import os
import sys
import json
from datetime import datetime
from agents import function_tool

# Workspace directory for memory storage
MEMORY_DIR = "./memory"

# Lazy initialization of memory
_memory = None


def get_memory():
    """Lazy initialize mem0 memory with local ChromaDB storage."""
    global _memory
    if _memory is None:
        try:
            from mem0 import Memory
            os.makedirs(MEMORY_DIR, exist_ok=True)

            # mem0 1.0+ API uses Memory.from_config()
            config = {
                "vector_store": {
                    "provider": "chroma",
                    "config": {
                        "collection_name": "research_memory",
                        "path": os.path.join(MEMORY_DIR, "chroma_db")
                    }
                }
            }
            _memory = Memory.from_config(config)
            print(f"[MEMORY] Initialized at {MEMORY_DIR}", file=sys.stderr)
        except Exception as e:
            print(f"[MEMORY] Init failed: {e}", file=sys.stderr)
            _memory = None
    return _memory


@function_tool
def store_memory(content: str, category: str, topic: str, confidence: str = "medium") -> str:
    """
    Store a piece of research knowledge in long-term memory.

    Args:
        content: The information to remember (a fact, finding, or insight)
        category: Type of memory - one of: fact, source, finding, trend, preference
        topic: The research topic this relates to
        confidence: Confidence level - one of: high, medium, low

    Returns:
        Confirmation message with memory storage status
    """
    memory = get_memory()
    if memory is None:
        return json.dumps({"status": "error", "message": "Memory not available"})

    try:
        result = memory.add(
            content,
            user_id="research_agent",
            metadata={
                "category": category,
                "topic": topic,
                "confidence": confidence,
                "timestamp": datetime.now().isoformat()
            }
        )
        # mem0 1.0+ returns {'results': [{'id': '...', 'memory': '...', 'event': 'ADD'}]}
        results_list = result.get("results", [])
        memory_id = results_list[0].get("id", "unknown") if results_list else "unknown"

        return json.dumps({
            "status": "success",
            "message": f"Stored {category} about '{topic}' with {confidence} confidence",
            "memory_id": memory_id
        })
    except Exception as e:
        return json.dumps({"status": "error", "message": str(e)})


@function_tool
def search_memory(query: str, limit: int = 5) -> str:
    """
    Search past research memory for relevant information.

    Args:
        query: What to search for in memory
        limit: Maximum number of results to return (default 5)

    Returns:
        JSON string of relevant memories from past research
    """
    memory = get_memory()
    if memory is None:
        return json.dumps({"status": "error", "memories": [], "message": "Memory not available"})

    try:
        result = memory.search(query, user_id="research_agent", limit=limit)
        # mem0 1.0+ returns {'results': [...]}
        results_list = result.get("results", [])

        memories = []
        for r in results_list:
            memories.append({
                "content": r.get("memory", ""),
                "metadata": r.get("metadata", {}),
                "score": r.get("score", 0)
            })

        return json.dumps({
            "status": "success",
            "query": query,
            "memories": memories,
            "count": len(memories)
        })
    except Exception as e:
        return json.dumps({"status": "error", "memories": [], "message": str(e)})


@function_tool
def recall_all_memories(topic: str) -> str:
    """
    Recall all memories related to a specific research topic.

    Args:
        topic: The research topic to recall everything about

    Returns:
        All stored knowledge about this topic
    """
    memory = get_memory()
    if memory is None:
        return json.dumps({"status": "error", "memories": [], "message": "Memory not available"})

    try:
        result = memory.search(topic, user_id="research_agent", limit=20)
        # mem0 1.0+ returns {'results': [...]}
        results_list = result.get("results", [])

        memories = []
        for r in results_list:
            memories.append({
                "content": r.get("memory", ""),
                "category": r.get("metadata", {}).get("category", "unknown"),
                "confidence": r.get("metadata", {}).get("confidence", "unknown"),
                "timestamp": r.get("metadata", {}).get("timestamp", "unknown")
            })

        return json.dumps({
            "status": "success",
            "topic": topic,
            "memories": memories,
            "total_found": len(memories)
        })
    except Exception as e:
        return json.dumps({"status": "error", "memories": [], "message": str(e)})
