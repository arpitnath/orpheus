"""PS command - List running containers."""

import sys
from pathlib import Path
from typing import Optional
import typer

from agentscale.utils.output import print_error, print_info


def ps(
    all: bool = typer.Option(False, "-a", "--all", help="Show all containers (including stopped)"),
) -> None:
    """List running containers.

    Shows currently running agent containers. Use -a to show all containers
    including stopped ones.

    Examples:
        orpheus ps      # Running containers
        orpheus ps -a   # All containers
    """
    # Get socket path
    if sys.platform == "darwin":
        socket_path = Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"
    else:
        socket_path = Path("/var/run/orpheus.sock")

    # Check if daemon is running
    if not socket_path.exists():
        print_error(
            "Daemon not running",
            "Start with: orpheus vm start" if sys.platform == "darwin" else "orpheus daemon start"
        )
        raise typer.Exit(1)

    # Query daemon API
    try:
        import httpx

        transport = httpx.HTTPTransport(uds=str(socket_path))
        with httpx.Client(transport=transport, timeout=5) as client:
            response = client.get("http://localhost/v1/agents/")
            data = response.json()

            agents = data.get("agents", [])

            if not agents:
                print_info("No containers running")
                return

            # Print header
            print("")
            print(f"{'ID':<12} {'AGENT':<20} {'STATUS':<10} {'STARTED':<15} {'DURATION':<10}")
            print("-" * 75)

            # Print agents
            for agent in agents:
                agent_id = agent.get("id", "")[:11]
                agent_name = agent.get("agent_path", "").split("/")[-1][:19]
                status = agent.get("status", "unknown")[:9]
                started = format_time_ago(agent.get("started_at", ""))
                duration = format_duration(agent.get("running_ms", 0))

                print(f"{agent_id:<12} {agent_name:<20} {status:<10} {started:<15} {duration:<10}")

            print("")
            print(f"Total: {len(agents)} container(s)")
            print("")

    except Exception as e:
        print_error("Failed to list containers", str(e))
        raise typer.Exit(1)


def format_time_ago(timestamp: str) -> str:
    """Format timestamp as relative time."""
    # For now, just return the timestamp
    # TODO: Parse and format as relative time
    return timestamp[:15] if timestamp else "Unknown"


def format_duration(milliseconds: int) -> str:
    """Format duration in human-readable form."""
    seconds = milliseconds // 1000

    if seconds < 60:
        return f"{seconds}s"
    elif seconds < 3600:
        minutes = seconds // 60
        return f"{minutes}m"
    else:
        hours = seconds // 3600
        minutes = (seconds % 3600) // 60
        return f"{hours}h{minutes}m"
