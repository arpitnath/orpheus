# Calculator Agent (Node.js)

A mathematical calculator assistant built with the OpenAI Agents SDK for JavaScript/TypeScript.

## Features

- Basic arithmetic (add, subtract, multiply, divide)
- Compound interest calculations
- Geometric calculations (circle area, triangle area)
- Temperature conversions (Celsius, Fahrenheit, Kelvin)

## Prerequisites

- Node.js 20+
- OpenAI API key (set as `OPENAI_API_KEY` environment variable)

## Local Development

```bash
# Install dependencies
npm install

# Set your OpenAI API key
export OPENAI_API_KEY=sk-...

# Test locally (not in container)
node -e "
import('./agent.js').then(m => {
  m.handler({ query: 'What is 2 + 2?' }).then(console.log);
});
"
```

## Deploy to Orpheus

```bash
cd examples/calculator-nodejs
orpheus deploy .

# Invoke the agent
orpheus invoke calculator-nodejs '{"query": "What is 2 + 2?"}'
```

## Example Queries

- "What is 25 times 4?"
- "Calculate the area of a circle with radius 5"
- "Convert 100 degrees Fahrenheit to Celsius"
- "Calculate compound interest on $1000 at 5% for 10 years"

## OpenAI Agents SDK

This agent uses the [OpenAI Agents SDK for JavaScript](https://github.com/openai/openai-agents-js):

```javascript
import { Agent, run, tool } from '@openai/agents';
import { z } from 'zod';

const addNumbers = tool({
  name: 'add_numbers',
  description: 'Add two numbers',
  parameters: z.object({ a: z.number(), b: z.number() }),
  execute: async ({ a, b }) => String(a + b)
});

const agent = new Agent({
  name: 'Calculator',
  instructions: 'You are a calculator assistant',
  tools: [addNumbers]
});

const result = await run(agent, 'What is 2 + 2?');
console.log(result.finalOutput);
```

## Handler Pattern

Orpheus expects an async `handler` function that takes input data and returns a result:

```javascript
export async function handler(inputData) {
  const query = inputData.query || inputData.input || '';
  const result = await run(agent, query);
  return { response: result.finalOutput, status: 'success' };
}
```
