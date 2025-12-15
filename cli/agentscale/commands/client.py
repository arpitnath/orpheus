"""HTTP client commands for AgentScale server."""

import json
from typing import Optional

import httpx
import typer

from agentscale.utils.output import print_error, print_json, print_success

DEFAULT_SERVER = "http://localhost:8080"


def invoke(
    input_data: str = typer.Argument(..., help="JSON input for the agent"),
    server: str = typer.Option(DEFAULT_SERVER, "--server", "-s", help="Server URL"),
    timeout: int = typer.Option(300, "--timeout", "-t", help="Request timeout in seconds"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Invoke an agent on a running AgentScale server."""

    # Parse input (accept raw string or JSON)
    try:
        parsed_input = json.loads(input_data)
    except json.JSONDecodeError:
        # Treat as raw string input
        parsed_input = {"input": input_data}

    url = f"{server.rstrip('/')}/invoke"

    try:
        with httpx.Client(timeout=timeout) as client:
            response = client.post(url, json=parsed_input)

            if response.status_code == 200:
                result = response.json()
                if raw:
                    print(json.dumps(result))
                else:
                    status = result.get("status", "unknown")
                    print_json(result, title=f"Result ({status})")
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

    except httpx.ConnectError:
        print_error("Connection failed", f"Could not connect to {server}")
        raise typer.Exit(1)
    except httpx.TimeoutException:
        print_error("Request timeout", f"Request timed out after {timeout}s")
        raise typer.Exit(1)


def status(
    server: str = typer.Option(DEFAULT_SERVER, "--server", "-s", help="Server URL"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Get server status and statistics."""

    url = f"{server.rstrip('/')}/stats"

    try:
        with httpx.Client(timeout=10) as client:
            response = client.get(url)

            if response.status_code == 200:
                stats = response.json()
                if raw:
                    print(json.dumps(stats))
                else:
                    print_json(stats, title="Server Status")
            else:
                print_error(f"Request failed ({response.status_code})", response.text)
                raise typer.Exit(1)

    except httpx.ConnectError:
        print_error("Connection failed", f"Could not connect to {server}")
        raise typer.Exit(1)


def health(
    server: str = typer.Option(DEFAULT_SERVER, "--server", "-s", help="Server URL"),
) -> None:
    """Check server health."""

    url = f"{server.rstrip('/')}/health"

    try:
        with httpx.Client(timeout=5) as client:
            response = client.get(url)

            if response.status_code == 200:
                data = response.json()
                agent_id = data.get("agent_id", "unknown")
                tier = data.get("tier", "unknown")
                print_success(f"Server healthy - agent: {agent_id}, tier: {tier}")
            else:
                print_error("Server unhealthy", f"Status code: {response.status_code}")
                raise typer.Exit(1)

    except httpx.ConnectError:
        print_error("Server unreachable", f"Could not connect to {server}")
        raise typer.Exit(1)
