#!/usr/bin/env python3
"""
AgentScale Bridge - Internal Runtime Component

Connects the stdin/stdout contract to actual agent code.
This is NOT user-facing code - developers never touch this.

Contract:
  INPUT:  JSON object via stdin
  OUTPUT: JSON object via stdout
  LOGS:   Text via stderr
  EXIT:   0 = success, non-zero = failure

Usage:
  echo '{"query": "..."}' | python run_agent.py /path/to/agent.py
"""

import sys
import os
import json
import asyncio
import importlib.util
import traceback
from typing import Any, Optional
from pathlib import Path


def log(message: str):
    """Log to stderr (captured as agent thoughts/logs)"""
    print(f"[agentscale] {message}", file=sys.stderr)


def load_agent_module(agent_path: str):
    """Dynamically load an agent module from file path"""
    path = Path(agent_path).resolve()

    if not path.exists():
        raise FileNotFoundError(f"Agent file not found: {agent_path}")

    # Add agent's directory to path for relative imports
    agent_dir = str(path.parent)
    if agent_dir not in sys.path:
        sys.path.insert(0, agent_dir)

    spec = importlib.util.spec_from_file_location("agent_module", path)
    if spec is None or spec.loader is None:
        raise ImportError(f"Could not load agent from: {agent_path}")

    module = importlib.util.module_from_spec(spec)
    sys.modules["agent_module"] = module
    spec.loader.exec_module(module)

    return module


def detect_agent_type(module) -> str:
    """Detect the type of agent based on available functions"""
    if hasattr(module, 'run_workflow'):
        return 'openai_agents_sdk'
    elif hasattr(module, 'handler'):
        return 'simple_handler'
    elif hasattr(module, 'main'):
        return 'main_function'
    elif hasattr(module, 'run'):
        return 'run_function'
    else:
        return 'unknown'


async def execute_agent(module, input_data: dict) -> Any:
    """Execute the agent based on its type"""
    agent_type = detect_agent_type(module)
    log(f"Detected agent type: {agent_type}")

    if agent_type == 'openai_agents_sdk':
        # OpenAI Agents SDK pattern - expects WorkflowInput with input_as_text
        WorkflowInput = getattr(module, 'WorkflowInput', None)

        if WorkflowInput is not None:
            # Create input using the agent's expected schema
            query = input_data.get('query', input_data.get('input', str(input_data)))
            workflow_input = WorkflowInput(input_as_text=str(query))
            result = await module.run_workflow(workflow_input)
        else:
            # Direct dict input
            result = await module.run_workflow(input_data)

        return result

    elif agent_type == 'simple_handler':
        # Simple sync handler pattern
        result = module.handler(input_data)
        if asyncio.iscoroutine(result):
            result = await result
        return result

    elif agent_type == 'main_function':
        # Main function pattern
        result = module.main(input_data)
        if asyncio.iscoroutine(result):
            result = await result
        return result

    elif agent_type == 'run_function':
        # Run function pattern
        result = module.run(input_data)
        if asyncio.iscoroutine(result):
            result = await result
        return result

    else:
        raise ValueError(
            f"Agent must have one of: run_workflow, handler, main, or run function. "
            f"Found: {dir(module)}"
        )


def serialize_result(result: Any) -> dict:
    """Convert agent result to JSON-serializable dict"""
    if result is None:
        return {"result": None, "status": "completed"}

    if isinstance(result, dict):
        return result

    if isinstance(result, str):
        return {"result": result}

    # Try pydantic model
    if hasattr(result, 'model_dump'):
        return {"result": result.model_dump()}

    # Try dict method
    if hasattr(result, 'dict'):
        return {"result": result.dict()}

    # Try to convert to string
    return {"result": str(result)}


async def main():
    # Check arguments
    if len(sys.argv) < 2:
        log("Error: No agent path provided")
        print(json.dumps({
            "error": "No agent path provided",
            "usage": "echo '{\"query\": \"...\"}' | python run_agent.py /path/to/agent.py"
        }))
        sys.exit(1)

    agent_path = sys.argv[1]
    log(f"Loading agent: {agent_path}")

    try:
        # Read input from stdin
        if sys.stdin.isatty():
            log("Warning: No stdin input, using empty query")
            input_data = {"query": ""}
        else:
            stdin_content = sys.stdin.read().strip()
            if not stdin_content:
                input_data = {"query": ""}
            else:
                input_data = json.loads(stdin_content)

        log(f"Input received: {json.dumps(input_data)[:200]}...")

        # Load agent module
        module = load_agent_module(agent_path)
        log(f"Agent loaded successfully")

        # Execute agent
        result = await execute_agent(module, input_data)

        # Serialize and output result
        output = serialize_result(result)
        output["status"] = "success"

        print(json.dumps(output))
        sys.exit(0)

    except json.JSONDecodeError as e:
        log(f"Error: Invalid JSON input - {e}")
        print(json.dumps({
            "error": f"Invalid JSON input: {str(e)}",
            "status": "error"
        }))
        sys.exit(1)

    except FileNotFoundError as e:
        log(f"Error: {e}")
        print(json.dumps({
            "error": str(e),
            "status": "error"
        }))
        sys.exit(1)

    except Exception as e:
        log(f"Error executing agent: {e}")
        log(traceback.format_exc())
        print(json.dumps({
            "error": str(e),
            "traceback": traceback.format_exc(),
            "status": "error"
        }))
        sys.exit(1)


if __name__ == "__main__":
    asyncio.run(main())
