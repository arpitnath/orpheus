"""List command - List deployed agents and base images."""

import json
from datetime import datetime
from pathlib import Path
from typing import List, Dict
import typer

from agentscale.utils.output import print_error, print_info


def list_agents(
    images: bool = typer.Option(False, "--images", help="Show base images instead of agents"),
) -> None:
    """List deployed agents or base images.

    Shows all deployed agents with their runtime, size, and deployment time.
    Use --images to show base images instead.

    Examples:
        agentscale list            # List deployed agents
        agentscale list --images   # List base images
    """
    if images:
        show_base_images()
    else:
        show_deployed_agents()


def show_deployed_agents() -> None:
    """Display deployed agents in a table."""
    agents_dir = Path.home() / ".agentscale" / "agents"

    if not agents_dir.exists():
        print_info("No agents deployed yet")
        print("")
        print("Deploy an agent with:")
        print("  agentscale deploy <agent-path>")
        return

    agents = []
    for agent_dir in agents_dir.iterdir():
        if agent_dir.is_dir() and (agent_dir / "manifest.json").exists():
            try:
                manifest = json.loads((agent_dir / "manifest.json").read_text())
                agents.append({
                    "name": manifest["agent"]["name"],
                    "runtime": f"python-{manifest['agent']['runtime_version']}",
                    "size_mb": manifest["image"]["size_mb"],
                    "created": manifest["build"]["created"],
                })
            except Exception:
                # Skip invalid manifests
                continue

    if not agents:
        print_info("No agents deployed yet")
        return

    # Sort by name
    agents.sort(key=lambda x: x["name"])

    # Print header
    print("")
    print(f"{'NAME':<20} {'RUNTIME':<15} {'SIZE':<10} {'DEPLOYED':<20}")
    print("-" * 70)

    # Print agents
    for agent in agents:
        name = agent["name"][:19]
        runtime = agent["runtime"][:14]
        size = format_size(agent["size_mb"])
        deployed = format_time_ago(agent["created"])

        print(f"{name:<20} {runtime:<15} {size:<10} {deployed:<20}")

    print("")
    print(f"Total: {len(agents)} agent(s)")
    print("")


def show_base_images() -> None:
    """Display base images in a table."""
    images_dir = Path.home() / ".agentscale" / "images"

    if not images_dir.exists():
        print_info("No base images found")
        return

    images = []
    for image_dir in images_dir.iterdir():
        if image_dir.is_dir() and (image_dir / "manifest.json").exists():
            try:
                manifest = json.loads((image_dir / "manifest.json").read_text())

                # Calculate size
                size_mb = manifest.get("size", {}).get("megabytes", 0)
                if size_mb == 0:
                    # Calculate from filesystem
                    size_mb = calculate_directory_size(image_dir)

                images.append({
                    "name": manifest["name"],
                    "version": manifest["runtime"]["version"],
                    "size_mb": size_mb,
                    "created": manifest.get("created", "Unknown"),
                })
            except Exception:
                # Skip invalid manifests
                continue

    if not images:
        print_info("No base images found")
        return

    # Sort by name
    images.sort(key=lambda x: x["name"])

    # Print header
    print("")
    print(f"{'NAME':<15} {'VERSION':<10} {'SIZE':<10} {'CREATED':<20}")
    print("-" * 60)

    # Print images
    for image in images:
        name = image["name"][:14]
        version = str(image["version"])[:9]
        size = format_size(image["size_mb"])
        created = format_time_ago(image["created"])

        print(f"{name:<15} {version:<10} {size:<10} {created:<20}")

    print("")
    print(f"Total: {len(images)} image(s)")
    print("")


def format_size(size_mb: int) -> str:
    """Format size in human-readable form."""
    if size_mb < 1024:
        return f"{size_mb}MB"
    else:
        size_gb = size_mb / 1024
        return f"{size_gb:.1f}GB"


def format_time_ago(iso_timestamp: str) -> str:
    """Format ISO timestamp as relative time."""
    try:
        # Parse ISO timestamp
        created = datetime.fromisoformat(iso_timestamp.replace("Z", "+00:00"))
        now = datetime.now(created.tzinfo)
        diff = now - created

        seconds = diff.total_seconds()

        if seconds < 60:
            return "just now"
        elif seconds < 3600:
            minutes = int(seconds // 60)
            return f"{minutes}m ago"
        elif seconds < 86400:
            hours = int(seconds // 3600)
            return f"{hours}h ago"
        elif seconds < 604800:  # 7 days
            days = int(seconds // 86400)
            return f"{days}d ago"
        else:
            return created.strftime("%Y-%m-%d")

    except Exception:
        return "Unknown"


def calculate_directory_size(directory: Path) -> int:
    """Calculate directory size in MB."""
    total_size = 0
    for item in directory.rglob("*"):
        if item.is_file():
            total_size += item.stat().st_size

    return total_size // (1024 * 1024)
