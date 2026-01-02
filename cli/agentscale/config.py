"""Configuration management for AgentScale CLI."""

import os
from pathlib import Path
from typing import Optional, Dict, Any

import yaml


CONFIG_FILE = Path.home() / ".orpheus" / "config.yaml"


def load_config() -> Dict[str, Any]:
    """Load CLI configuration from file.

    Returns:
        Configuration dict with servers and active server.
        Returns default config if file doesn't exist.
    """
    if not CONFIG_FILE.exists():
        return get_default_config()

    try:
        with open(CONFIG_FILE) as f:
            config = yaml.safe_load(f)
            return config or get_default_config()
    except Exception:
        return get_default_config()


def save_config(config: Dict[str, Any]) -> None:
    """Save CLI configuration to file.

    Args:
        config: Configuration dict to save
    """
    # Create directory if it doesn't exist
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)

    with open(CONFIG_FILE, "w") as f:
        yaml.dump(config, f, default_flow_style=False, sort_keys=False)


def get_default_config() -> Dict[str, Any]:
    """Get default CLI configuration.

    Returns:
        Default config with local Unix socket server
    """
    socket_path = get_default_socket_path()

    return {
        "active": "local",
        "servers": {
            "local": {
                "mode": "unix_socket",
                "socket_path": str(socket_path),
            }
        },
    }


def get_default_socket_path() -> Path:
    """Get default Unix socket path based on OS.

    Returns:
        Path to daemon Unix socket
    """
    import sys

    if sys.platform == "darwin":
        # macOS: Lima-forwarded socket
        return Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"

    # Linux: Local socket
    return Path("/var/run/orpheus.sock")


def get_active_server() -> Dict[str, Any]:
    """Get active server configuration.

    Returns:
        Server config dict
    """
    config = load_config()
    active = config.get("active", "local")

    if active not in config.get("servers", {}):
        # Active server doesn't exist, fall back to local
        return config["servers"]["local"]

    return config["servers"][active]


def add_server(name: str, url: str, auth_key: Optional[str] = None) -> None:
    """Add a server to configuration.

    Args:
        name: Server name (e.g., "production", "staging")
        url: Server URL (e.g., "https://agents.mycompany.com")
        auth_key: Optional API key
    """
    config = load_config()

    if "servers" not in config:
        config["servers"] = {}

    config["servers"][name] = {
        "mode": "tcp",
        "url": url,
    }

    if auth_key:
        config["servers"][name]["auth_key"] = auth_key

    save_config(config)


def set_active_server(name: str) -> None:
    """Set the active server.

    Args:
        name: Server name to activate
    """
    config = load_config()

    if name not in config.get("servers", {}):
        raise ValueError(f"Server '{name}' not found in configuration")

    config["active"] = name
    save_config(config)


def remove_server(name: str) -> None:
    """Remove a server from configuration.

    Args:
        name: Server name to remove
    """
    config = load_config()

    if name in config.get("servers", {}):
        del config["servers"][name]

        # If removing active server, switch to local
        if config.get("active") == name:
            config["active"] = "local"

        save_config(config)


def list_servers() -> Dict[str, Dict[str, Any]]:
    """List all configured servers.

    Returns:
        Dict mapping server name to server config
    """
    config = load_config()
    return config.get("servers", {})
