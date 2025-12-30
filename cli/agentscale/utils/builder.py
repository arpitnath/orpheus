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
    supported_runtimes = ["python3", "nodejs20"]
    if data["runtime"] not in supported_runtimes:
        raise ConfigError(
            f"Unsupported runtime: {data['runtime']}\n"
            f"Supported: {', '.join(supported_runtimes)}"
        )

    # Add runtime_version if not specified
    if "runtime_version" not in data:
        if data["runtime"] == "python3":
            data["runtime_version"] = "3.10"
        elif data["runtime"] == "nodejs20":
            data["runtime_version"] = "20"

    # Add agent_path for reference
    data["agent_path"] = str(Path(agent_path).absolute())

    return data


# ============================================================================
# Base Image Resolution
# ============================================================================

def find_base_image(runtime: str, version: str) -> Path:
    """Find base image directory (works on Linux and macOS+Lima).

    Args:
        runtime: Runtime type (python3, nodejs20, etc.)
        version: Runtime version (3.10, 20, etc.)

    Returns:
        Path to base image directory

    Raises:
        ImageNotFoundError: If base image not found
    """
    images_dir = Path.home() / ".agentscale" / "images"

    # Determine image name based on runtime
    if runtime == "python3":
        image_name = f"python-{version}"
        build_script = f"./scripts/build-ubuntu-python-from-lima.sh"
    elif runtime == "nodejs20":
        image_name = f"nodejs-{version}"
        build_script = f"./scripts/build-nodejs20-from-lima.sh"
    else:
        raise ImageNotFoundError(
            f"Unsupported runtime: {runtime}\n"
            f"Supported: python3, nodejs20"
        )

    base_path = images_dir / image_name

    if not base_path.exists():
        raise ImageNotFoundError(
            f"Base image not found: {base_path}\n\n"
            f"Please build it first:\n"
            f"  cd agentscale\n"
            f"  {build_script}\n"
        )

    # Verify complete rootfs (must have /lib for dynamic linker)
    if not (base_path / "lib").exists():
        raise ImageNotFoundError(
            f"Incomplete base image (missing /lib): {base_path}\n\n"
            f"Rebuild with:\n"
            f"  cd agentscale\n"
            f"  {build_script}\n"
        )

    return base_path


# ============================================================================
# Agent Image Directory
# ============================================================================

