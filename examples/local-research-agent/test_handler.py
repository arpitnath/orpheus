import json
import sys

# Import the handler
from agent import handler

# Test with simple query
test_input = {"query": "What time is it?"}

print("Testing handler with:", json.dumps(test_input))
print("=" * 60)

result = handler(test_input)

print("\nResult:")
print(json.dumps(result, indent=2))
