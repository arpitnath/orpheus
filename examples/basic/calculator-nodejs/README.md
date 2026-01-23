# Node.js Calculator

A simple but complete example of an Orpheus agent written in **Node.js**.

## Overview

This agent demonstrates how to use the **Node.js 20 runtime**. It accepts a JSON payload with a math expression and returns the result. It calculates usage of `eval()` (safe-guarded) to process the math.

## Code Structure

*   `agent.js`: The main entrypoint. Exports a `handler` function.
*   `agent.yaml`: Configuration specifying `runtime: nodejs20`.

## Usage

```bash
orpheus run calculator-nodejs '{"expression": "10 * 10"}'
```

Returns:
```json
{
  "result": 100,
  "status": "success"
}
```
