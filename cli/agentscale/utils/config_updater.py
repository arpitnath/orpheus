"""orpheus.yaml configuration updater."""

from pathlib import Path
from typing import Dict, Any, Optional

import yaml


class ConfigUpdateError(Exception):
    """Error updating configuration."""
    pass


def update_orpheus_yaml(
    agent_config: Dict[str, Any],
    agent_image_dir: Path,
    config_path: Optional[str] = None,
    verbose: bool = False
) -> None:
    """Add or update agent entry in orpheus.yaml.

    Args:
        agent_config: Agent configuration dict
        agent_image_dir: Path to deployed agent image
        config_path: Path to orpheus.yaml (optional)

    Raises:
        ConfigUpdateError: If update fails
    """
    # Find config file
    if config_path:
        yaml_path = Path(config_path)
    else:
        # Look in current directory first
        yaml_path = Path("orpheus.yaml")
        if not yaml_path.exists():
            # Look in ~/.orpheus/
            home_config = Path.home() / ".orpheus" / "orpheus.yaml"
            if home_config.exists():
                yaml_path = home_config
            # If neither exists, create in current directory
            # (will be created below)

    # Load existing config or create new
    if yaml_path.exists():
        try:
            with open(yaml_path) as f:
                config = yaml.safe_load(f) or {}
        except yaml.YAMLError as e:
            raise ConfigUpdateError(f"Failed to parse {yaml_path}: {e}")
    else:
        if verbose:
            print(f"[config] Creating new configuration at {yaml_path}")
        config = create_default_config()

    # Ensure agents section exists
    if "agents" not in config:
        config["agents"] = {}

    # Add/update agent entry
    agent_id = agent_config["name"]

    if agent_id in config["agents"]:
        if verbose:
            print(f"[config] Agent '{agent_id}' already exists, updating...")

    config["agents"][agent_id] = {
        "image": str(agent_image_dir),
        "scaling": get_default_scaling_config()
    }

    # Write back to file
    try:
        with open(yaml_path, "w") as f:
            yaml.dump(config, f, default_flow_style=False, sort_keys=False)
    except Exception as e:
        raise ConfigUpdateError(f"Failed to write {yaml_path}: {e}")

    # Validate written config
    try:
        with open(yaml_path) as f:
            yaml.safe_load(f)
    except yaml.YAMLError as e:
        raise ConfigUpdateError(f"Generated invalid YAML: {e}")


def create_default_config() -> Dict[str, Any]:
    """Create default orpheus.yaml structure.

    Returns:
        Default configuration dictionary
    """
    return {
        "server": {
            "port": 8080,
            "autoscaler_interval": "10s",
            "isolation": {
                "enabled": True,
                "type": "auto",
                "defaults": {
                    "memory_limit": "512mb",
                    "timeout": "300s"
                }
            }
        },
        "agents": {}
    }


def get_default_scaling_config() -> Dict[str, Any]:
    """Return sensible default scaling configuration.

    Returns:
        Default scaling policy dictionary
    """
    return {
        "min_workers": 1,
        "max_workers": 5,
        "target_utilization": 2.0,
        "scale_up_threshold": 3.0,
        "scale_down_threshold": 0.5,
        "scale_up_delay": "15s",
        "scale_down_delay": "1m",
        "queue_size": 100
    }
