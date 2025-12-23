"""Run command implementation - daemon only."""

import json
import sys
from pathlib import Path
from typing import Optional

import httpx
import typer

from agentscale.utils.output import print_json, print_error

app = typer.Typer()


def get_daemon_socket() -> Path:
    """Get daemon socket path based on OS."""
    if sys.platform == "darwin":
        # macOS: Lima-forwarded socket
        return Path.home() / ".lima" / "agentscale" / "sock" / "agentscale.sock"
    # Linux: Local socket
    return Path("/var/run/agentscale.sock")


def daemon_available(socket_path: Path) -> bool:
    """Check if daemon is running and healthy."""
    if not socket_path.exists():
        return False
    try:
        transport = httpx.HTTPTransport(uds=str(socket_path))
        with httpx.Client(transport=transport, timeout=2) as client:
            resp = client.get("http://localhost/v1/health")
            return resp.status_code == 200
    except Exception:
        return False


def run_via_daemon(
    socket_path: Path,
    agent_dir: str,
    input_data: dict,
    memory: Optional[int],
    timeout: Optional[int],
    idle_timeout: Optional[int],
) -> dict:
    """Execute agent via daemon."""
    transport = httpx.HTTPTransport(uds=str(socket_path))
    with httpx.Client(transport=transport, timeout=600) as client:
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


@app.command()
def run(
    agent_dir: str = typer.Argument(..., help="Path to agent directory"),
    memory: Optional[int] = typer.Option(None, "--memory", "-m", help="Memory limit (MB)"),
    timeout: Optional[int] = typer.Option(None, "--timeout", "-t", help="Timeout (seconds)"),
    idle_timeout: Optional[int] = typer.Option(None, "--idle-timeout", help="Idle timeout (seconds)"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Run an agent via the daemon.

    Requires daemon to be running. Start with:
      - macOS: agentscale vm start
      - Linux: agentscale daemon start

    Examples:
        echo '{"query": "hello"}' | agentscale run ./my-agent
        agentscale run ./my-agent --memory 512 --timeout 60
    """
    # Check daemon availability
    socket_path = get_daemon_socket()

    if not daemon_available(socket_path):
        if sys.platform == "darwin":
            print_error("Daemon not running", "Start with: agentscale vm start")
        else:
            print_error("Daemon not running", "Start with: agentscale daemon start")
        raise typer.Exit(1)

    # Read stdin
    input_data = {}
    if not sys.stdin.isatty():
        stdin_data = sys.stdin.read()
        if stdin_data:
            try:
                input_data = json.loads(stdin_data)
            except json.JSONDecodeError:
                input_data = {"raw_input": stdin_data}

    # Execute via daemon
    try:
        output = run_via_daemon(
            socket_path, agent_dir, input_data,
            memory, timeout, idle_timeout
        )
        exit_code = 0 if output.get("status") == "success" else 1
    except httpx.ConnectError:
        print_error("Daemon connection failed", "Is the daemon running?")
        raise typer.Exit(1)
    except httpx.TimeoutException:
        print_error("Request timeout", "Agent execution timed out")
        raise typer.Exit(1)
    except Exception as e:
        print_error(f"Daemon error: {e}")
        raise typer.Exit(1)

    # Print output
    if raw:
        print(json.dumps(output))
    else:
        status = output.get("status", "unknown")
        title = f"Agent Result ({status})"
        print_json(output, title=title)

    raise typer.Exit(exit_code)
