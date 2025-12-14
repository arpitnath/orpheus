#!/bin/bash
# Setup script for planning-agent
# This installs dependencies needed to run the agent

set -e

echo "[setup] Installing dependencies for planning-agent..."

# Install required packages
pip install openai-agents pydantic

echo "[setup] Dependencies installed successfully!"
echo ""
echo "To run the agent, set your OpenAI API key:"
echo "  export OPENAI_API_KEY='your-key-here'"
echo ""
echo "Then run:"
echo "  cd /Users/arpit/Desktop/playground2.0/test-agentrun/agentscale"
echo "  echo '{\"query\": \"Launch a marketing campaign for Q2 with 50k budget and 3 team members\"}' | ./isolate/bin/isolate run \"python3 bridge/run_agent.py examples/planning-agent/planning_agent.py\""
