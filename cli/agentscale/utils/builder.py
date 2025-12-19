"""Agent image builder utilities."""

import json
import os
import platform
import shutil
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, Optional

import yaml


# ============================================================================
# Exceptions
# ============================================================================

class DeployError(Exception):
    """Base exception for deployment errors."""
    pass


class ConfigError(DeployError):
    """Agent configuration error."""
    pass


class ImageNotFoundError(DeployError):
    """Base image not found."""
    pass


class BuildError(DeployError):
    """Error during image build."""
    pass


# ============================================================================
# Agent Configuration
# ============================================================================

def read_agent_config(agent_path: str) -> Dict[str, Any]:
    """Read and parse agent.yaml configuration.

    Args:
        agent_path: Path to agent directory

    Returns:
        Dictionary with agent configuration

    Raises:
        ConfigError: If agent.yaml is missing or invalid
    """
    agent_yaml_path = Path(agent_path) / "agent.yaml"

    if not agent_yaml_path.exists():
        raise ConfigError(f"agent.yaml not found at {agent_yaml_path}")

    try:
        with open(agent_yaml_path) as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        raise ConfigError(f"Invalid YAML in agent.yaml: {e}")

    if not data:
        raise ConfigError("agent.yaml is empty")

    # Validate required fields
    required_fields = ["name", "runtime", "module", "entrypoint"]
    missing = [field for field in required_fields if field not in data]
    if missing:
        raise ConfigError(f"Missing required fields in agent.yaml: {', '.join(missing)}")

    # Validate runtime
    if data["runtime"] != "python3":
        raise ConfigError(
            f"Unsupported runtime: {data['runtime']}\n"
            f"Only 'python3' is supported in v0.1.0"
        )

    # Add runtime_version if not specified (default to 3.10)
    if "runtime_version" not in data:
        data["runtime_version"] = "3.10"

    # Add agent_path for reference
    data["agent_path"] = str(Path(agent_path).absolute())

    return data


# ============================================================================
# Base Image Resolution
# ============================================================================

def find_base_image(runtime: str, version: str) -> Path:
    """Find appropriate base image based on runtime.

    Args:
        runtime: Runtime type (python3, node, etc.)
        version: Runtime version (3.10, 3.11, etc.)

    Returns:
        Path to base image

    Raises:
        ImageNotFoundError: If base image not found
    """
    if runtime != "python3":
        raise ImageNotFoundError(
            f"Unsupported runtime: {runtime}\n"
            f"Only python3 is supported in v0.1.0"
        )

    # Construct image name
    image_name = f"python-{version}"
    images_dir = Path.home() / ".agentscale" / "images"

    # Platform-specific base image location
    if sys.platform.startswith("linux"):
        # Linux: Directory-based rootfs
        base_path = images_dir / image_name

        if not base_path.exists():
            raise ImageNotFoundError(
                f"Base image not found: {base_path}\n\n"
                f"Please build it first:\n"
                f"  cd agentscale/isolate/scripts\n"
                f"  ./build-python-{version}-image.sh\n"
            )

        return base_path

    elif sys.platform == "darwin":
        # macOS: Compressed initrd file
        base_path = images_dir / f"{image_name}.initrd.gz"

        if not base_path.exists():
            raise ImageNotFoundError(
                f"Base image not found: {base_path}\n\n"
                f"Please build it first:\n"
                f"  cd agentscale/isolate/scripts\n"
                f"  ./build-python-{version}-initrd.sh\n"
            )

        return base_path

    else:
        raise ImageNotFoundError(
            f"Unsupported platform: {sys.platform}\n"
            f"Only Linux and macOS (darwin) are supported"
        )


# ============================================================================
# Agent Image Directory
# ============================================================================

