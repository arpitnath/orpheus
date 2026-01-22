#!/usr/bin/env python3
"""
Indexer: Builds FAISS vector index from documents in ./data/docs.
Persists index to disk so workers don't need to re-embed every start.
"""

import os
import sys
from pathlib import Path

from langchain_community.document_loaders import DirectoryLoader, TextLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import FAISS

# Configuration
DOCS_DIR = os.getenv("DOCS_DIR", "./data/docs")
INDEX_DIR = os.getenv("INDEX_DIR", "./data/faiss_index")
EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "sentence-transformers/all-MiniLM-L6-v2")
CHUNK_SIZE = int(os.getenv("CHUNK_SIZE", "500"))
CHUNK_OVERLAP = int(os.getenv("CHUNK_OVERLAP", "50"))


def load_documents(docs_dir: str) -> list:
    """Load all markdown and text files from the docs directory."""
    docs_path = Path(docs_dir)

    if not docs_path.exists():
        raise FileNotFoundError(f"Documents directory not found: {docs_dir}")

    # Load markdown files
    md_loader = DirectoryLoader(
        str(docs_path),
        glob="**/*.md",
        loader_cls=TextLoader,
        loader_kwargs={"encoding": "utf-8"},
    )

    # Load text files
    txt_loader = DirectoryLoader(
        str(docs_path),
        glob="**/*.txt",
        loader_cls=TextLoader,
        loader_kwargs={"encoding": "utf-8"},
    )

    docs = md_loader.load() + txt_loader.load()
    print(f"[indexer] Loaded {len(docs)} documents from {docs_dir}")

    return docs


def split_documents(docs: list) -> list:
    """Split documents into chunks for embedding."""
    splitter = RecursiveCharacterTextSplitter(
        chunk_size=CHUNK_SIZE,
        chunk_overlap=CHUNK_OVERLAP,
        separators=["\n\n", "\n", ". ", " ", ""],
    )

    chunks = splitter.split_documents(docs)
    print(
        f"[indexer] Split into {len(chunks)} chunks (size={CHUNK_SIZE}, overlap={CHUNK_OVERLAP})"
    )

    return chunks


def create_embeddings():
    """Initialize the embedding model."""
    print(f"[indexer] Loading embedding model: {EMBEDDING_MODEL}")
    embeddings = HuggingFaceEmbeddings(
        model_name=EMBEDDING_MODEL,
        model_kwargs={"device": "cpu"},
        encode_kwargs={"normalize_embeddings": True},
    )
    return embeddings


def build_index(chunks: list, embeddings) -> FAISS:
    """Build FAISS index from document chunks."""
    print(f"[indexer] Building FAISS index from {len(chunks)} chunks...")

    vectorstore = FAISS.from_documents(
        documents=chunks,
        embedding=embeddings,
    )

    print("[indexer] FAISS index built successfully")
    return vectorstore


def save_index(vectorstore: FAISS, index_dir: str):
    """Save FAISS index to disk."""
    index_path = Path(index_dir)
    index_path.mkdir(parents=True, exist_ok=True)

    vectorstore.save_local(str(index_path))
    print(f"[indexer] Index saved to {index_dir}")


def main():
    """Main indexer entry point."""
    print("=" * 60)
    print("RAG Indexer - Building FAISS Vector Index")
    print("=" * 60)

    # Load documents
    docs = load_documents(DOCS_DIR)
    if not docs:
        print("[indexer] ERROR: No documents found!")
        sys.exit(1)

    # Split into chunks
    chunks = split_documents(docs)

    # Create embeddings model
    embeddings = create_embeddings()

    # Build index
    vectorstore = build_index(chunks, embeddings)

    # Save to disk
    save_index(vectorstore, INDEX_DIR)

    print("=" * 60)
    print("Indexing complete!")
    print(f"  Documents: {len(docs)}")
    print(f"  Chunks: {len(chunks)}")
    print(f"  Index location: {INDEX_DIR}")
    print("=" * 60)


if __name__ == "__main__":
    main()
