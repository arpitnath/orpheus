#!/usr/bin/env python3
"""
Deep Research Agent - Persistent, memory-enabled research assistant.

Features:
- Multi-step web research on any topic
- Persistent memory using mem0 (survives restarts)
- Report generation saved to workspace
- Cross-session knowledge retention
"""

import os
import sys
import json
import asyncio
from datetime import datetime
from typing import Any, Dict

from openai import AsyncOpenAI
from agents import Agent, Runner, set_default_openai_client, WebSearchTool

# Import tools from tools package
from tools import (
    store_memory,
    search_memory,
    recall_all_memories,
    write_report,
    list_past_reports,
    read_report,
)

# ─────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────

# Model configuration - using GPT-5 mini for cost-effective research
MODEL = "gpt-5-mini"

# Initialize OpenAI client
client = AsyncOpenAI(
    api_key=os.getenv("OPENAI_API_KEY"),
    timeout=300.0
)
set_default_openai_client(client)

# ─────────────────────────────────────────────────────────────
# Agent Instructions
# ─────────────────────────────────────────────────────────────

RESEARCH_INSTRUCTIONS = """You are a world-class research analyst with perfect memory. You help users research any topic thoroughly and remember findings across sessions.

## Your Capabilities

1. **Web Research**: Search the web for current information using WebSearchTool
2. **Memory**: Store and recall information across sessions using memory tools
3. **Report Generation**: Create structured research reports saved to workspace

## Research Workflow

When given a research query:

1. **Check Memory First**
   - Use `search_memory` or `recall_all_memories` to see if we've researched this before
   - Identify what we already know vs what needs updating
   - Tell the user what you remember

2. **Plan Research**
   - Break complex topics into searchable sub-questions
   - Identify what sources would be authoritative

3. **Execute Research**
   - Use WebSearchTool for each sub-question
   - Cross-reference multiple sources
   - Note publication dates for recency

4. **Store Findings**
   - Use `store_memory` for key facts with:
     - category: "fact", "source", "finding", "trend"
     - confidence: "high", "medium", "low"
     - topic: the research topic
   - Store atomic facts (one concept per memory)

5. **Generate Output**
   - Synthesize findings into coherent narrative
   - Include source citations
   - Note what we learned vs what we already knew
   - Use `write_report` for detailed reports when asked

## Research Depth Levels

Based on the "depth" parameter:
- **quick**: 2-3 searches, brief summary (~2 minutes)
- **standard**: 5-7 searches, detailed summary with sources (~5 minutes)
- **deep**: 10+ searches, comprehensive report with analysis (~10 minutes)

## Output Format

Structure your final response as JSON:

```json
{
  "topic": "The research topic",
  "prior_knowledge": "What we already knew from memory (if any)",
  "new_findings": "Summary of what we learned",
  "key_facts": ["List of important facts discovered"],
  "sources": ["List of source URLs used"],
  "confidence": "Overall confidence: high/medium/low",
  "memories_stored": 3,
  "follow_up_questions": ["Suggested further research topics"]
}
```

## Important Rules

- Always check memory before researching (avoid duplicate work)
- Cite sources with URLs when available
- Distinguish between facts and opinions
- Note when information might be outdated
- Store important findings in memory for future sessions
- Be transparent about uncertainty
"""


# ─────────────────────────────────────────────────────────────
# Agent Creation
# ─────────────────────────────────────────────────────────────

def create_research_agent() -> Agent:
    """Create the Deep Research Agent with all tools."""
    return Agent(
        name="Deep Research Agent",
        model=MODEL,
        instructions=RESEARCH_INSTRUCTIONS,
        tools=[
            # Web search (native OpenAI tool)
            WebSearchTool(search_context_size="high"),
            # Memory tools
            store_memory,
            search_memory,
            recall_all_memories,
            # File tools
            write_report,
            list_past_reports,
            read_report,
        ]
    )


# ─────────────────────────────────────────────────────────────
# Main Handler (Orpheus Entry Point)
# ─────────────────────────────────────────────────────────────

def handler(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Orpheus handler function for Deep Research Agent.

    Input:
    {
        "query": "Research topic or question",
        "depth": "quick" | "standard" | "deep",  # Optional, default: standard
        "output_format": "summary" | "report",   # Optional, default: summary
        "session_id": "optional-session-id"
    }

    Output:
    {
        "status": "success" | "error",
        "result": { ... research findings ... },
        "execution_time_seconds": 12.5
    }
    """
    start_time = datetime.now()

    try:
        # Extract parameters
        query = input_data.get("query", "")
        if not query:
            return {
                "status": "error",
                "error": "Query is required",
                "agent": "deep-research-agent"
            }

        depth = input_data.get("depth", "standard")
        output_format = input_data.get("output_format", "summary")
        session_id = input_data.get("session_id", f"research-{datetime.now().strftime('%Y%m%d%H%M%S')}")

        print(f"[RESEARCH] Starting research: {query[:100]}...", file=sys.stderr)
        print(f"[RESEARCH] Depth: {depth}, Format: {output_format}", file=sys.stderr)

        # Create the agent
        agent = create_research_agent()

        # Build the prompt with depth instruction
        research_prompt = f"""Research Query: {query}

Depth Level: {depth}
Output Format: {output_format}

Please conduct research following your workflow:
1. First check your memory for any prior knowledge on this topic
2. Then search the web for current information
3. Store important findings in memory
4. {"Generate a full report using write_report" if output_format == "report" else "Provide a structured summary"}
"""

        # Run the agent
        result = asyncio.run(Runner.run(agent, input=research_prompt))

        execution_time = (datetime.now() - start_time).total_seconds()
        print(f"[RESEARCH] Completed in {execution_time:.2f}s", file=sys.stderr)

        # Extract the result
        output = result.final_output if hasattr(result, 'final_output') else str(result)

        # Try to parse as JSON if the agent returned structured output
        try:
            parsed_output = json.loads(output)
        except json.JSONDecodeError:
            parsed_output = {"raw_response": output}

        return {
            "status": "success",
            "result": parsed_output,
            "agent": "deep-research-agent",
            "session_id": session_id,
            "depth": depth,
            "execution_time_seconds": round(execution_time, 2)
        }

    except Exception as e:
        execution_time = (datetime.now() - start_time).total_seconds()
        print(f"[RESEARCH] Error after {execution_time:.2f}s: {str(e)}", file=sys.stderr)

        return {
            "status": "error",
            "error": str(e),
            "agent": "deep-research-agent",
            "execution_time_seconds": round(execution_time, 2)
        }


# ─────────────────────────────────────────────────────────────
# CLI Entry Point (for local testing)
# ─────────────────────────────────────────────────────────────

if __name__ == "__main__":
    # For local testing: python agent.py '{"query": "..."}'
    if len(sys.argv) > 1:
        try:
            input_data = json.loads(sys.argv[1])
        except json.JSONDecodeError:
            # Treat as plain query string
            input_data = {"query": sys.argv[1]}
    else:
        # Read from stdin (Orpheus mode)
        input_data = json.loads(sys.stdin.read())

    result = handler(input_data)
    print(json.dumps(result, indent=2))
