"""Deploy command implementation."""

import os
import sys
from pathlib import Path
from typing import Optional
import re

import typer

from agentscale.utils.output import print_error, print_info, print_success
from agentscale.utils.builder import build_agent_image, DeployError, finalize_agent_image, cleanup_temp_build
from agentscale.utils.archive import create_tar, calculate_checksum, get_tar_size_mb
from agentscale.utils.client import get_client
from agentscale.config import get_active_server


def resolve_env_vars_for_deploy(env_list: list) -> list:
    """Resolve ${VAR} and ${VAR:-default} from shell environment.

    Args:
        env_list: List of env strings like ["KEY=${VAR}", "KEY2=value"]

    Returns:
        List of resolved env strings with actual values

    Raises:
        DeployError: If required variable is not set
    """
    resolved = []

    for env_str in env_list:
        if '=' not in env_str:
            continue

        key, template = env_str.split('=', 1)

        # Check for ${VAR:-default} pattern
        default_match = re.match(r'\$\{([^}:]+):-([^}]*)\}', template)
        if default_match:
            var_name = default_match.group(1)
            default_val = default_match.group(2)
            value = os.environ.get(var_name, default_val)
            resolved.append(f"{key}={value}")
            continue

        # Check for ${VAR} pattern
        var_match = re.match(r'\$\{([^}]+)\}', template)
        if var_match:
            var_name = var_match.group(1)
            value = os.environ.get(var_name)
            if value is None:
                raise DeployError(
                    f"Environment variable {var_name} not set.\n\n"
                    f"  Required by: env.{key}\n"
                    f"  Set it with: export {var_name}=your-value"
                )
            resolved.append(f"{key}={value}")
            continue

        # No variable reference - use as-is
        resolved.append(env_str)

    return resolved


