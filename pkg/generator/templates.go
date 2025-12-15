// Package generator creates auto-generated entry point files for agents.
package generator

// PythonAsyncTemplate is the template for async Python agents
const PythonAsyncTemplate = `#!/usr/bin/env python3
"""Auto-generated entry point - DO NOT EDIT"""
import sys
import json
import asyncio
import traceback

# Import the user's module
from {{.Module}} import {{.Entrypoint}}
{{if .InputType}}from {{.Module}} import {{.InputType}}{{end}}

async def main():
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
        result = await {{.Entrypoint}}(input_obj)
        {{else}}
        # Pass raw dict to handler
        result = await {{.Entrypoint}}(data)
        {{end}}

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
    asyncio.run(main())
`

// PythonSyncTemplate is the template for synchronous Python agents
const PythonSyncTemplate = `#!/usr/bin/env python3
"""Auto-generated entry point - DO NOT EDIT"""
import sys
import json
import traceback

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
    main()
`
