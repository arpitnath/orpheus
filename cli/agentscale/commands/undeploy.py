"""Undeploy command - Remove deployed agent."""

import json
import shutil
from pathlib import Path
import typer
import yaml
import httpx

from agentscale.utils.output import print_error, print_info, print_success, print_warning
from agentscale.config import get_active_server, load_config
from agentscale.utils.client import get_client


def undeploy(
    agent_name: str = typer.Argument(..., help="Agent name to remove"),
    force: bool = typer.Option(False, "--force", "-f", help="Skip confirmation prompt"),
) -> None:
    """Remove a deployed agent.

    Removes agent from active server (local or remote based on config).
    Also deletes local state and updates configuration.

    Examples:
        agentscale undeploy calculator-agent
        agentscale undeploy calculator-agent --force
    """
    # Detect server mode
    server_config = get_active_server()
    mode = server_config.get("mode", "unix_socket")
    is_remote = (mode == "tcp")

    # Get server name for messaging
    config = load_config()
    active_server_name = config.get("active", "local")

    # Find agent directory
    agents_dir = Path.home() / ".agentscale" / "agents"
    agent_dir = agents_dir / agent_name

    # Get agent size if local state exists
    size_mb = 0
    if agent_dir.exists():
        try:
            manifest_file = agent_dir / "manifest.json"
            if manifest_file.exists():
                manifest = json.loads(manifest_file.read_text())
                size_mb = manifest["image"]["size_mb"]
            else:
                size_mb = calculate_directory_size(agent_dir)
        except Exception:
            size_mb = 0

    size_display = format_size(size_mb) if size_mb > 0 else "unknown"

    # Confirm deletion
    if not force:
        location = f"server '{active_server_name}'" if is_remote else "locally"
        message = f"This will undeploy agent '{agent_name}' from {location}"
        if size_mb > 0:
            message += f" ({size_display})"

        print_warning(message, "This action cannot be undone")
        print("")

        confirm = typer.confirm("Continue?", default=False)
        if not confirm:
            print_info("Cancelled")
            raise typer.Exit(0)

    # Message
    if is_remote:
        print_info(f"Undeploying agent '{agent_name}' from {active_server_name}...")
    else:
        print_info(f"Removing agent '{agent_name}'...")

    print("")

    # Remote cleanup first (if applicable)
    remote_success = True
    if is_remote:
        remote_success = undeploy_from_remote(agent_name)
        if not remote_success and not force:
            raise typer.Exit(1)

    # Local cleanup (always do this - handles orphaned local state)
    if agent_dir.exists():
        try:
            shutil.rmtree(agent_dir)
            print_success("✓ Agent directory removed")
        except Exception as e:
            print_error("Failed to remove agent directory", str(e))
            raise typer.Exit(1)
    else:
        if not is_remote:
            # Local mode and no local state - this is an error
            print_error(
                f"Agent '{agent_name}' not found",
                "List deployed agents with: agentscale list"
            )
            raise typer.Exit(1)
        # Remote mode with no local state is OK (was deployed remotely only)

    # Update agentscale.yaml
    try:
        remove_from_config(agent_name)
        print_success("✓ Configuration updated")
    except Exception as e:
        print_warning("Failed to update agentscale.yaml", str(e))

    print("")
    if size_mb > 0:
        print(f"Freed: {size_display}")

    # Warn if remote cleanup failed
    if is_remote and not remote_success:
        print("")
        print_warning(
            "Remote cleanup incomplete",
            f"Agent may still exist on server '{active_server_name}'"
        )

    print("")


def undeploy_from_remote(agent_name: str) -> bool:
    """Undeploy agent from remote server.

    Args:
        agent_name: Agent to undeploy

    Returns:
        True if successful or agent not found (idempotent), False on error
    """
    try:
        client = get_client(timeout=30)
        response = client.delete(f'/v1/agents/{agent_name}')

        if response.status_code == 200:
            print_success("✓ Removed from server (registry + pool)")
            return True
        elif response.status_code == 404:
            print_warning(
                f"Agent '{agent_name}' not found on server",
                "It may have been already undeployed"
            )
            return True  # Not an error - idempotent
        else:
            error_response = response.json()
            error_msg = error_response.get('error', f"HTTP {response.status_code}")
            print_error(f"Failed to undeploy from server: {error_msg}")
            return False

    except httpx.ConnectError:
        server_config = get_active_server()
        server_url = server_config.get("url", "unknown")
        print_warning(
            f"Cannot connect to server: {server_url}",
            "Local state will still be removed"
        )
        return True  # Continue with local cleanup
    except Exception as e:
        print_error(f"Server request failed: {str(e)}")
        return False


def remove_from_config(agent_name: str) -> None:
    """Remove agent from agentscale.yaml."""
    config_file = Path("agentscale.yaml")

    if not config_file.exists():
        # No config file, nothing to update
        return

    try:
        config = yaml.safe_load(config_file.read_text())

        if "agents" in config and agent_name in config["agents"]:
            del config["agents"][agent_name]

            # Write back
            with open(config_file, "w") as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=False)

    except Exception as e:
        raise Exception(f"Failed to update config: {e}")


def calculate_directory_size(directory: Path) -> int:
    """Calculate directory size in MB."""
    total_size = 0
    for item in directory.rglob("*"):
        if item.is_file():
            total_size += item.stat().st_size

    return total_size // (1024 * 1024)


def format_size(size_mb: int) -> str:
    """Format size in human-readable form."""
    if size_mb < 1024:
        return f"{size_mb}MB"
    else:
        size_gb = size_mb / 1024
        return f"{size_gb:.1f}GB"
