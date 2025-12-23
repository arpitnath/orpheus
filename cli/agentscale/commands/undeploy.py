"""Undeploy command - Remove deployed agent."""

import json
import shutil
from pathlib import Path
import typer
import yaml

from agentscale.utils.output import print_error, print_info, print_success, print_warning


def undeploy(
    agent_name: str = typer.Argument(..., help="Agent name to remove"),
    force: bool = typer.Option(False, "--force", "-f", help="Skip confirmation prompt"),
) -> None:
    """Remove a deployed agent.

    Deletes the agent directory and updates agentscale.yaml configuration.
    Shows disk space freed after removal.

    Examples:
        agentscale undeploy calculator-agent
        agentscale undeploy calculator-agent --force
    """
    # Find agent directory
    agents_dir = Path.home() / ".agentscale" / "agents"
    agent_dir = agents_dir / agent_name

    if not agent_dir.exists():
        print_error(
            f"Agent '{agent_name}' not found",
            "List deployed agents with: agentscale list"
        )
        raise typer.Exit(1)

    # Get agent size
    try:
        manifest_file = agent_dir / "manifest.json"
        if manifest_file.exists():
            manifest = json.loads(manifest_file.read_text())
            size_mb = manifest["image"]["size_mb"]
        else:
            # Calculate size if no manifest
            size_mb = calculate_directory_size(agent_dir)
    except Exception:
        size_mb = 0

    size_display = format_size(size_mb)

    # Confirm deletion
    if not force:
        print_warning(
            f"This will remove agent '{agent_name}' ({size_display})",
            "This action cannot be undone"
        )
        print("")

        confirm = typer.confirm("Continue?", default=False)
        if not confirm:
            print_info("Cancelled")
            raise typer.Exit(0)

    print_info(f"Removing agent '{agent_name}'...")

    # Remove agent directory
    try:
        shutil.rmtree(agent_dir)
        print_success(f"✓ Agent directory removed")
    except Exception as e:
        print_error("Failed to remove agent directory", str(e))
        raise typer.Exit(1)

    # Update agentscale.yaml
    try:
        remove_from_config(agent_name)
        print_success("✓ Configuration updated")
    except Exception as e:
        print_warning("Failed to update agentscale.yaml", str(e))

    print("")
    print(f"Freed: {size_display}")
    print("")


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
