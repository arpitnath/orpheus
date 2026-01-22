/**
 * Calculator Agent - Node.js version using OpenAI Agents SDK
 *
 * A mathematical calculator assistant with custom tools for various calculations.
 * This is the Node.js equivalent of the Python calculator agent.
 */

import { Agent, run, tool } from '@openai/agents';
import { z } from 'zod';

// Tool Functions

const addNumbers = tool({
  name: 'add_numbers',
  description: 'Add two numbers together',
  parameters: z.object({
    a: z.number().describe('First number'),
    b: z.number().describe('Second number')
  }),
  execute: async ({ a, b }) => String(a + b)
});

const subtractNumbers = tool({
  name: 'subtract_numbers',
  description: 'Subtract second number from first number',
  parameters: z.object({
    a: z.number().describe('First number'),
    b: z.number().describe('Second number')
  }),
  execute: async ({ a, b }) => String(a - b)
});

const multiplyNumbers = tool({
  name: 'multiply_numbers',
  description: 'Multiply two numbers together',
  parameters: z.object({
    a: z.number().describe('First number'),
    b: z.number().describe('Second number')
  }),
  execute: async ({ a, b }) => String(a * b)
});

const divideNumbers = tool({
  name: 'divide_numbers',
  description: 'Divide first number by second number',
  parameters: z.object({
    a: z.number().describe('Dividend'),
    b: z.number().describe('Divisor')
  }),
  execute: async ({ a, b }) => {
    if (b === 0) {
      return 'Error: Cannot divide by zero';
    }
    return String(a / b);
  }
});

const calculateCompoundInterest = tool({
  name: 'calculate_compound_interest',
  description: 'Calculate compound interest using the formula A = P(1 + r/n)^(nt)',
  parameters: z.object({
    principal: z.number().describe('Initial principal amount'),
    rate: z.number().describe('Annual interest rate (as decimal or percentage)'),
    time: z.number().describe('Time in years'),
    compoundsPerYear: z.number().default(1).describe('Number of times interest is compounded per year')
  }),
  execute: async ({ principal, rate, time, compoundsPerYear = 1 }) => {
    if (principal <= 0 || rate < 0 || time <= 0 || compoundsPerYear <= 0) {
      return 'Error: All values must be positive';
    }

    // Convert percentage to decimal if needed
    const r = rate > 1 ? rate / 100 : rate;

    const amount = principal * Math.pow(1 + r / compoundsPerYear, compoundsPerYear * time);
    const interest = amount - principal;

    return `Principal: $${principal.toLocaleString('en-US', { minimumFractionDigits: 2 })}, ` +
      `Final Amount: $${amount.toLocaleString('en-US', { minimumFractionDigits: 2 })}, ` +
      `Interest Earned: $${interest.toLocaleString('en-US', { minimumFractionDigits: 2 })}`;
  }
});

const calculateCircleArea = tool({
  name: 'calculate_circle_area',
  description: 'Calculate the area of a circle given its radius',
  parameters: z.object({
    radius: z.number().describe('Radius of the circle')
  }),
  execute: async ({ radius }) => {
    if (radius <= 0) {
      return 'Error: Radius must be positive';
    }
    const area = Math.PI * radius * radius;
    return `Circle with radius ${radius} has area ${area.toFixed(2)} square units`;
  }
});

const calculateTriangleArea = tool({
  name: 'calculate_triangle_area',
  description: 'Calculate the area of a triangle given base and height',
  parameters: z.object({
    base: z.number().describe('Base of the triangle'),
    height: z.number().describe('Height of the triangle')
  }),
  execute: async ({ base, height }) => {
    if (base <= 0 || height <= 0) {
      return 'Error: Base and height must be positive';
    }
    const area = 0.5 * base * height;
    return `Triangle with base ${base} and height ${height} has area ${area.toFixed(2)} square units`;
  }
});

const convertTemperature = tool({
  name: 'convert_temperature',
  description: 'Convert temperature between Celsius, Fahrenheit, and Kelvin',
  parameters: z.object({
    temperature: z.number().describe('Temperature value to convert'),
    fromUnit: z.string().describe('Source unit (celsius/c, fahrenheit/f, kelvin/k)'),
    toUnit: z.string().describe('Target unit (celsius/c, fahrenheit/f, kelvin/k)')
  }),
  execute: async ({ temperature, fromUnit, toUnit }) => {
    const from = fromUnit.toLowerCase();
    const to = toUnit.toLowerCase();

    // Convert to Celsius first
    let celsius;
    if (from === 'fahrenheit' || from === 'f') {
      celsius = (temperature - 32) * 5 / 9;
    } else if (from === 'kelvin' || from === 'k') {
      celsius = temperature - 273.15;
    } else if (from === 'celsius' || from === 'c') {
      celsius = temperature;
    } else {
      return 'Error: Supported units are Celsius, Fahrenheit, and Kelvin';
    }

    // Convert from Celsius to target unit
    let result, unitSymbol;
    if (to === 'fahrenheit' || to === 'f') {
      result = celsius * 9 / 5 + 32;
      unitSymbol = 'F';
    } else if (to === 'kelvin' || to === 'k') {
      result = celsius + 273.15;
      unitSymbol = 'K';
    } else if (to === 'celsius' || to === 'c') {
      result = celsius;
      unitSymbol = 'C';
    } else {
      return 'Error: Supported units are Celsius, Fahrenheit, and Kelvin';
    }

    return `${temperature}° ${fromUnit} = ${result.toFixed(2)}°${unitSymbol}`;
  }
});

// Agent Definition
const calculatorAgent = new Agent({
  name: 'Calculator Agent',
  instructions: `
    You are a mathematical calculator assistant with access to various calculation tools.

    You can help with:
    - Basic arithmetic (addition, subtraction, multiplication, division)
    - Compound interest calculations
    - Geometric calculations (circle and triangle areas)
    - Temperature conversions between Celsius, Fahrenheit, and Kelvin

    When users ask for calculations:
    1. Use the appropriate tool for the calculation
    2. Explain what calculation you're performing
    3. Show the result clearly
    4. Provide additional context if helpful

    Always use the provided tools rather than doing calculations yourself.
    Be helpful and explain your calculations step by step.
  `,
  tools: [
    addNumbers,
    subtractNumbers,
    multiplyNumbers,
    divideNumbers,
    calculateCompoundInterest,
    calculateCircleArea,
    calculateTriangleArea,
    convertTemperature
  ]
});

// Orpheus Handler
export async function handler(inputData) {
  /**
   * Orpheus entry point for calculator agent.
   *
   * @param {Object} inputData - Dict with 'query' key containing calculation request
   * @returns {Object} Dict with 'response' and status information
   */
  const query = inputData.query || inputData.input || '';

  if (!query) {
    return {
      error: 'No query provided',
      usage: "Provide a 'query' field with your calculation request",
      agent: 'calculator-nodejs'
    };
  }

  try {
    // Execute agent with query
    const result = await run(calculatorAgent, query);

    return {
      response: result.finalOutput,
      status: 'success',
      agent: 'calculator-nodejs'
    };
  } catch (e) {
    return {
      error: e.message,
      status: 'error',
      agent: 'calculator-nodejs'
    };
  }
}
