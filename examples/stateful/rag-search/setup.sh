#!/bin/bash
# Setup script for RAG agent

set -e

echo "Installing RAG agent dependencies..."

pip install --quiet \
    langchain>=0.1.0 \
    langchain-community>=0.0.10 \
    langchain-huggingface>=0.0.1 \
    langchain-ollama>=0.0.1 \
    faiss-cpu>=1.7.0 \
    sentence-transformers>=2.2.0

echo "Dependencies installed."
echo ""
echo "Prerequisites:"
echo "  1. Ollama running with Mistral model:"
echo "     ollama pull mistral"
echo "     ollama serve"
echo ""
echo "  2. FAISS index at /workspace/faiss_index or INDEX_DIR"
echo "     (Copy from blog experiment or run indexer)"
echo ""
echo "Done!"