def create_agent_image_dir(agent_name: str, force: bool = False) -> Path:
    """Create directory structure for agent image.

    Args:
        agent_name: Name of the agent
        force: If True, overwrite existing deployment

    Returns:
        Path to agent image directory

    Raises:
        BuildError: If agent already deployed and force=False
    """
    agents_dir = Path.home() / ".agentscale" / "agents"
    agent_dir = agents_dir / agent_name

    # Check if already deployed
    if agent_dir.exists():
        if not force:
            raise BuildError(
                f"Agent '{agent_name}' already deployed at {agent_dir}\n"
                f"Use --force to overwrite"
            )

        # Remove existing
        print(f"[deploy] Removing existing deployment at {agent_dir}")
        shutil.rmtree(agent_dir)

    # Create structure
    agent_dir.mkdir(parents=True, exist_ok=True)
    (agent_dir / "runtime").mkdir(exist_ok=True)
    (agent_dir / "packages").mkdir(exist_ok=True)
    (agent_dir / "agent").mkdir(exist_ok=True)

    return agent_dir


def calculate_size(directory: Path) -> tuple[int, int]:
    """Calculate directory size in bytes and megabytes.

    Args:
        directory: Path to directory

    Returns:
        Tuple of (bytes, megabytes)
    """
    total_size = 0
    for dirpath, dirnames, filenames in os.walk(directory):
        for filename in filenames:
            filepath = Path(dirpath) / filename
            if filepath.exists():
                total_size += filepath.stat().st_size

    size_mb = total_size // (1024 * 1024)
    return total_size, size_mb


# ============================================================================
# Main Build Function (Placeholder)
# ============================================================================

def build_agent_image(
    agent_path: str,
    force: bool = False,
    no_cache: bool = False,
    config_path: Optional[str] = None
) -> Dict[str, Any]:
    """Build complete agent image with runtime, dependencies, and code.

    Args:
        agent_path: Path to agent directory
        force: Overwrite existing deployment
        no_cache: Don't use pip cache
        config_path: Path to agentscale.yaml

    Returns:
        Dictionary with deployment result info

    Raises:
        DeployError: If build fails
    """
    # Step 1: Read agent configuration
    print("[deploy] Reading agent configuration...")
    agent_config = read_agent_config(agent_path)

    print(f"[deploy] Agent: {agent_config['name']}")
    print(f"[deploy] Runtime: {agent_config['runtime']} {agent_config['runtime_version']}")
    print("")

    # Step 2: Find base image
    print("[deploy] Finding base image...")
    base_image_path = find_base_image(agent_config["runtime"], agent_config["runtime_version"])
    print(f"[deploy] ✓ Base image: {base_image_path}")
    print("")

    # Step 3: Create agent image directory
    print("[deploy] Creating agent image structure...")
    agent_image_dir = create_agent_image_dir(agent_config["name"], force=force)
    print(f"[deploy] ✓ Created: {agent_image_dir}")
    print("")

    # Remaining steps will be implemented in subsequent tasks
    # For now, return basic result

    size_bytes, size_mb = calculate_size(agent_image_dir)

    # Step 4: Copy runtime from base image
    print("[deploy] Step 1/4: Copying runtime...")
    runtime_dir = agent_image_dir / "runtime"
    copy_runtime(base_image_path, runtime_dir)
    runtime_size_bytes, runtime_size_mb = calculate_size(runtime_dir)
    print(f"[deploy] ✓ Runtime copied ({runtime_size_mb}MB)")
    print("")

    # Step 5: Install dependencies
    print("[deploy] Step 2/4: Installing dependencies...")
    packages_dir = agent_image_dir / "packages"
    install_dependencies(agent_path, packages_dir, no_cache)
    deps_size_bytes, deps_size_mb = calculate_size(packages_dir)
    if deps_size_mb > 0:
        print(f"[deploy] ✓ Dependencies installed ({deps_size_mb}MB)")
    print("")

    # Step 6: Copy agent code
    print("[deploy] Step 3/4: Copying agent code...")
    agent_code_dir = agent_image_dir / "agent"
    copy_agent_code(agent_path, agent_code_dir)
    code_size_bytes, code_size_mb = calculate_size(agent_code_dir)
    print(f"[deploy] ✓ Agent code copied ({code_size_mb}MB)")
    print("")

    # Step 7: Create manifest
    print("[deploy] Step 4/4: Creating manifest...")
    create_manifest(agent_config, agent_image_dir, base_image_path)
    print(f"[deploy] ✓ Manifest created")
    print("")

    # Step 8: Update agentscale.yaml
    if not config_path and not os.path.exists("agentscale.yaml"):
        config_path = None  # Will handle in config_updater

    if config_path is not False:  # Allow skipping with --skip-config
        print("[deploy] Updating agentscale.yaml...")
        from agentscale.utils.config_updater import update_agentscale_yaml
        update_agentscale_yaml(agent_config, agent_image_dir, config_path)
        print(f"[deploy] ✓ Configuration updated")

    size_bytes, size_mb = calculate_size(agent_image_dir)

    return {
        "agent_name": agent_config["name"],
        "image_path": str(agent_image_dir),
        "size_mb": size_mb,
        "runtime": f"{agent_config['runtime']}-{agent_config['runtime_version']}"
    }


