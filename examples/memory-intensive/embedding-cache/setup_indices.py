#!/usr/bin/env python3
"""
Setup script to create sample FAISS indices for embedding-cache-agent.

⚠️ NOTE: Pre-built FAISS indices are already provided in data/collections/.
This script is NOT needed for normal use. It's kept for reference only.

The pre-built indices are copied from the rag-agent example and use the same
embedding model (sentence-transformers/all-MiniLM-L6-v2). You can deploy
the agent immediately without running this script.

If you want to regenerate the indices from scratch:
- Creates 3 collections (collection_1, collection_2, collection_3)
- Each: 100 documents → chunk → embed → FAISS index
- Saved to data/collections/{name}/faiss_index/
- Uses sentence-transformers/all-MiniLM-L6-v2 (same as agent)

Run this only if you need to regenerate:
    python setup_indices.py
"""

import sys
from pathlib import Path
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import FAISS
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_core.documents import Document

# Configuration
COLLECTIONS_DIR = Path("./data/collections")
EMBEDDING_MODEL = "sentence-transformers/all-MiniLM-L6-v2"
NUM_DOCS = 100  # Documents per collection
CHUNK_SIZE = 500
CHUNK_OVERLAP = 50

# Sample document content (will be varied per collection)
SAMPLE_TEXT_TEMPLATES = [
    "Machine learning is a subset of artificial intelligence that enables systems to learn and improve from experience without being explicitly programmed.",
    "Deep learning uses neural networks with multiple layers to progressively extract higher-level features from raw input.",
    "Natural language processing allows computers to understand, interpret, and generate human language in valuable ways.",
    "Computer vision enables machines to derive meaningful information from digital images, videos, and other visual inputs.",
    "Reinforcement learning is an area of machine learning where agents learn to make decisions by taking actions in an environment.",
    "Transfer learning involves taking a pre-trained model and adapting it to a new, related task.",
    "Data preprocessing is a crucial step in machine learning that involves cleaning and transforming raw data.",
    "Feature engineering is the process of using domain knowledge to extract features from raw data.",
    "Model evaluation metrics help determine how well a machine learning model performs on unseen data.",
    "Hyperparameter tuning is the process of optimizing the configuration of a machine learning algorithm.",
]


def create_sample_documents(collection_name: str, num_docs: int = NUM_DOCS) -> list:
    """Generate sample documents for a collection."""
    documents = []

    for i in range(num_docs):
        # Cycle through templates and add collection-specific variation
        template_idx = i % len(SAMPLE_TEXT_TEMPLATES)
        base_text = SAMPLE_TEXT_TEMPLATES[template_idx]

        # Add variation and context
        text = f"{base_text} In the context of {collection_name}, document {i+1}: "
        text += f"This is additional content to make the document longer and more realistic. "
        text += f"It contains information specific to {collection_name} and includes various technical details. "
        text += f"The document ID is {i+1} and it belongs to the {collection_name} collection. "
        text += base_text  # Repeat for more content

        doc = Document(
            page_content=text,
            metadata={
                "collection": collection_name,
                "doc_id": i + 1,
                "template": template_idx,
            },
        )
        documents.append(doc)

    return documents


def create_collection(name: str, embeddings: HuggingFaceEmbeddings):
    """Create a FAISS index for a collection."""
    print(f"\n{'=' * 60}")
    print(f"Creating collection: {name}")
    print(f"{'=' * 60}")

    # Create output directory
    output_dir = COLLECTIONS_DIR / name / "faiss_index"
    output_dir.parent.mkdir(parents=True, exist_ok=True)

    # Generate sample documents
    print(f"[{name}] Generating {NUM_DOCS} sample documents...")
    documents = create_sample_documents(name, NUM_DOCS)

    # Split documents into chunks
    print(f"[{name}] Splitting documents into chunks (size={CHUNK_SIZE}, overlap={CHUNK_OVERLAP})...")
    text_splitter = RecursiveCharacterTextSplitter(
        chunk_size=CHUNK_SIZE,
        chunk_overlap=CHUNK_OVERLAP,
    )
    chunks = text_splitter.split_documents(documents)
    print(f"[{name}] Created {len(chunks)} chunks from {len(documents)} documents")

    # Create FAISS index from chunks
    print(f"[{name}] Creating FAISS index (this will embed {len(chunks)} chunks)...")
    vectorstore = FAISS.from_documents(chunks, embeddings)

    # Save the index
    print(f"[{name}] Saving FAISS index to {output_dir}...")
    vectorstore.save_local(str(output_dir))

    print(f"[{name}] ✓ Collection created successfully!")
    print(f"[{name}]   Documents: {len(documents)}")
    print(f"[{name}]   Chunks: {len(chunks)}")
    print(f"[{name}]   Location: {output_dir}")

    return vectorstore


def main():
    """Create all sample collections."""
    print("=" * 60)
    print("Embedding Cache Agent - Setup Script")
    print("=" * 60)
    print(f"Embedding Model: {EMBEDDING_MODEL}")
    print(f"Collections Directory: {COLLECTIONS_DIR}")
    print(f"Documents per Collection: {NUM_DOCS}")
    print("=" * 60)

    # Load embedding model once (shared across all collections)
    print("\nLoading embedding model...")
    print("(This will download ~400MB on first run)")
    embeddings = HuggingFaceEmbeddings(
        model_name=EMBEDDING_MODEL,
        model_kwargs={"device": "cpu"},
        encode_kwargs={"normalize_embeddings": True},
    )
    print("✓ Embedding model loaded")

    # Create 3 sample collections
    collections = ["collection_1", "collection_2", "collection_3"]

    for collection_name in collections:
        try:
            create_collection(collection_name, embeddings)
        except Exception as e:
            print(f"\n[ERROR] Failed to create {collection_name}: {e}", file=sys.stderr)
            return 1

    # Summary
    print("\n" + "=" * 60)
    print("Setup Complete!")
    print("=" * 60)
    print(f"Created {len(collections)} collections:")
    for name in collections:
        index_path = COLLECTIONS_DIR / name / "faiss_index"
        print(f"  ✓ {name} → {index_path}")

    print("\nNext steps:")
    print("  1. Deploy the agent: orpheus deploy .")
    print('  2. Test loading: orpheus invoke embedding-cache-agent \'{"action":"load"}\'')
    print('  3. Test search: orpheus invoke embedding-cache-agent \'{"action":"search","collection":"collection_1","query":"machine learning"}\'')
    print()

    return 0


if __name__ == "__main__":
    sys.exit(main())