def deploy(
    agent_path: str = typer.Argument(..., help="Path to agent directory containing agent.yaml"),
    config: Optional[str] = typer.Option(None, "--config", "-c", help="Path to orpheus.yaml to update"),
    force: bool = typer.Option(False, "--force", "-f", help="Overwrite existing deployment"),
    no_cache: bool = typer.Option(False, "--no-cache", help="Don't use pip cache for dependencies"),
    remote: bool = typer.Option(False, "--remote", help="Deploy to remote server"),
    verbose: bool = typer.Option(False, "--verbose", "-v", help="Show detailed output"),
    quiet: bool = typer.Option(False, "--quiet", "-q", help="Minimal output"),
    json_output: bool = typer.Option(False, "--json", help="JSON output for scripting"),
) -> None:
    """Deploy an agent by building a complete self-contained image.

    This command:
    1. Reads agent.yaml from the agent directory
    2. Finds the appropriate base image (python-3.10)
    3. Builds a complete agent image with runtime, dependencies, and code
    4. Updates orpheus.yaml with the deployment

    Examples:
        orpheus deploy ./my-agent
        orpheus deploy ./my-agent --force
        orpheus deploy ./my-agent --config ./custom-config.yaml
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

    # CI detection
    import os
    is_ci = os.environ.get('CI') or os.environ.get('GITHUB_ACTIONS')
    if is_ci and not verbose:
        quiet = True

    if not quiet and not json_output:
        print_info(f"Deploying agent from: {agent_path}")
        print("")

    # Build agent image
    build_result = None  # Track for cleanup
    try:
        if remote:
            # Remote mode: Build in temporary location
            if verbose:
                print_info("Building agent image (temporary)...")

            build_result = build_agent_image(
                agent_path=str(agent_dir.absolute()),
                force=force,
                no_cache=no_cache,
                config_path=False,  # Don't update config yet (will do after finalization)
                verbose=verbose,
                quiet=quiet,
                temp=True  # Build in temp location
            )

            # Deploy to remote server
            deploy_remote(build_result, force=force, verbose=verbose, quiet=quiet, json_output=json_output)

            # Remote deployment succeeded - finalize local state
            if verbose:
                print_info("Finalizing local state...")

            final_path = finalize_agent_image(
                build_result['image_path'],
                build_result['agent_name'],
                force=force,
                verbose=verbose
            )

            # Update config to point to final location
            if config is not False:
                from agentscale.utils.config_updater import update_orpheus_yaml
                update_orpheus_yaml(
                    build_result['agent_config'],
                    Path(final_path),
                    config,
                    verbose=verbose
                )

        else:
            # Local mode: Build directly in final location (existing behavior)
            build_result = build_agent_image(
                agent_path=str(agent_dir.absolute()),
                force=force,
                no_cache=no_cache,
                config_path=config,
                verbose=verbose,
                quiet=quiet,
                temp=False  # Build in final location
            )

            # Local deployment output
            print("")
            print_success("Agent deployed locally!")
            print("")
            print(f"  Agent:   {build_result['agent_name']}")
            print(f"  Image:   {build_result['image_path']}")
            print(f"  Size:    {build_result['size_mb']}MB")
            print(f"  Runtime: {build_result['runtime']}")
            print("")
            print("Ready to run:")
            print(f"  echo '{{\"query\": \"test\"}}' | orpheus run {build_result['agent_name']}")
            print("")

    except DeployError as e:
        # Cleanup temporary build on failure
        if build_result and build_result.get('is_temp') and 'image_path' in build_result:
            cleanup_temp_build(build_result['image_path'], verbose=verbose)
        print_error("Deployment failed", str(e))
        raise typer.Exit(1)
    except Exception as e:
        # Cleanup temporary build on unexpected errors
        if build_result and build_result.get('is_temp') and 'image_path' in build_result:
            cleanup_temp_build(build_result['image_path'], verbose=verbose)
        print_error("Unexpected error during deployment", str(e))
        raise typer.Exit(1)


def deploy_remote(build_result: dict, force: bool = False, verbose: bool = False, quiet: bool = False, json_output: bool = False) -> None:
    """Deploy agent to remote server.

    Args:
        build_result: Result from build_agent_image()
        verbose: Show detailed output
        quiet: Minimal output
        json_output: JSON format output
    """
    import httpx

    if verbose:
        print_info("Deploying to remote server...")
        print("")

    # Get active server
    server_config = get_active_server()
    if server_config.get("mode") != "tcp":
        print_error(
            "Active server is not a remote server",
            "Use 'orpheus login' to configure a remote server"
        )
        raise typer.Exit(1)

    # Resolve environment variables from shell before deploying
    resolved_env = []
    if 'env' in build_result.get('agent_config', {}):
        resolved_env = resolve_env_vars_for_deploy(build_result['agent_config']['env'])

    # Create tar of agent image
    if verbose:
        print_info("Creating archive...")
    image_path = Path(build_result['image_path'])
    # Use agent name as arcname (not directory name which might have UUID suffix for temp builds)
    tar_file = create_tar(image_path, arcname=build_result['agent_name'])
    tar_size_mb = get_tar_size_mb(tar_file)
    if verbose:
        print_info(f"Archive created: {tar_file.name} ({tar_size_mb} MB)")

    # Calculate checksum
    if verbose:
        print_info("Calculating checksum...")
    checksum = calculate_checksum(tar_file)
    if verbose:
        print_info(f"Checksum: {checksum[:16]}...")

    # Upload to server
    if verbose:
        print_info("Uploading to server...")
    try:
        client = get_client(timeout=1800)  # 30 min timeout for large uploads

        # Prepare form data with resolved env vars
        import json
        form_data = {
            'agent_name': build_result['agent_name'],
            'checksum': checksum,
            'env': json.dumps(resolved_env),  # Send resolved env vars
        }

        with open(tar_file, 'rb') as f:
            files = {'agent_tar': (tar_file.name, f, 'application/gzip')}

            response = client.post('/v1/deploy', files=files, data=form_data)

            # Check status code first (before printing success)
            if response.status_code != 200:
                error_response = response.json()
                error_msg = error_response.get('error', f"HTTP {response.status_code}")

                # Provide helpful message for conflict errors
                if response.status_code == 409:  # Conflict
                    print_error(
                        "Agent already exists on server",
                        f"{error_msg}\n\nTo replace it, use:\n  orpheus undeploy {build_result['agent_name']}\n  Then retry deployment"
                    )
                else:
                    print_error("Deployment failed", error_msg)
                raise typer.Exit(1)

            # Parse success response
            deploy_response = response.json()

            # Validate response structure
            if 'agent_name' not in deploy_response:
                print_error("Invalid response from server", f"Response: {deploy_response}")
                raise typer.Exit(1)

            # Success - output based on mode
            if json_output:
                # JSON output for scripting
                import json
                output_data = {
                    "success": True,
                    "agent_name": deploy_response.get('agent_name'),
                    "endpoints": deploy_response.get('endpoints', {}),
                    "size_mb": deploy_response.get('size_mb')
                }
                print(json.dumps(output_data))
            elif quiet:
                # Minimal output - just agent name
                print(deploy_response.get('agent_name'))
            else:
                # Default: Clean formatted output
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