def create_agent_image_dir(agent_name: str, force: bool = False, temp: bool = False) -> Path:
    """Create directory structure for agent image.

    Args:
        agent_name: Name of the agent
        force: If True, overwrite existing deployment
        temp: If True, create in temporary location (for remote builds)

    Returns:
        Path to agent image directory

    Raises:
        BuildError: If agent already deployed and force=False
    """
    if temp:
        # Remote build: use temporary directory
        import tempfile
        import uuid
        temp_base = Path(tempfile.gettempdir()) / ".agentscale-build"
        temp_base.mkdir(parents=True, exist_ok=True)
        temp_dir = temp_base / f"{agent_name}-{uuid.uuid4().hex[:8]}"
        temp_dir.mkdir(parents=True, exist_ok=True)
        (temp_dir / "packages").mkdir(exist_ok=True)
        (temp_dir / "agent").mkdir(exist_ok=True)
        return temp_dir

    # Local build: use final location
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
    # Note: Don't pre-create runtime/ - base image copy creates the structure
    # Only create directories for packages and agent code
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
    config_path: Optional[str] = None,
    verbose: bool = False,
    quiet: bool = False,
    temp: bool = False
) -> Dict[str, Any]:
    """Build complete agent image with runtime, dependencies, and code.

    Args:
        agent_path: Path to agent directory
        force: Overwrite existing deployment
        no_cache: Don't use pip cache
        config_path: Path to agentscale.yaml
        temp: Build in temporary location (for remote deployments)

    Returns:
        Dictionary with deployment result info

    Raises:
        DeployError: If build fails
    """
    # Step 1: Read agent configuration
    if verbose:
        print("[deploy] Reading agent configuration...")
    agent_config = read_agent_config(agent_path)

    if verbose:
        print(f"[deploy] Agent: {agent_config['name']}")
        print(f"[deploy] Runtime: {agent_config['runtime']} {agent_config['runtime_version']}")
        print("")

    # Step 2: Find base image
    if verbose:
        print("[deploy] Finding base image...")
    base_image_path = find_base_image(agent_config["runtime"], agent_config["runtime_version"])
    if verbose:
        print(f"[deploy] ✓ Base image: {base_image_path}")
        print("")

    # Step 3: Create agent image directory
    if verbose:
        print("[deploy] Creating agent image structure...")
    agent_image_dir = create_agent_image_dir(agent_config["name"], force=force, temp=temp)
    if verbose:
        location = "temporary location" if temp else str(agent_image_dir)
        print(f"[deploy] ✓ Created: {location}")
        print("")

    # Remaining steps will be implemented in subsequent tasks
    # For now, return basic result

    size_bytes, size_mb = calculate_size(agent_image_dir)

    # Step 4: Copy runtime from base image (creates complete rootfs)
    if verbose:
        print("[deploy] Step 1/5: Copying runtime...")
    copy_runtime(base_image_path, agent_image_dir)
    # Calculate size after base image copy
    base_size_bytes, base_size_mb = calculate_size(agent_image_dir)
    if verbose:
        print(f"[deploy] ✓ Runtime copied ({base_size_mb}MB)")
        print("")

    # Step 5: Install dependencies
    if verbose:
        print("[deploy] Step 2/5: Installing dependencies...")
    packages_dir = agent_image_dir / "packages"
    install_dependencies(agent_path, packages_dir, no_cache, verbose=verbose, runtime=agent_config["runtime"])
    deps_size_bytes, deps_size_mb = calculate_size(packages_dir)
    if verbose and deps_size_mb > 0:
        print(f"[deploy] ✓ Dependencies installed ({deps_size_mb}MB)")
    if verbose:
        print("")

    # Step 6: Copy agent code
    if verbose:
        print("[deploy] Step 3/5: Copying agent code...")
    agent_code_dir = agent_image_dir / "agent"
    copy_agent_code(agent_path, agent_code_dir, runtime=agent_config["runtime"])
    code_size_bytes, code_size_mb = calculate_size(agent_code_dir)
    if verbose:
        print(f"[deploy] ✓ Agent code copied ({code_size_mb}MB)")
        print("")

    # Step 6.5: Generate entrypoint
    if verbose:
        print("[deploy] Step 4/5: Generating entrypoint...")
    generate_entrypoint(agent_config, agent_code_dir)
    if verbose:
        print(f"[deploy] ✓ Entrypoint generated")
        print("")

    # Step 6.6: Copy agent.yaml to root (daemon expects it there)
    agent_yaml_src = agent_code_dir / "agent.yaml"
    agent_yaml_dst = agent_image_dir / "agent.yaml"
    if agent_yaml_src.exists():
        shutil.copy2(agent_yaml_src, agent_yaml_dst)

    # Step 7: Create manifest
    if verbose:
        print("[deploy] Step 5/5: Creating manifest...")
    create_manifest(agent_config, agent_image_dir, base_image_path)
    if verbose:
        print(f"[deploy] ✓ Manifest created")
        print("")

    # Step 8: Update agentscale.yaml (skip for temp builds - will update after finalization)
    if not config_path and not os.path.exists("agentscale.yaml"):
        config_path = None  # Will handle in config_updater

    if not temp and config_path is not False:  # Skip config update for temp builds
        if verbose:
            print("[deploy] Updating agentscale.yaml...")
        from agentscale.utils.config_updater import update_agentscale_yaml
        update_agentscale_yaml(agent_config, agent_image_dir, config_path, verbose=verbose)
        if verbose:
            print(f"[deploy] ✓ Configuration updated")

    size_bytes, size_mb = calculate_size(agent_image_dir)

    return {
        "agent_name": agent_config["name"],
        "image_path": str(agent_image_dir),
        "size_mb": size_mb,
        "runtime": f"{agent_config['runtime']}-{agent_config['runtime_version']}",
        "agent_config": agent_config,  # Include for env resolution
        "is_temp": temp  # Track if this is a temporary build
    }


