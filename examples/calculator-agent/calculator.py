"""
Calculator Agent - Adapted for AgentScale

A mathematical calculator assistant with 8 custom tools for various calculations.
Adapted from awesome-llm-apps OpenAI SDK crash course.
"""

import math
from agents import Agent, Runner, function_tool

# Tool Functions

@function_tool
def add_numbers(a: float, b: float) -> float:
    """Add two numbers together"""
    return a + b

@function_tool
def subtract_numbers(a: float, b: float) -> float:
    """Subtract second number from first number"""
    return a - b

@function_tool
def multiply_numbers(a: float, b: float) -> float:
    """Multiply two numbers together"""
    return a * b

@function_tool
def divide_numbers(a: float, b: float) -> float:
    """Divide first number by second number"""
    if b == 0:
        return "Error: Cannot divide by zero"
    return a / b

@function_tool
def calculate_compound_interest(principal: float, rate: float, time: int, compounds_per_year: int = 1) -> str:
    """Calculate compound interest using the formula A = P(1 + r/n)^(nt)"""
    if principal <= 0 or rate < 0 or time <= 0 or compounds_per_year <= 0:
        return "Error: All values must be positive"

    # Convert percentage to decimal if needed
    if rate > 1:
        rate = rate / 100

    amount = principal * (1 + rate/compounds_per_year) ** (compounds_per_year * time)
    interest = amount - principal

    return f"Principal: ${principal:,.2f}, Final Amount: ${amount:,.2f}, Interest Earned: ${interest:,.2f}"

@function_tool
def calculate_circle_area(radius: float) -> str:
    """Calculate the area of a circle given its radius"""
    if radius <= 0:
        return "Error: Radius must be positive"

    area = math.pi * radius ** 2
    return f"Circle with radius {radius} has area {area:.2f} square units"

@function_tool
def calculate_triangle_area(base: float, height: float) -> str:
    """Calculate the area of a triangle given base and height"""
    if base <= 0 or height <= 0:
        return "Error: Base and height must be positive"

    area = 0.5 * base * height
    return f"Triangle with base {base} and height {height} has area {area:.2f} square units"

@function_tool
def convert_temperature(temperature: float, from_unit: str, to_unit: str) -> str:
    """Convert temperature between Celsius, Fahrenheit, and Kelvin"""
    from_unit = from_unit.lower()
    to_unit = to_unit.lower()

    # Convert to Celsius first
    if from_unit == "fahrenheit" or from_unit == "f":
        celsius = (temperature - 32) * 5/9
    elif from_unit == "kelvin" or from_unit == "k":
        celsius = temperature - 273.15
    elif from_unit == "celsius" or from_unit == "c":
        celsius = temperature
    else:
        return "Error: Supported units are Celsius, Fahrenheit, and Kelvin"

    # Convert from Celsius to target unit
    if to_unit == "fahrenheit" or to_unit == "f":
        result = celsius * 9/5 + 32
        unit_symbol = "°F"
    elif to_unit == "kelvin" or to_unit == "k":
        result = celsius + 273.15
        unit_symbol = "K"
    elif to_unit == "celsius" or to_unit == "c":
        result = celsius
        unit_symbol = "°C"
    else:
        return "Error: Supported units are Celsius, Fahrenheit, and Kelvin"

    return f"{temperature}° {from_unit.title()} = {result:.2f}{unit_symbol}"

# Agent Definition

calculator_agent = Agent(
    name="Calculator Agent",
    instructions="""
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
    """,
    tools=[
        add_numbers,
        subtract_numbers,
        multiply_numbers,
        divide_numbers,
        calculate_compound_interest,
        calculate_circle_area,
        calculate_triangle_area,
        convert_temperature
    ]
)

# AgentScale Handler

def handler(input_data: dict) -> dict:
    """
    AgentScale entry point for calculator agent.

    Args:
        input_data: Dict with 'query' key containing calculation request

    Returns:
        Dict with 'response' and status information
    """
    query = input_data.get('query', input_data.get('input', ''))

    if not query:
        return {
            "error": "No query provided",
            "usage": "Provide a 'query' field with your calculation request",
            "agent": "calculator-agent"
        }

    try:
        # Execute agent with query
        result = Runner.run_sync(calculator_agent, query)

        return {
            "response": result.final_output,
            "status": "success",
            "agent": "calculator-agent"
        }
    except Exception as e:
        return {
            "error": str(e),
            "status": "error",
            "agent": "calculator-agent"
        }
