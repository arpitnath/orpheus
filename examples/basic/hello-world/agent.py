"""
Hello World Agent - Zero-dependency example for Orpheus

This agent demonstrates basic Orpheus functionality without requiring
any external dependencies, API keys, or services. Perfect for testing
and learning.
"""

import time
from typing import Dict, Any


async def handler(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Simple handler that demonstrates basic agent functionality.

    Supported operations:
    - greet: Returns personalized greeting(s)
    - calculate: Performs basic arithmetic
    - echo: Returns the input data

    Args:
        input_data: Dictionary with operation and parameters

    Returns:
        Dictionary with results and metadata
    """
    # Extract parameters
    operation = input_data.get('operation', 'greet')
    name = input_data.get('name', 'World')
    count = input_data.get('count', 1)

    results = []

    if operation == 'greet':
        # Generate greetings
        for i in range(count):
            results.append(f"Hello, {name}! (message #{i+1})")

    elif operation == 'calculate':
        # Basic arithmetic
        a = input_data.get('a', 0)
        b = input_data.get('b', 0)
        op = input_data.get('op', 'add')

        if op == 'add':
            result = a + b
        elif op == 'subtract':
            result = a - b
        elif op == 'multiply':
            result = a * b
        elif op == 'divide':
            result = a / b if b != 0 else 'Error: Division by zero'
        else:
            result = f'Unknown operation: {op}'

        results.append(f"{a} {op} {b} = {result}")

    elif operation == 'echo':
        # Echo back the input
        results.append(str(input_data))

    else:
        results.append(f"Unknown operation: {operation}")

    # Return response with metadata
    return {
        "results": results,
        "timestamp": time.time(),
        "operation": operation,
        "input_received": input_data
    }
