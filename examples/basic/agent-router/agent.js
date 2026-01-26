/**
 * Agent Router
 *
 * An Orpheus agent that uses Claude (via Anthropic SDK) to route requests
 * to other Orpheus agents via MCP protocol. This demonstrates:
 * 1. Agent-to-agent communication via MCP
 * 2. Anthropic SDK MCP connector integration
 * 3. Orchestration patterns with Orpheus
 */

import Anthropic from "@anthropic-ai/sdk";

// Environment variables (set in agent.yaml)
const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;
const MCP_URL = process.env.MCP_URL;

/**
 * Orpheus handler for MCP client agent.
 *
 * @param {Object} inputData - Input with 'query' field
 * @returns {Object} Response with result or error
 */
export async function handler(inputData) {
  const query = inputData.query || inputData.input || "";

  if (!query) {
    return {
      error: "No query provided",
      usage: "Provide a 'query' field with your request",
      agent: "agent-router",
    };
  }

  if (!ANTHROPIC_API_KEY) {
    return {
      error: "ANTHROPIC_API_KEY not configured",
      agent: "agent-router",
    };
  }

  if (!MCP_URL) {
    return {
      error: "MCP_URL not configured",
      agent: "agent-router",
    };
  }

  try {
    // Initialize Anthropic client with MCP beta header
    const client = new Anthropic({
      apiKey: ANTHROPIC_API_KEY,
      defaultHeaders: {
        "anthropic-beta": "mcp-client-2025-11-20",
      },
    });

    // Call Claude with MCP connection to the target agent
    const response = await client.messages.create({
      model: "claude-sonnet-4-20250514",
      max_tokens: 1024,
      mcp_servers: [
        {
          type: "url",
          url: MCP_URL,
          name: "orpheus-target-agent",
        },
      ],
      tools: [
        {
          type: "mcp_toolset",
          mcp_server_name: "orpheus-target-agent",
        },
      ],
      messages: [
        {
          role: "user",
          content: query,
        },
      ],
    });

    // Extract text response
    let textResponse = "";
    const toolCalls = [];

    for (const block of response.content) {
      if (block.type === "text") {
        textResponse += block.text;
      } else if (block.type === "tool_use" || block.type === "mcp_tool_use") {
        toolCalls.push({
          tool: block.name,
          input: block.input,
          server_name: block.server_name || null,
          type: block.type,
        });
      }
    }

    return {
      response: textResponse,
      tool_calls: toolCalls,
      stop_reason: response.stop_reason,
      status: "success",
      agent: "agent-router",
      mcp_target: MCP_URL,
    };
  } catch (error) {
    return {
      error: error.message,
      status: "error",
      agent: "agent-router",
    };
  }
}