def finalize_agent_image(temp_image_path: str, agent_name: str, force: bool = False, verbose: bool = False) -> str:
    """Move temporary build to final location.

    Called after successful remote deployment to finalize the local state.

    Args:
        temp_image_path: Path to temporary build directory
        agent_name: Agent name
        force: Overwrite existing deployment
        verbose: Show progress

    Returns:
        Path to final agent directory

    Raises:
        DeployError: If finalization fails
    """
    temp_path = Path(temp_image_path)
    if not temp_path.exists():
        raise DeployError(f"Temporary build not found: {temp_image_path}")

    # Determine final location
    agents_dir = Path.home() / ".agentscale" / "agents"
    final_path = agents_dir / agent_name

    # Check if final location already exists
    if final_path.exists():
        if not force:
            raise DeployError(
                f"Agent '{agent_name}' already deployed at {final_path}\n"
                f"Use --force to overwrite"
            )
        if verbose:
            print(f"[deploy] Removing existing deployment at {final_path}")
        shutil.rmtree(final_path)

    # Ensure parent directory exists
    agents_dir.mkdir(parents=True, exist_ok=True)

    # Move temp to final location
    try:
        shutil.move(str(temp_path), str(final_path))
        if verbose:
            print(f"[deploy] ✓ Finalized to {final_path}")
        return str(final_path)
    except Exception as e:
        raise DeployError(f"Failed to finalize deployment: {e}")


def cleanup_temp_build(temp_image_path: str, verbose: bool = False) -> None:
    """Clean up temporary build directory.

    Args:
        temp_image_path: Path to temporary build directory
        verbose: Show progress
    """
    temp_path = Path(temp_image_path)
    if temp_path.exists() and ".agentscale-build" in str(temp_path):
        try:
            shutil.rmtree(temp_path)
            if verbose:
                print(f"[deploy] Cleaned up temporary build")
        except Exception as e:
            # Don't fail if cleanup fails - just log it
            if verbose:
                print(f"[deploy] Warning: Failed to cleanup temp: {e}")


# ============================================================================
# Runtime Copy
# ============================================================================

def copy_runtime(base_image_path: Path, agent_image_dir: Path) -> None:
    """Copy complete base image to agent directory.

    Creates a complete Linux rootfs by copying the entire base image
    directory tree. This provides all necessary system libraries,
    binaries, and the Python runtime.

    Args:
        base_image_path: Path to base image directory
        agent_image_dir: Target agent directory (will be populated with rootfs)

    Raises:
        BuildError: If copy fails
    """
    try:
        # Copy entire directory tree directly into agent directory
        # This creates bin/, lib/, usr/, etc/ at the root of agent directory
        for item in base_image_path.iterdir():
            src = item
            dst = agent_image_dir / item.name

            if item.is_dir():
                shutil.copytree(
                    src,
                    dst,
                    symlinks=True,  # Preserve symlinks
                    ignore_dangling_symlinks=True,
                    dirs_exist_ok=True
                )
            else:
                shutil.copy2(src, dst)
    except Exception as e:
        raise BuildError(f"Failed to copy runtime: {e}")


# ============================================================================
# Dependency Installation
# ============================================================================

def install_dependencies(agent_path: str, packages_dir: Path, no_cache: bool = False, verbose: bool = False, runtime: str = "python3") -> None:
    """Install dependencies based on runtime type.

    For Python: Install from requirements.txt to packages_dir
    For Node.js: Run npm install in agent directory (node_modules are copied with code)

    Args:
        agent_path: Path to agent directory
        packages_dir: Target directory for packages (Python) or ignored (Node.js)
        no_cache: If True, don't use pip/npm cache
        verbose: Show progress
        runtime: Runtime type (python3 or nodejs20)
    """
    if runtime == "nodejs20":
        install_nodejs_dependencies(agent_path, no_cache, verbose)
    else:
        install_python_dependencies(agent_path, packages_dir, no_cache, verbose)


