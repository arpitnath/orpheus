"""Deploy command implementation."""

import sys
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_error, print_info, print_success
from agentscale.utils.builder import build_agent_image, DeployError
from agentscale.utils.archive import create_tar, calculate_checksum, get_tar_size_mb
from agentscale.utils.client import get_client
from agentscale.config import get_active_server


def deploy(
    agent_path: str = typer.Argument(..., help="Path to agent directory containing agent.yaml"),
    config: Optional[str] = typer.Option(None, "--config", "-c", help="Path to agentscale.yaml to update"),
    force: bool = typer.Option(False, "--force", "-f", help="Overwrite existing deployment"),
    no_cache: bool = typer.Option(False, "--no-cache", help="Don't use pip cache for dependencies"),
    remote: bool = typer.Option(False, "--remote", help="Deploy to remote server"),
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

        if remote:
            # Remote deployment
            deploy_remote(result)
        else:
            # Local deployment (existing behavior)
            print("")
            print_success("Agent deployed locally!")
            print("")
            print(f"  Agent:   {result['agent_name']}")
            print(f"  Image:   {result['image_path']}")
            print(f"  Size:    {result['size_mb']}MB")
            print(f"  Runtime: {result['runtime']}")
            print("")
            print("Ready to run:")
            print(f"  echo '{{\"query\": \"test\"}}' | agentscale run {result['agent_name']}")
            print("")

    except DeployError as e:
        print_error("Deployment failed", str(e))
        raise typer.Exit(1)
    except Exception as e:
        print_error("Unexpected error during deployment", str(e))
        raise typer.Exit(1)


def deploy_remote(build_result: dict) -> None:
    """Deploy agent to remote server.

    Args:
        build_result: Result from build_agent_image()
    """
    import httpx

    print_info("Deploying to remote server...")
    print("")

    # Get active server
    server_config = get_active_server()
    if server_config.get("mode") != "tcp":
        print_error(
            "Active server is not a remote server",
            "Use 'agentscale login' to configure a remote server"
        )
        raise typer.Exit(1)

    # Create tar of agent image
    print_info("Creating archive...")
    image_path = Path(build_result['image_path'])
    tar_file = create_tar(image_path)
    tar_size_mb = get_tar_size_mb(tar_file)
    print_info(f"Archive created: {tar_file.name} ({tar_size_mb} MB)")

    # Calculate checksum
    print_info("Calculating checksum...")
    checksum = calculate_checksum(tar_file)
    print_info(f"Checksum: {checksum[:16]}...")

    # Upload to server
    print_info("Uploading to server...")
    try:
        client = get_client(timeout=1800)  # 30 min timeout for large uploads

        with open(tar_file, 'rb') as f:
            files = {'agent_tar': (tar_file.name, f, 'application/gzip')}
            data = {
                'agent_name': build_result['agent_name'],
                'checksum': checksum
            }

            response = client.post('/v1/deploy', files=files, data=data)

            # Check status code first (before printing success)
            if response.status_code != 200:
                error_response = response.json()
                error_msg = error_response.get('error', f"HTTP {response.status_code}")
                print_error("Deployment failed", error_msg)
                raise typer.Exit(1)

            # Parse success response
            deploy_response = response.json()

            # Validate response structure
            if 'agent_name' not in deploy_response:
                print_error("Invalid response from server", f"Response: {deploy_response}")
                raise typer.Exit(1)

            # Success - now safe to print
            print("")
            print_success("Agent deployed to remote server!")
            print("")
            print(f"  Agent:   {deploy_response.get('agent_name')}")
            print(f"  Status:  {deploy_response.get('status')}")
            print(f"  Size:    {deploy_response.get('size_mb')} MB")
            print("")

            if 'endpoints' in deploy_response and deploy_response['endpoints']:
                print("Endpoints:")
                for name, url in deploy_response['endpoints'].items():
                    print(f"  {name}: {url}")
                print("")

    except httpx.HTTPStatusError as e:
        error_detail = e.response.json().get('error', str(e))
        print_error("Deployment failed", error_detail)
        raise typer.Exit(1)
    except httpx.ConnectError:
        server_url = server_config.get("url", "unknown")
        print_error(
            f"Cannot connect to server: {server_url}",
            "Check that server is running and URL is correct"
        )
        raise typer.Exit(1)
    except Exception as e:
        print_error("Upload failed", str(e))
        raise typer.Exit(1)
    finally:
        # Cleanup temp tar file
        if tar_file.exists():
            tar_file.unlink()
