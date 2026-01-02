"""Status command - Show system overview."""

import json
import sys
import subprocess
from pathlib import Path
from typing import Optional
import typer

from agentscale.utils.output import print_error, print_info, print_success


def status() -> None:
    """Show AgentScale system status.

    Displays an overview of the AgentScale system including daemon health,
    VM status (on macOS), deployed agents, and base images.

    Examples:
        orpheus status
    """
    print_info("AgentScale System Status")
    print("")

    # Get socket path
    if sys.platform == "darwin":
        socket_path = Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"
    else:
        socket_path = Path("/var/run/orpheus.sock")

    # Check daemon health
    daemon_status = check_daemon_health(socket_path)

    # Check VM status (macOS only)
    vm_status = check_vm_status() if sys.platform == "darwin" else None

    # Count deployed agents
    agents_count = count_deployed_agents()

    # Count base images
    images_count = count_base_images()

    # Display status
    if daemon_status:
        uptime = format_uptime(daemon_status.get("uptime_seconds", 0))
        print(f"  Daemon:     Running (uptime: {uptime})")
    else:
        print(f"  Daemon:     Not responding")

    if vm_status:
        print(f"  VM:         {vm_status}")

    print(f"  Socket:     {socket_path}")
    print("")

    print(f"  Agents:     {agents_count} deployed")
    print(f"  Images:     {images_count} base image(s)")
    print("")

    # Show recent activity if daemon is healthy
    if daemon_status:
        running = daemon_status.get("running_agents", 0)
        if running > 0:
            print("  Recent Activity:")
            print(f"    {running} agent(s) currently running")
            print("")


def check_daemon_health(socket_path: Path) -> Optional[dict]:
    """Check if daemon is healthy."""
    try:
        import httpx

        if not socket_path.exists():
            return None

        transport = httpx.HTTPTransport(uds=str(socket_path))
        with httpx.Client(transport=transport, timeout=5) as client:
            response = client.get("http://localhost/v1/health")
            return response.json()
    except Exception:
        return None


def check_vm_status() -> str:
    """Check Lima VM status (macOS only)."""
    try:
        result = subprocess.run(
            ["limactl", "list", "--json"],
            capture_output=True,
            text=True,
            check=True
        )

        vms = json.loads(result.stdout)
        for vm in vms:
            if vm["name"] == "orpheus":
                status = vm["status"]
                arch = vm.get("arch", "unknown")
                cpus = vm.get("cpus", 0)
                memory_gb = vm.get("memory", 0) // (1024 * 1024 * 1024)

                if status == "Running":
                    return f"Running (Ubuntu 24.04, {memory_gb}GB RAM, {cpus} CPUs)"
                else:
                    return f"{status}"

        return "Not found"
    except Exception:
        return "Unknown"


def count_deployed_agents() -> int:
    """Count deployed agents."""
    agents_dir = Path.home() / ".orpheus" / "agents"
    if not agents_dir.exists():
        return 0

    count = 0
    for item in agents_dir.iterdir():
        if item.is_dir() and (item / "manifest.json").exists():
            count += 1

    return count


def count_base_images() -> int:
    """Count base images."""
    images_dir = Path.home() / ".orpheus" / "images"
    if not images_dir.exists():
        return 0

    count = 0
    for item in images_dir.iterdir():
        if item.is_dir() and (item / "manifest.json").exists():
            count += 1

    return count


def format_uptime(seconds: int) -> str:
    """Format uptime in human-readable form."""
    if seconds < 60:
        return f"{seconds}s"
    elif seconds < 3600:
        minutes = seconds // 60
        return f"{minutes}m"
    elif seconds < 86400:
        hours = seconds // 3600
        minutes = (seconds % 3600) // 60
        return f"{hours}h {minutes}m"
    else:
        days = seconds // 86400
        hours = (seconds % 86400) // 3600
        return f"{days}d {hours}h"