def install_python_dependencies(agent_path: str, packages_dir: Path, no_cache: bool = False, verbose: bool = False) -> None:
    """Install Python dependencies from requirements.txt.

    On macOS, downloads Linux ARM64 packages for Lima VM execution.
    """
    requirements_file = Path(agent_path) / "requirements.txt"

    # Handle missing requirements.txt
    if not requirements_file.exists():
        if verbose:
            print("[deploy] No requirements.txt found, skipping dependencies")
        return

    # Read and count packages
    with open(requirements_file) as f:
        lines = [line.strip() for line in f if line.strip() and not line.startswith("#")]

    if not lines:
        if verbose:
            print("[deploy] requirements.txt is empty, skipping dependencies")
        return

    if verbose:
        print(f"[deploy] Installing {len(lines)} package(s) for Linux ARM64...")

    # Ensure packages directory exists
    packages_dir.mkdir(parents=True, exist_ok=True)

    # Build pip command with Linux ARM64 target
    cmd = [
        sys.executable, "-m", "pip", "install",
        "--target", str(packages_dir),
        "--platform", "manylinux2014_aarch64",
        "--only-binary=:all:",
        "--python-version", "3.12",
        "--implementation", "cp",
        "--abi", "cp312",
        "-r", str(requirements_file),
        "--quiet"
    ]

    if no_cache:
        cmd.append("--no-cache-dir")

    try:
        subprocess.run(cmd, capture_output=True, text=True, check=True)

        darwin_files = list(packages_dir.rglob("*.darwin.so"))
        if darwin_files:
            raise BuildError(
                f"Found macOS packages (expected Linux): {[f.name for f in darwin_files[:3]]}\n"
                f"Try: pip install --upgrade pip"
            )

    except subprocess.CalledProcessError as e:
        raise BuildError(
            f"Failed to install dependencies:\n\n{e.stderr}\n\n"
            f"Check requirements.txt syntax and package availability on PyPI"
        )


def install_nodejs_dependencies(agent_path: str, no_cache: bool = False, verbose: bool = False) -> None:
    """Install Node.js dependencies from package.json.

    Dependencies are installed in-place (node_modules in agent dir).
    They will be copied with the agent code during deployment.
    """
    package_json = Path(agent_path) / "package.json"

    if not package_json.exists():
        if verbose:
            print("[deploy] No package.json found, skipping npm install")
        return

    if verbose:
        print("[deploy] Installing npm dependencies...")

    # Build npm command
    cmd = ["npm", "install", "--production"]
    if no_cache:
        cmd.append("--cache=/dev/null")

    try:
        result = subprocess.run(
            cmd,
            cwd=agent_path,
            capture_output=True,
            text=True,
            check=True
        )
        if verbose and result.stdout:
            # Count packages
            node_modules = Path(agent_path) / "node_modules"
            if node_modules.exists():
                pkg_count = len([d for d in node_modules.iterdir() if d.is_dir() and not d.name.startswith(".")])
                print(f"[deploy] ✓ Installed {pkg_count} npm packages")

    except subprocess.CalledProcessError as e:
        raise BuildError(
            f"Failed to install npm dependencies:\n\n{e.stderr}\n\n"
            f"Check package.json syntax and network connectivity"
        )
    except FileNotFoundError:
        raise BuildError(
            "npm not found. Please install Node.js to deploy Node.js agents."
        )


# ============================================================================
# Agent Code Copy
# ============================================================================

def copy_agent_code(agent_path: str, agent_code_dir: Path, runtime: str = "python3") -> None:
    """Copy entire agent directory to image.

    Args:
        agent_path: Source agent directory
        agent_code_dir: Target directory in image
        runtime: Runtime type (python3 or nodejs20)
    """
    # Define ignore patterns based on runtime
    def ignore_patterns(directory, files):
        ignored = []
        for f in files:
            # Always ignore these
            if f in ("__pycache__", ".git", ".gitignore", "venv", ".venv"):
                ignored.append(f)
            elif f.endswith((".pyc", ".pyo", ".egg-info")):
                ignored.append(f)
            # For Python, ignore node_modules
            # For Node.js, keep node_modules (they contain dependencies)
            elif f == "node_modules" and runtime != "nodejs20":
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
        # Fix permissions - container runs as UID 1000, needs readable files
        # Directories: 755, Files: 644
        for root, dirs, files in os.walk(agent_code_dir):
            for d in dirs:
                os.chmod(os.path.join(root, d), 0o755)
            for f in files:
                os.chmod(os.path.join(root, f), 0o644)
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
            "python_binary": "/usr/local/bin/python3.10",
            "packages_dir": "/packages",
            "agent_code": "/agent",
            "entrypoint": "/agent/_entrypoint.py"
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


# ============================================================================
# Entrypoint Generation
# ============================================================================

