"""
Local Research Agent - Powered by Mistral 7B via Ollama
Demonstrates local LLM with tool calling (no cloud, no API keys)
"""

import ollama
import json
import os
from datetime import datetime

# Configure Ollama client to use Orpheus-provided endpoint
# Falls back to localhost if not set (for standalone testing)
OLLAMA_HOST = os.getenv('OPENAI_BASE_URL', 'http://localhost:11434').replace('/v1', '')
ollama_client = ollama.Client(host=OLLAMA_HOST)

# Tools the agent can use
def search_docs(query: str) -> str:
    """Search documentation for a query"""
    # Simulated doc search (replace with real implementation)
    docs = {
        "python": "Python is a high-level programming language",
        "javascript": "JavaScript is a scripting language for web browsers",
        "rust": "Rust is a systems programming language",
    }

    for key, value in docs.items():
        if key.lower() in query.lower():
            return f"Found: {value}"

    return f"No documentation found for: {query}"

def get_current_time() -> str:
    """Get the current time"""
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")

def calculate(expression: str) -> str:
    """Safely evaluate a mathematical expression"""
    try:
        # Only allow basic math operations (safe)
        allowed_chars = set("0123456789+-*/()., ")
        if not all(c in allowed_chars for c in expression):
            return "Error: Expression contains invalid characters"

        result = eval(expression)
        return f"Result: {result}"
    except Exception as e:
        return f"Error: {str(e)}"

# Available tools
TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "search_docs",
            "description": "Search documentation for information about a topic",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "The search query"
                    }
                },
                "required": ["query"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "get_current_time",
            "description": "Get the current date and time",
            "parameters": {
                "type": "object",
                "properties": {},
                "required": []
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "calculate",
            "description": "Perform a mathematical calculation",
            "parameters": {
                "type": "object",
                "properties": {
                    "expression": {
                        "type": "string",
                        "description": "Mathematical expression to evaluate (e.g., '2 + 2', '10 * 5')"
                    }
                },
                "required": ["expression"]
            }
        }
    }
]

# Tool execution dispatcher
def execute_tool(tool_name: str, arguments: dict) -> str:
    """Execute a tool by name with given arguments"""
    if tool_name == "search_docs":
        return search_docs(**arguments)
    elif tool_name == "get_current_time":
        return get_current_time()
    elif tool_name == "calculate":
        return calculate(**arguments)
    else:
        return f"Error: Unknown tool '{tool_name}'"

def handler(input_data: dict) -> dict:
    """
    Orpheus agent entrypoint.

    Args:
        input_data: Dict with 'query' key containing the user's question

    Returns:
        Dict with 'response' and status information
    """
    query = input_data.get('query', input_data.get('input', ''))

    if not query:
        return {
            "error": "No query provided",
            "usage": "Provide a 'query' field with your question",
            "agent": "local-research-agent",
            "model": "mistral:7b (local)"
        }

    try:
        # Call Mistral with tools via Ollama
        response = ollama_client.chat(
            model='mistral',
            messages=[{
                'role': 'user',
                'content': query
            }],
            tools=TOOLS,
        )

        # Check if model wants to call tools
        message = response.get('message', {})
        tool_calls = message.get('tool_calls', [])

        if tool_calls:
            # Execute tool calls
            tool_results = []
            executed_tools = []

            for tool_call in tool_calls:
                function = tool_call.get('function', {})
                tool_name = function.get('name')
                arguments = function.get('arguments', {})

                # Execute the tool
                result = execute_tool(tool_name, arguments)
                executed_tools.append(f"{tool_name}({arguments})")

                tool_results.append({
                    'role': 'tool',
                    'content': result
                })

            # Get final answer from model
            final_response = ollama_client.chat(
                model='mistral',
                messages=[
                    {'role': 'user', 'content': query},
                    message,
                    *tool_results
                ]
            )

            return {
                "response": final_response['message']['content'],
                "tools_used": executed_tools,
                "status": "success",
                "agent": "local-research-agent",
                "model": "mistral:7b (local)"
            }
        else:
            # Direct answer, no tools
            return {
                "response": message.get('content', 'No response'),
                "tools_used": [],
                "status": "success",
                "agent": "local-research-agent",
                "model": "mistral:7b (local)"
            }

    except Exception as e:
        return {
            "error": str(e),
            "status": "error",
            "agent": "local-research-agent",
            "model": "mistral:7b (local)"
        }
