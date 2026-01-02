"""HTTP client commands for AgentScale server."""

import json
from typing import Optional

import httpx
import typer

from agentscale.utils.output import print_error, print_json, print_success
from agentscale.utils.client import get_client


def invoke(
    agent_id: str = typer.Argument(..., help="Agent ID to invoke"),
    input_data: str = typer.Argument(..., help="JSON input for the agent"),
    timeout: int = typer.Option(300, "--timeout", "-t", help="Request timeout in seconds"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Invoke an agent on a running AgentScale server.

    Uses the active server from ~/.orpheus/config.yaml

    Examples:
        orpheus invoke planning-agent '{"query": "test"}'
        orpheus invoke simple-agent '{"input": "hello"}'
    """

    # Parse input (accept raw string or JSON)
    try:
        parsed_input = json.loads(input_data)
    except json.JSONDecodeError:
        # Treat as raw string input
        parsed_input = {"input": input_data}

    # Build URL with agent query parameter
    url = f"/invoke?agent={agent_id}"

    try:
        with get_client(timeout=timeout) as client:
            response = client.post(url, json=parsed_input)

            if response.status_code == 200:
                result = response.json()
                if raw:
                    print(json.dumps(result))
                else:
                    status = result.get("status", "unknown")
                    print_json(result, title=f"Result from {agent_id} ({status})")
            elif response.status_code == 404:
                error_data = response.json()
                print_error("Agent not found", error_data.get("error", f"Unknown agent: {agent_id}"))
                raise typer.Exit(1)
            elif response.status_code == 503:
                error_data = response.json()
                print_error("Server unavailable", error_data.get("error", "Queue full"))
                raise typer.Exit(1)
            elif response.status_code == 504:
                print_error("Request timeout", "Agent execution timed out")
                raise typer.Exit(1)
            else:
                print_error(f"Request failed ({response.status_code})", response.text)
                raise typer.Exit(1)

    except httpx.ConnectError as e:
        print_error("Connection failed", f"Could not connect to server: {e}")
        raise typer.Exit(1)
    except httpx.TimeoutException:
        print_error("Request timeout", f"Request timed out after {timeout}s")
        raise typer.Exit(1)


def stats(
    agent_id: Optional[str] = typer.Argument(None, help="Agent ID (optional, shows all if omitted)"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Get server statistics for one or all agents.

    Uses the active server from ~/.orpheus/config.yaml

    Examples:
        orpheus stats                    # All agents
        orpheus stats calculator-test    # Specific agent
    """

    if agent_id:
        url = f"/v1/stats?agent={agent_id}"
    else:
        url = "/v1/stats"

    try:
        with get_client(timeout=10) as client:
            response = client.get(url)

            if response.status_code == 200:
                stats_data = response.json()
                if raw:
                    print(json.dumps(stats_data))
                else:
                    if agent_id:
                        print_json(stats_data, title=f"Stats for {agent_id}")
                    else:
                        print_json(stats_data, title="All Agent Stats")
            elif response.status_code == 404:
                print_error("Agent not found", f"Unknown agent: {agent_id}")
                raise typer.Exit(1)
            else:
                print_error(f"Request failed ({response.status_code})", response.text)
                raise typer.Exit(1)

    except httpx.ConnectError as e:
        print_error("Connection failed", f"Could not connect to server: {e}")
        raise typer.Exit(1)


def health() -> None:
    """Check server health.

    Uses the active server from ~/.orpheus/config.yaml
    """

    url = "/v1/health"

    try:
        with get_client(timeout=5) as client:
            response = client.get(url)

            if response.status_code == 200:
                data = response.json()
                status = data.get("status", "unknown")
                version = data.get("version", "unknown")
                uptime = data.get("uptime_seconds", 0)
                running = data.get("running_agents", 0)

                print_success(f"Server healthy")
                print(f"  Version: {version}")
                print(f"  Uptime: {uptime}s")
                print(f"  Running agents: {running}")
            else:
                print_error("Server unhealthy", f"Status code: {response.status_code}")
                raise typer.Exit(1)

    except httpx.ConnectError as e:
        print_error("Server unreachable", f"Could not connect to server: {e}")
        raise typer.Exit(1)
