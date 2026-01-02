"""Inspect command - Show detailed agent information."""

import json
from pathlib import Path
from typing import Optional
import typer
import yaml

from agentscale.utils.output import print_error, print_info, print_json


def inspect(
    agent_name: str = typer.Argument(..., help="Agent name to inspect"),
    format: Optional[str] = typer.Option(None, "--format", help="Output format (json|yaml)"),
) -> None:
    """Show detailed information about a deployed agent.

    Displays configuration, deployment info, paths, and dependencies
    for the specified agent.

    Examples:
        orpheus inspect calculator-agent
        orpheus inspect calculator-agent --format json
        orpheus inspect calculator-agent --format yaml
    """
    # Find agent directory
    agents_dir = Path.home() / ".orpheus" / "agents"
    agent_dir = agents_dir / agent_name

    if not agent_dir.exists():
        print_error(
            f"Agent '{agent_name}' not found",
            f"List deployed agents with: orpheus list"
        )
        raise typer.Exit(1)

    # Read manifest
    manifest_file = agent_dir / "manifest.json"
    if not manifest_file.exists():
        print_error(
            "Invalid agent directory",
            "Missing manifest.json - agent may be corrupted"
        )
        raise typer.Exit(1)

    try:
        manifest = json.loads(manifest_file.read_text())
    except Exception as e:
        print_error("Failed to read manifest", str(e))
        raise typer.Exit(1)

    # Read agent.yaml
    agent_yaml_file = agent_dir / "agent.yaml"
    agent_config = None
    if agent_yaml_file.exists():
        try:
            agent_config = yaml.safe_load(agent_yaml_file.read_text())
        except Exception:
            pass

    # Output format
    if format == "json":
        print(json.dumps(manifest, indent=2))
        return
    elif format == "yaml":
        print(yaml.dump(manifest, default_flow_style=False))
        return

    # Human-readable format
    print("")
    print(f"Agent: {manifest['agent']['name']}")
    print("")

    print("Configuration:")
    print(f"  Module:      {manifest['agent']['module']}")
    print(f"  Entrypoint:  {manifest['agent']['entrypoint']}")
    print(f"  Runtime:     {manifest['agent']['runtime']}-{manifest['agent']['runtime_version']}")
    print(f"  Memory:      {manifest['agent'].get('memory', 256)}MB")
    print(f"  Timeout:     {manifest['agent'].get('timeout', 300)}s")
    print("")

    print("Deployment:")
    print(f"  Image:       {manifest['image']['path']}")
    print(f"  Size:        {format_size(manifest['image']['size_mb'])}")
    print(f"  Created:     {manifest['build']['created']}")
    print(f"  Base Image:  {Path(manifest['build']['base_image']).name}")
    print("")

    print("Paths:")
    print(f"  Python:      {manifest['paths']['python_binary']}")
    print(f"  Entrypoint:  {manifest['paths']['entrypoint']}")
    print(f"  Packages:    {manifest['paths']['packages_dir']}")
    print(f"  Agent Code:  {manifest['paths']['agent_code']}")
    print("")

    # Show dependencies if requirements.txt exists
    req_file = agent_dir / "agent" / "requirements.txt"
    if req_file.exists():
        print("Dependencies:")
        try:
            reqs = req_file.read_text().strip().split("\n")
            reqs = [r.strip() for r in reqs if r.strip() and not r.startswith("#")]

            if reqs:
                # Show first 10 dependencies
                for req in reqs[:10]:
                    # Parse package name and version
                    if "==" in req:
                        pkg, ver = req.split("==", 1)
                        print(f"  {pkg:<20} {ver}")
                    else:
                        print(f"  {req}")

                if len(reqs) > 10:
                    print(f"  ... and {len(reqs) - 10} more")
            else:
                print("  (none)")
        except Exception:
            print("  (error reading requirements.txt)")

        print("")


def format_size(size_mb: int) -> str:
    """Format size in human-readable form."""
    if size_mb < 1024:
        return f"{size_mb}MB"
    else:
        size_gb = size_mb / 1024
        return f"{size_gb:.1f}GB"
