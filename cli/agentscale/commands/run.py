"""Run command implementation - daemon only."""

import json
import sys
from pathlib import Path
from typing import Optional

import httpx
import typer

from agentscale.utils.output import print_json, print_error
from agentscale.utils.client import get_client, test_connection
from agentscale.config import get_active_server

app = typer.Typer()


def run_via_daemon(
    agent_dir: str,
    input_data: dict,
    memory: Optional[int],
    timeout: Optional[int],
    idle_timeout: Optional[int],
) -> dict:
    """Execute agent via daemon.

    Uses smart client that auto-detects Unix socket vs TCP mode.
    """
    try:
        client = get_client()

        request_body = {
            "agent_path": str(Path(agent_dir).resolve()),
            "input": input_data,
            "options": {}
        }
        if memory:
            request_body["options"]["memory_limit"] = memory
        if timeout:
            request_body["options"]["timeout"] = timeout
        if idle_timeout:
            request_body["options"]["idle_timeout"] = idle_timeout

        resp = client.post("http://localhost/v1/agents/run", json=request_body)
        return resp.json()

    except httpx.ConnectError:
        server_config = get_active_server()
        mode = server_config.get("mode", "unix_socket")

        if mode == "unix_socket":
            print_error(
                "Cannot connect to daemon",
                "Start daemon with: orpheus vm start (macOS) or orpheus daemon start (Linux)"
            )
        else:
            url = server_config.get("url", "unknown")
            print_error(
                f"Cannot connect to server: {url}",
                "Check that server is running and URL is correct"
            )
        raise typer.Exit(1)


@app.command()
def run(
    agent_dir: str = typer.Argument(..., help="Path to agent directory"),
    memory: Optional[int] = typer.Option(None, "--memory", "-m", help="Memory limit (MB)"),
    timeout: Optional[int] = typer.Option(None, "--timeout", "-t", help="Timeout (seconds)"),
    idle_timeout: Optional[int] = typer.Option(None, "--idle-timeout", help="Idle timeout (seconds)"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Run an agent via the daemon.

    Automatically connects to active server (local or remote).
    Use 'orpheus login' to configure remote servers.

    Examples:
        echo '{"query": "hello"}' | orpheus run ./my-agent
        orpheus run ./my-agent --memory 512 --timeout 60
    """
    # Read stdin
    input_data = {}
    if not sys.stdin.isatty():
        stdin_data = sys.stdin.read()
        if stdin_data:
            try:
                input_data = json.loads(stdin_data)
            except json.JSONDecodeError:
                input_data = {"raw_input": stdin_data}

    # Execute via daemon (smart client handles Unix socket vs TCP)
    try:
        output = run_via_daemon(
            agent_dir, input_data,
            memory, timeout, idle_timeout
        )
        exit_code = 0 if output.get("status") == "success" else 1
    except httpx.TimeoutException:
        print_error("Request timeout", "Agent execution timed out")
        raise typer.Exit(1)
    except Exception as e:
        print_error(f"Error: {e}")
        raise typer.Exit(1)

    # Print output
    if raw:
        print(json.dumps(output))
    else:
        status = output.get("status", "unknown")
        title = f"Agent Result ({status})"
        print_json(output, title=title)

    raise typer.Exit(exit_code)
