"""Smart HTTP client for AgentScale daemon/server communication."""

from pathlib import Path
from typing import Optional

import httpx

from agentscale.config import get_active_server, get_default_socket_path


def get_client(server_name: Optional[str] = None, timeout: int = 600) -> httpx.Client:
    """Get HTTP client configured for active server.

    Automatically detects Unix socket vs TCP mode and returns
    appropriately configured client.

    Args:
        server_name: Optional server name (uses active if not specified)
        timeout: Request timeout in seconds (default: 600 for long agent executions)

    Returns:
        Configured httpx.Client
    """
    server_config = get_active_server()

    mode = server_config.get("mode", "unix_socket")

    if mode == "unix_socket":
        # Unix socket mode (local development)
        socket_path = server_config.get("socket_path")
        if not socket_path:
            socket_path = str(get_default_socket_path())

        transport = httpx.HTTPTransport(uds=str(socket_path))
        return httpx.Client(
            transport=transport,
            timeout=timeout,
        )

    elif mode == "tcp":
        # TCP mode (remote server)
        url = server_config.get("url")
        if not url:
            raise ValueError("Server URL not configured")

        headers = {}
        auth_key = server_config.get("auth_key")
        if auth_key:
            headers["Authorization"] = f"Bearer {auth_key}"

        return httpx.Client(
            base_url=url,
            headers=headers,
            timeout=timeout,
        )

    else:
        raise ValueError(f"Unknown server mode: {mode}")


def test_connection(client: httpx.Client) -> bool:
    """Test if server is reachable.

    Args:
        client: HTTP client

    Returns:
        True if server responds to health check
    """
    try:
        response = client.get("http://localhost/v1/health")
        return response.status_code == 200
    except Exception:
        return False