def generate_entrypoint(agent_config: Dict[str, Any], agent_code_dir: Path) -> None:
    """Generate entrypoint script for deployed agent.

    For Python: Generates _entrypoint.py
    For Node.js: Generates _entrypoint.mjs

    Args:
        agent_config: Agent configuration dict
        agent_code_dir: Path to agent code directory

    Raises:
        BuildError: If entrypoint generation fails
    """
    runtime = agent_config.get("runtime", "python3")

    if runtime == "nodejs20":
        generate_nodejs_entrypoint(agent_config, agent_code_dir)
    else:
        generate_python_entrypoint(agent_config, agent_code_dir)


def generate_python_entrypoint(agent_config: Dict[str, Any], agent_code_dir: Path) -> None:
    """Generate _entrypoint.py for Python agents."""
    from string import Template

    module = agent_config["module"].rstrip(".py")
    entrypoint = agent_config["entrypoint"]
    input_type = agent_config.get("input_type", "")

    template_str = '''#!/usr/bin/env python3
"""Auto-generated entry point - DO NOT EDIT"""
import sys
import json
import asyncio
import traceback
import inspect

from ${module} import ${entrypoint}
${input_type_import}

def main():
    try:
        input_data = sys.stdin.read().strip()
        data = json.loads(input_data) if input_data else {}

        ${input_handling}

        # Handle async handlers
        if inspect.iscoroutinefunction(${entrypoint}) or inspect.iscoroutine(result):
            if inspect.iscoroutine(result):
                result = asyncio.run(result)
            else:
                result = asyncio.run(${entrypoint}(data))

        # Serialize result
        if hasattr(result, 'model_dump'):
            output = result.model_dump()
        elif isinstance(result, dict):
            output = result
        else:
            output = {"result": str(result)}

        print(json.dumps(output))
    except Exception as e:
        print(json.dumps({
            "error": str(e),
            "traceback": traceback.format_exc(),
            "status": "error"
        }))
        sys.exit(1)

if __name__ == "__main__":
    main()
'''

    vars = {"module": module, "entrypoint": entrypoint}
    if input_type:
        vars["input_type_import"] = f"from {module} import {input_type}"
        vars["input_handling"] = f"input_obj = {input_type}(**data)\nresult = {entrypoint}(input_obj)"
    else:
        vars["input_type_import"] = ""
        vars["input_handling"] = f"result = {entrypoint}(data)"

    try:
        entrypoint_code = Template(template_str).substitute(vars)
        entrypoint_path = agent_code_dir / "_entrypoint.py"
        entrypoint_path.write_text(entrypoint_code)
        entrypoint_path.chmod(0o755)
    except Exception as e:
        raise BuildError(f"Failed to generate Python entrypoint: {e}")


def generate_nodejs_entrypoint(agent_config: Dict[str, Any], agent_code_dir: Path) -> None:
    """Generate _entrypoint.mjs for Node.js agents."""
    from string import Template

    # Strip .js, .mjs, .ts extensions
    module = agent_config["module"]
    for ext in [".js", ".mjs", ".ts"]:
        if module.endswith(ext):
            module = module[:-len(ext)]
            break

    entrypoint = agent_config["entrypoint"]

    template_str = '''#!/usr/bin/env node
/**
 * Auto-generated entry point - DO NOT EDIT
 * Generated by AgentScale for Node.js runtime
 */
import { readFileSync } from 'fs';

// Import the user's handler
const module = await import('./${module}.js');
const ${entrypoint} = module.${entrypoint} || module.default;

async function main() {
  try {
    // Read input from stdin
    const inputData = readFileSync(0, 'utf8').trim();
    const data = inputData ? JSON.parse(inputData) : {};

    // Call the handler
    let result = await ${entrypoint}(data);

    // Handle result serialization
    let output;
    if (result === null || result === undefined) {
      output = {};
    } else if (typeof result === 'object') {
      output = result;
    } else {
      output = { result: String(result) };
    }

    // Write JSON output to stdout
    console.log(JSON.stringify(output));

  } catch (e) {
    const errorOutput = {
      error: e.message,
      stack: e.stack,
      status: 'error'
    };
    console.log(JSON.stringify(errorOutput));
    process.exit(1);
  }
}

main();
'''

    try:
        entrypoint_code = Template(template_str).substitute({
            "module": module,
            "entrypoint": entrypoint
        })
        entrypoint_path = agent_code_dir / "_entrypoint.mjs"
        entrypoint_path.write_text(entrypoint_code)
        entrypoint_path.chmod(0o755)
    except Exception as e:
        raise BuildError(f"Failed to generate Node.js entrypoint: {e}")