# ============================================================================
# Runtime Copy
# ============================================================================

def copy_runtime(base_image_path: Path, runtime_dir: Path) -> None:
    """Copy base image to agent runtime directory.

    Args:
        base_image_path: Path to base image
        runtime_dir: Target runtime directory

    Raises:
        BuildError: If copy fails
    """
    if sys.platform.startswith("linux"):
        copy_runtime_linux(base_image_path, runtime_dir)
    elif sys.platform == "darwin":
        copy_runtime_macos(base_image_path, runtime_dir)
    else:
        raise BuildError(f"Unsupported platform: {sys.platform}")


def copy_runtime_linux(base_path: Path, runtime_dir: Path) -> None:
    """Copy Linux base image (directory) to runtime directory.

    Args:
        base_path: Path to base image directory
        runtime_dir: Target directory
    """
    try:
        # Copy entire directory tree
        shutil.copytree(
            base_path,
            runtime_dir,
            symlinks=True,  # Preserve symlinks
            ignore_dangling_symlinks=True,
            dirs_exist_ok=True
        )
    except Exception as e:
        raise BuildError(f"Failed to copy Linux runtime: {e}")


def copy_runtime_macos(base_initrd: Path, runtime_dir: Path) -> None:
    """Extract macOS base image (initrd) to runtime directory.

    Args:
        base_initrd: Path to compressed initrd file
        runtime_dir: Target directory
    """
    try:
        # Extract initrd: gunzip -c base.initrd.gz | cpio -idm -D runtime_dir
        runtime_dir.mkdir(parents=True, exist_ok=True)

        # Run gunzip and pipe to cpio
        with subprocess.Popen(
            ["gunzip", "-c", str(base_initrd)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        ) as gunzip_proc:

            cpio_result = subprocess.run(
                ["cpio", "-idm"],
                stdin=gunzip_proc.stdout,
                cwd=runtime_dir,
                capture_output=True,
                text=True
            )

            if cpio_result.returncode != 0:
                raise BuildError(f"cpio extraction failed: {cpio_result.stderr}")

    except subprocess.CalledProcessError as e:
        raise BuildError(f"Failed to extract macOS runtime: {e}")
    except Exception as e:
        raise BuildError(f"Failed to copy macOS runtime: {e}")


# ============================================================================
# Dependency Installation
# ============================================================================

def install_dependencies(agent_path: str, packages_dir: Path, no_cache: bool = False) -> None:
    """Install Python dependencies from requirements.txt.

    Args:
        agent_path: Path to agent directory
        packages_dir: Target directory for packages
        no_cache: If True, don't use pip cache
    """
    requirements_file = Path(agent_path) / "requirements.txt"

    # Handle missing requirements.txt
    if not requirements_file.exists():
        print("[deploy] No requirements.txt found, skipping dependencies")
        return

    # Read and count packages
    with open(requirements_file) as f:
        lines = [line.strip() for line in f if line.strip() and not line.startswith("#")]

    if not lines:
        print("[deploy] requirements.txt is empty, skipping dependencies")
        return

    print(f"[deploy] Installing {len(lines)} package(s)...")

    # Ensure packages directory exists
    packages_dir.mkdir(parents=True, exist_ok=True)

    # Build pip command
    cmd = [
        sys.executable, "-m", "pip", "install",
        "--target", str(packages_dir),
        "-r", str(requirements_file),
        "--quiet"
    ]

    if no_cache:
        cmd.append("--no-cache-dir")

    # Run pip install
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            check=True
        )
    except subprocess.CalledProcessError as e:
        # Extract package name from error if possible
        error_msg = e.stderr

        raise BuildError(
            f"Failed to install dependencies:\n\n{error_msg}\n\n"
            f"Check requirements.txt syntax and package availability on PyPI"
        )


