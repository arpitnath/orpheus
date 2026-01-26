/**
 * Orpheus MCP Client
 *
 * A test client that uses Claude (via Anthropic SDK) to connect to
 * Orpheus agents via MCP protocol. This validates:
 * 1. MCP endpoint works end-to-end
 * 2. Agent-to-agent communication via MCP
 * 3. Anthropic SDK MCP connector integration
 */

import Anthropic from "@anthropic-ai/sdk";

const ORPHEUS_HOST = process.env.ORPHEUS_HOST || "http://localhost:7777";
const AGENT_NAME = process.env.AGENT_NAME || "calculator-python";
const MCP_ENDPOINT = `${ORPHEUS_HOST}/mcp/agents/${AGENT_NAME}`;

async function main() {
  console.log("=".repeat(50));
  console.log("  Orpheus MCP Client Test");
  console.log("=".repeat(50));
  console.log(`  MCP Endpoint: ${MCP_ENDPOINT}`);
  console.log("=".repeat(50));
  console.log();

  // Initialize Anthropic client with MCP beta header
  const client = new Anthropic({
    defaultHeaders: {
      "anthropic-beta": "mcp-client-2025-11-20"
    }
  });

  // Test query for the calculator agent
  const query = process.argv[2] || "Calculate 15 multiplied by 7, then add 23";

  console.log(`Query: ${query}\n`);
  console.log("Sending to Claude with MCP connection to Orpheus...\n");

  try {
    const response = await client.messages.create({
      model: "claude-sonnet-4-20250514",
      max_tokens: 1024,
      // @ts-expect-error - mcp_servers and tools.mcp_toolset are beta features
      mcp_servers: [
        {
          type: "url",
          url: MCP_ENDPOINT,
          name: "orpheus-calculator"
        }
      ],
      tools: [
        {
          type: "mcp_toolset",
          mcp_server_name: "orpheus-calculator"
        }
      ],
      messages: [
        {
          role: "user",
          content: query
        }
      ]
    });

    console.log("Response from Claude:");
    console.log("-".repeat(40));

    for (const block of response.content) {
      if (block.type === "text") {
        console.log(block.text);
      } else if (block.type === "tool_use") {
        console.log(`[Tool Call: ${block.name}]`);
        console.log(JSON.stringify(block.input, null, 2));
      }
    }

    console.log("-".repeat(40));
    console.log("\nMCP Integration Test: SUCCESS");

  } catch (error) {
    console.error("Error:", error);
    console.log("\nMCP Integration Test: FAILED");
    process.exit(1);
  }
}

main();
