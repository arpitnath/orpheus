// Package generator creates auto-generated entry point files for agents.
package generator

// PythonTemplate is the unified template for Python agents (handles both sync and async)
const PythonTemplate = `#!/usr/bin/env python3
"""Auto-generated entry point - DO NOT EDIT"""
import sys
import json
import asyncio
import traceback
import inspect

# Import the user's module
from {{.Module}} import {{.Entrypoint}}
{{if .InputType}}from {{.Module}} import {{.InputType}}{{end}}

def main():
    try:
        # Read input from stdin
        input_data = sys.stdin.read().strip()
        if input_data:
            data = json.loads(input_data)
        else:
            data = {}

        {{if .InputType}}
        # Parse input using Pydantic model
        input_obj = {{.InputType}}(**data)
        result = {{.Entrypoint}}(input_obj)
        {{else}}
        # Pass raw dict to handler
        result = {{.Entrypoint}}(data)
        {{end}}

        # Handle async functions
        if inspect.iscoroutine(result) or inspect.iscoroutinefunction({{.Entrypoint}}):
            # Handler is async - run in event loop
            result = asyncio.run(result) if inspect.iscoroutine(result) else asyncio.run({{.Entrypoint}}(data))

        # Handle result serialization
        if hasattr(result, 'model_dump'):
            output = result.model_dump()
        elif hasattr(result, 'dict'):
            output = result.dict()
        elif isinstance(result, dict):
            output = result
        else:
            output = {"result": str(result)}

        # Write JSON output to stdout
        print(json.dumps(output))

    except Exception as e:
        error_output = {
            "error": str(e),
            "traceback": traceback.format_exc(),
            "status": "error"
        }
        print(json.dumps(error_output))
        sys.exit(1)

if __name__ == "__main__":
    main()  # No asyncio.run() here - only use it if handler is async
`

// PythonAsyncTemplate is kept for backwards compatibility
const PythonAsyncTemplate = PythonTemplate

// PythonSyncTemplate is kept for backwards compatibility
const PythonSyncTemplate = PythonTemplate
