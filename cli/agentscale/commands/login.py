"""Login command - Authenticate to remote AgentScale server."""

import typer
import httpx

from agentscale.config import add_server, set_active_server
from agentscale.utils.output import print_error, print_success, print_info


def login(
    server_url: str = typer.Option(None, "--server", "-s", help="Server URL (e.g., https://agents.mycompany.com)", prompt="Server URL"),
    auth_key: str = typer.Option(None, "--key", "-k", help="API key", prompt="API Key", hide_input=True),
    server_name: str = typer.Option("production", "--name", help="Server name for this configuration"),
) -> None:
    """Authenticate to a remote AgentScale server.

    Saves server configuration to ~/.agentscale/config.yaml and tests connection.

    Examples:
        agentscale login
        agentscale login --server https://agents.company.com --key agsk_abc123
        agentscale login --name staging --server https://staging.company.com
    """
    print_info(f"Authenticating to {server_url}...")

    # Test connection
    try:
        client = httpx.Client(
            base_url=server_url,
            headers={"Authorization": f"Bearer {auth_key}"} if auth_key else {},
            timeout=10
        )

        response = client.get("http://localhost/v1/health")

        if response.status_code != 200:
            print_error(
                "Connection test failed",
                f"Server returned status {response.status_code}"
            )
            raise typer.Exit(1)

        # Check response
        health = response.json()
        version = health.get("version", "unknown")

        print_success(f"✓ Connected to AgentScale v{version}")

    except httpx.ConnectError:
        print_error(
            "Cannot connect to server",
            f"Check that server is running at {server_url}"
        )
        raise typer.Exit(1)
    except Exception as e:
        print_error("Connection failed", str(e))
        raise typer.Exit(1)

    # Save configuration
    try:
        add_server(server_name, server_url, auth_key)
        set_active_server(server_name)

        print_success(f"✓ Server '{server_name}' saved to configuration")
        print_success(f"✓ Active server set to '{server_name}'")
        print("")
        print("You can now use:")
        print(f"  agentscale run ./my-agent")
        print(f"  agentscale deploy ./my-agent --remote")
        print("")

    except Exception as e:
        print_error("Failed to save configuration", str(e))
        raise typer.Exit(1)
