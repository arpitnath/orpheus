"""
Simple Agent - For testing AgentScale bridge without external dependencies

This agent demonstrates the minimal handler pattern:
- Receives JSON input via stdin (handled by bridge)
- Returns JSON output via stdout (handled by bridge)
"""

def handler(input_data: dict) -> dict:
    """
    Simple handler function - the minimal AgentScale pattern.

    Args:
        input_data: Dict with 'query' key from stdin JSON

    Returns:
        Dict that will be serialized to JSON on stdout
    """
    query = input_data.get('query', '')

    # Simple echo + processing
    return {
        "response": f"Processed: {query}",
        "input_length": len(query),
        "agent": "simple-agent",
        "version": "1.0.0"
    }


# Also support async pattern
async def run(input_data: dict) -> dict:
    """Alternative async entry point"""
    return handler(input_data)