# ============================================================================
# Agent Code Copy
# ============================================================================

def copy_agent_code(agent_path: str, agent_code_dir: Path) -> None:
    """Copy entire agent directory to image.

    Args:
        agent_path: Source agent directory
        agent_code_dir: Target directory in image
    """
    # Define ignore patterns
    def ignore_patterns(directory, files):
        ignored = []
        for f in files:
            if f in ("__pycache__", ".git", ".gitignore", "venv", ".venv", "node_modules"):
                ignored.append(f)
            elif f.endswith((".pyc", ".pyo", ".egg-info")):
                ignored.append(f)
        return ignored

    try:
        shutil.copytree(
            agent_path,
            agent_code_dir,
            ignore=ignore_patterns,
            symlinks=True,
            dirs_exist_ok=True
        )
    except Exception as e:
        raise BuildError(f"Failed to copy agent code: {e}")


# ============================================================================
# Manifest Creation
# ============================================================================

def create_manifest(agent_config: Dict[str, Any], agent_image_dir: Path, base_image_path: Path) -> None:
    """Create manifest.json with deployment metadata.

    Args:
        agent_config: Agent configuration dict
        agent_image_dir: Path to agent image directory
        base_image_path: Path to base image used
    """
    size_bytes, size_mb = calculate_size(agent_image_dir)

    manifest = {
        "version": "1.0.0",
        "spec_version": "v1",
        "agent": {
            "name": agent_config["name"],
            "runtime": agent_config["runtime"],
            "runtime_version": agent_config["runtime_version"],
            "module": agent_config["module"],
            "entrypoint": agent_config["entrypoint"],
            "memory": agent_config.get("memory", 512),
            "timeout": agent_config.get("timeout", 300)
        },
        "build": {
            "base_image": str(base_image_path),
            "created": datetime.utcnow().isoformat() + "Z",
            "platform": {
                "os": sys.platform,
                "arch": platform.machine()
            }
        },
        "image": {
            "path": str(agent_image_dir),
            "size_bytes": size_bytes,
            "size_mb": size_mb,
            "structure": {
                "runtime": str(agent_image_dir / "runtime"),
                "packages": str(agent_image_dir / "packages"),
                "agent": str(agent_image_dir / "agent")
            }
        },
        "paths": {
            "python_binary": "/runtime/usr/local/bin/python3",
            "packages_dir": "/packages",
            "agent_code": "/agent",
            "entrypoint": f"/agent/{agent_config['module']}.py"
        },
        "environment": {
            "PYTHONPATH": "/packages:/agent",
            "PYTHONUNBUFFERED": "1",
            "PYTHONDONTWRITEBYTECODE": "1"
        }
    }

    manifest_file = agent_image_dir / "manifest.json"
    with open(manifest_file, "w") as f:
        json.dump(manifest, f, indent=2)

