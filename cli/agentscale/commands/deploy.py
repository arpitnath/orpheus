"""Deploy command implementation."""

import sys
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_error, print_info, print_success
from agentscale.utils.builder import build_agent_image, DeployError


def deploy(
    agent_path: str = typer.Argument(..., help="Path to agent directory containing agent.yaml"),
    config: Optional[str] = typer.Option(None, "--config", "-c", help="Path to agentscale.yaml to update"),
    force: bool = typer.Option(False, "--force", "-f", help="Overwrite existing deployment"),
    no_cache: bool = typer.Option(False, "--no-cache", help="Don't use pip cache for dependencies"),
) -> None:
    """Deploy an agent by building a complete self-contained image.

    This command:
    1. Reads agent.yaml from the agent directory
    2. Finds the appropriate base image (python-3.10)
    3. Builds a complete agent image with runtime, dependencies, and code
    4. Updates agentscale.yaml with the deployment

    Examples:
        agentscale deploy ./my-agent
        agentscale deploy ./my-agent --force
        agentscale deploy ./my-agent --config ./custom-config.yaml
    """

    # Validate agent path exists
    agent_dir = Path(agent_path)
    if not agent_dir.exists():
        print_error("Agent directory not found", f"Path: {agent_path}")
        raise typer.Exit(1)

    if not agent_dir.is_dir():
        print_error("Agent path must be a directory", f"Path: {agent_path}")
        raise typer.Exit(1)

    # Check agent.yaml exists
    agent_yaml = agent_dir / "agent.yaml"
    if not agent_yaml.exists():
        print_error("agent.yaml not found", f"Expected at: {agent_yaml}")
        raise typer.Exit(1)

    print_info(f"Deploying agent from: {agent_path}")
    print("")

    # Build agent image
    try:
        result = build_agent_image(
            agent_path=str(agent_dir.absolute()),
            force=force,
            no_cache=no_cache,
            config_path=config
        )

        # Success message
        print("")
        print_success("Agent deployed successfully!")
        print("")
        print(f"  Agent:   {result['agent_name']}")
        print(f"  Image:   {result['image_path']}")
        print(f"  Size:    {result['size_mb']}MB")
        print(f"  Runtime: {result['runtime']}")
        print("")
        print("Ready to invoke:")
        print(f"  agentscale invoke {result['agent_name']} '{{\"query\": \"test\"}}'")
        print("")

    except DeployError as e:
        print_error("Deployment failed", str(e))
        raise typer.Exit(1)
    except Exception as e:
        print_error("Unexpected error during deployment", str(e))
        raise typer.Exit(1)
