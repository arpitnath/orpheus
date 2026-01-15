#!/usr/bin/env python3
"""
RAG Agent for Orpheus: Retrieval-Augmented Generation using FAISS + LLM.

This agent demonstrates Orpheus's core value proposition:

1. Queue-Depth Autoscaling (not CPU):
   - LLM inference is I/O-bound, CPU stays at ~3 millicores
   - Orpheus scales on queue depth, not CPU metrics
   - Result: 38x faster queue drain vs CPU-based HPA

2. Native Model Server Management (ServiceManager):
   - Agent declares `model: mistral` in agent.yaml
   - Orpheus auto-detects platform: macOS → Ollama, Linux+GPU → vLLM
   - No Docker/k8s sidecar config, just works

The agent:
1. Retrieves relevant documents from a FAISS index
2. Generates answers using LLM (model server managed by Orpheus)
3. Returns structured results with sources and timing
"""

import os
import time
from pathlib import Path

# RAG dependencies
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import FAISS
from langchain_ollama import OllamaLLM
from langchain_core.prompts import PromptTemplate

# Configuration
INDEX_DIR = os.getenv("INDEX_DIR", "./data/faiss_index")
EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "sentence-transformers/all-MiniLM-L6-v2")
TOP_K = int(os.getenv("TOP_K", "4"))

# Model endpoint - ServiceManager injects MODEL_URL automatically
# Fallback to OLLAMA_BASE_URL for manual override or local testing
MODEL_URL = os.getenv("MODEL_URL") or os.getenv("OLLAMA_BASE_URL", "http://localhost:11434")
MODEL_NAME = os.getenv("OLLAMA_MODEL", "mistral")

# RAG prompt template
RAG_PROMPT = PromptTemplate(
    input_variables=["context", "question"],
    template="""You are a helpful assistant that answers questions based on the provided context.
Use only the information from the context to answer. If the context doesn't contain
enough information to answer, say "I don't have enough information to answer this question."

Context:
{context}

Question: {question}

Answer:""",
)


class RAGEngine:
    """RAG engine that retrieves context and generates answers."""

    def __init__(self):
        self.vectorstore = None
        self.llm = None
        self._initialized = False
        self._timings = {}

    def initialize(self):
        """Load the FAISS index and initialize the LLM."""
        if self._initialized:
            return

        # Load embeddings model
        print(f"[rag] Loading embedding model: {EMBEDDING_MODEL}")
        start = time.time()
        embeddings = HuggingFaceEmbeddings(
            model_name=EMBEDDING_MODEL,
            model_kwargs={"device": "cpu"},
            encode_kwargs={"normalize_embeddings": True},
        )
        self._timings["embeddings_load"] = time.time() - start

        # Load FAISS index
        index_path = Path(INDEX_DIR)
        if not index_path.exists():
            raise FileNotFoundError(
                f"FAISS index not found at {INDEX_DIR}. "
                "Copy the index to /workspace/faiss_index or set INDEX_DIR."
            )

        print(f"[rag] Loading FAISS index from {INDEX_DIR}")
        start = time.time()
        self.vectorstore = FAISS.load_local(
            str(index_path),
            embeddings,
            allow_dangerous_deserialization=True,
        )
        self._timings["index_load"] = time.time() - start

        # Initialize LLM - endpoint provided by Orpheus ServiceManager
        print(f"[rag] Connecting to model server at {MODEL_URL} (model: {MODEL_NAME})")
        self.llm = OllamaLLM(
            base_url=MODEL_URL,
            model=MODEL_NAME,
            temperature=0.1,
        )

        self._initialized = True
        print(f"[rag] RAG engine initialized (embeddings: {self._timings['embeddings_load']:.2f}s, index: {self._timings['index_load']:.2f}s)")

    def retrieve(self, question: str, top_k: int = None) -> tuple:
        """Retrieve relevant documents for a question."""
        if not self._initialized:
            self.initialize()

        top_k = top_k or TOP_K

        start_time = time.time()
        docs = self.vectorstore.similarity_search(question, k=top_k)
        elapsed = time.time() - start_time

        print(f"[rag] Retrieved {len(docs)} docs in {elapsed:.3f}s")
        return docs, elapsed

    def generate(self, question: str, context_docs: list) -> tuple:
        """Generate an answer using the LLM."""
        if not self._initialized:
            self.initialize()

        # Format context from retrieved documents
        context = "\n\n---\n\n".join(
            [f"[{i+1}] {doc.page_content}" for i, doc in enumerate(context_docs)]
        )

        # Create prompt
        prompt = RAG_PROMPT.format(context=context, question=question)

        # Generate response
        start_time = time.time()
        response = self.llm.invoke(prompt)
        elapsed = time.time() - start_time

        print(f"[rag] LLM generated response in {elapsed:.3f}s")
        return response, elapsed

    def answer(self, question: str) -> dict:
        """Full RAG pipeline: retrieve + generate."""
        if not self._initialized:
            self.initialize()

        total_start = time.time()

        # Retrieve relevant context
        docs, retrieval_time = self.retrieve(question)

        # Generate answer
        answer, llm_time = self.generate(question, docs)

        total_time = time.time() - total_start

        return {
            "question": question,
            "answer": answer,
            "sources": [
                {
                    "content": doc.page_content[:200] + "...",
                    "metadata": doc.metadata,
                }
                for doc in docs
            ],
            "timing": {
                "retrieval_ms": int(retrieval_time * 1000),
                "llm_ms": int(llm_time * 1000),
                "total_ms": int(total_time * 1000),
            },
        }


# Global RAG engine instance (persists across invocations via workspace)
_engine = None


def get_engine() -> RAGEngine:
    """Get or create the global RAG engine instance."""
    global _engine
    if _engine is None:
        _engine = RAGEngine()
    return _engine


def handler(input_data: dict) -> dict:
    """
    Orpheus agent handler for RAG queries.

    Input:
        {"question": "What is X?"}
        or
        {"query": "What is X?"}  # alias

    Output:
        {
            "status": "success",
            "agent": "rag-agent",
            "answer": "...",
            "sources": [...],
            "timing": {...}
        }
    """
    try:
        # Get question from input (support both 'question' and 'query')
        question = input_data.get("question") or input_data.get("query")

        if not question:
            return {
                "status": "error",
                "agent": "rag-agent",
                "error": "Missing 'question' or 'query' field in input",
            }

        # Get RAG engine and process question
        engine = get_engine()
        result = engine.answer(question)

        return {
            "status": "success",
            "agent": "rag-agent",
            "answer": result["answer"],
            "sources": result["sources"],
            "timing": result["timing"],
        }

    except FileNotFoundError as e:
        return {
            "status": "error",
            "agent": "rag-agent",
            "error": str(e),
            "hint": "Ensure FAISS index is available at INDEX_DIR",
        }
    except Exception as e:
        return {
            "status": "error",
            "agent": "rag-agent",
            "error": str(e),
        }
