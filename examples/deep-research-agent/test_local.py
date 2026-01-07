#!/usr/bin/env python3
"""
Local testing script for Deep Research Agent.

Run without Orpheus to validate the agent works correctly.

Usage:
    python test_local.py                    # Interactive mode
    python test_local.py "your query"       # Single query mode
"""

import os
import sys
import json

# Ensure we're in the agent directory for relative paths
os.chdir(os.path.dirname(os.path.abspath(__file__)))

from agent import handler


def test_basic_research():
    """Test 1: Basic web research without memory."""
    print("\n" + "="*60)
    print("TEST 1: Basic Web Research")
    print("="*60)

    result = handler({
        "query": "What are the latest developments in AI agents in 2025?",
        "depth": "quick"
    })

    print(json.dumps(result, indent=2))
    return result["status"] == "success"


def test_memory_store_and_recall():
    """Test 2: Store something in memory, then recall it."""
    print("\n" + "="*60)
    print("TEST 2: Memory Store and Recall")
    print("="*60)

    # First research
    print("\n--- Step 1: Research a topic ---")
    result1 = handler({
        "query": "Research SpaceX Starship progress",
        "depth": "quick"
    })
    print(f"Status: {result1['status']}")

    # Recall
    print("\n--- Step 2: Recall what we learned ---")
    result2 = handler({
        "query": "What do you remember about SpaceX Starship from our previous research?",
        "depth": "quick"
    })
    print(json.dumps(result2, indent=2))

    return result1["status"] == "success" and result2["status"] == "success"


def test_report_generation():
    """Test 3: Generate a full report."""
    print("\n" + "="*60)
    print("TEST 3: Report Generation")
    print("="*60)

    result = handler({
        "query": "Compare different approaches to nuclear fusion energy",
        "depth": "standard",
        "output_format": "report"
    })

    print(json.dumps(result, indent=2))

    # Check if report was created
    if os.path.exists("./reports"):
        reports = os.listdir("./reports")
        print(f"\nReports in workspace: {reports}")

    return result["status"] == "success"


def interactive_mode():
    """Interactive testing mode."""
    print("\n" + "="*60)
    print("Deep Research Agent - Interactive Mode")
    print("="*60)
    print("Type your research queries. Commands:")
    print("  'quit' or 'exit' - Exit")
    print("  'quick: <query>' - Quick research")
    print("  'deep: <query>' - Deep research")
    print("  'report: <query>' - Generate report")
    print("="*60)

    while True:
        try:
            user_input = input("\n> ").strip()

            if not user_input:
                continue

            if user_input.lower() in ["quit", "exit"]:
                print("Goodbye!")
                break

            # Parse depth prefix
            depth = "standard"
            output_format = "summary"

            if user_input.startswith("quick:"):
                depth = "quick"
                user_input = user_input[6:].strip()
            elif user_input.startswith("deep:"):
                depth = "deep"
                user_input = user_input[5:].strip()
            elif user_input.startswith("report:"):
                depth = "deep"
                output_format = "report"
                user_input = user_input[7:].strip()

            print(f"\nResearching ({depth})...")

            result = handler({
                "query": user_input,
                "depth": depth,
                "output_format": output_format
            })

            print("\n" + "-"*40)
            if result["status"] == "success":
                if isinstance(result.get("result"), dict):
                    # Pretty print the research result
                    res = result["result"]
                    if "raw_response" in res:
                        print(res["raw_response"])
                    else:
                        print(json.dumps(res, indent=2))
                else:
                    print(result.get("result", "No result"))
            else:
                print(f"Error: {result.get('error', 'Unknown error')}")

            print(f"\n[Execution time: {result.get('execution_time_seconds', 0)}s]")

        except KeyboardInterrupt:
            print("\n\nInterrupted. Goodbye!")
            break
        except Exception as e:
            print(f"Error: {e}")


def main():
    # Check for OpenAI API key
    if not os.getenv("OPENAI_API_KEY"):
        print("ERROR: OPENAI_API_KEY environment variable not set")
        print("Set it with: export OPENAI_API_KEY='your-key'")
        sys.exit(1)

    if len(sys.argv) > 1:
        # Single query mode
        query = " ".join(sys.argv[1:])
        result = handler({"query": query, "depth": "standard"})
        print(json.dumps(result, indent=2))
    else:
        # Interactive mode
        interactive_mode()


if __name__ == "__main__":
    main()
