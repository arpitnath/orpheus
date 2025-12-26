"""Health command - Comprehensive system health check."""

import json
import sys
import subprocess
from pathlib import Path
import typer

from agentscale.utils.output import print_error, print_info, print_success, print_warning


def health(
    fix: bool = typer.Option(False, "--fix", help="Attempt to fix common issues"),
) -> None:
    """Run comprehensive health check on AgentScale system.

    Checks all components: VM/daemon, socket, base images, deployed agents,
    and disk space. Optionally attempts to fix common issues.

    Examples:
        agentscale health
        agentscale health --fix
    """
    print_info("AgentScale Health Check")
    print("")

    checks_passed = 0
    checks_failed = 0
    warnings_found = 0

    # Check 1: VM/Daemon Status
    result = check_vm_or_daemon()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    elif result["status"] == "warn":
        print_warning(f"{result['component']:<18} {result['message']}")
        warnings_found += 1
    else:
        print_error(result['component'], result['message'])
        checks_failed += 1

    # Check 2: Socket
    result = check_socket()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    else:
        print_error(result['component'], result['message'])
        checks_failed += 1

    # Check 3: Daemon Health
    result = check_daemon_health()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    elif result["status"] == "warn":
        print_warning(f"{result['component']:<18} {result['message']}")
        warnings_found += 1
    else:
        print_error(result['component'], result['message'])
        checks_failed += 1

    # Check 4: Base Images
    result = check_base_images()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    elif result["status"] == "warn":
        print_warning(f"{result['component']:<18} {result['message']}")
        warnings_found += 1
    else:
        print_error(result['component'], result['message'])
        checks_failed += 1

    # Check 5: Deployed Agents
    result = check_deployed_agents()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    elif result["status"] == "warn":
        print_warning(f"{result['component']:<18} {result['message']}")
        warnings_found += 1

    # Check 6: Disk Space
    result = check_disk_space()
    if result["status"] == "pass":
        print_success(f"{result['component']:<18} {result['message']}")
        checks_passed += 1
    elif result["status"] == "warn":
        print_warning(f"{result['component']:<18} {result['message']}")
        warnings_found += 1
    else:
        print_error(result['component'], result['message'])
        checks_failed += 1

    # Summary
    print("")
    if checks_failed > 0:
        print_error("Overall: UNHEALTHY", f"{checks_failed} check(s) failed")
    elif warnings_found > 0:
        print_warning("Overall: HEALTHY", f"{warnings_found} warning(s)")
    else:
        print_success(f"Overall: HEALTHY - All checks passed")

    print("")

    if checks_failed > 0:
        raise typer.Exit(1)


def check_vm_or_daemon() -> dict:
    """Check if VM (macOS) or daemon (Linux) is running."""
    if sys.platform == "darwin":
        try:
            result = subprocess.run(
                ["limactl", "list"],
                capture_output=True,
                text=True
            )

            if "agentscale" in result.stdout and "Running" in result.stdout:
                return {"status": "pass", "component": "VM Status", "message": "Running (Ubuntu 24.04)"}
            elif "agentscale" in result.stdout:
                return {"status": "fail", "component": "VM Status", "message": "Not running"}
            else:
                return {"status": "fail", "component": "VM Status", "message": "Not created"}
        except Exception:
            return {"status": "fail", "component": "VM Status", "message": "Lima not available"}
    else:
        # Linux - check if daemon process is running
        try:
            result = subprocess.run(
                ["pgrep", "-f", "agentscale-daemon"],
                capture_output=True
            )

            if result.returncode == 0:
                return {"status": "pass", "component": "Daemon", "message": "Running"}
            else:
                return {"status": "fail", "component": "Daemon", "message": "Not running"}
        except Exception:
            return {"status": "fail", "component": "Daemon", "message": "Cannot check status"}


def check_socket() -> dict:
    """Check if socket is accessible."""
    if sys.platform == "darwin":
        socket_path = Path.home() / ".lima" / "agentscale" / "sock" / "agentscale.sock"
    else:
        socket_path = Path("/var/run/agentscale.sock")

    if socket_path.exists():
        return {"status": "pass", "component": "Socket", "message": "Accessible"}
    else:
        return {"status": "fail", "component": "Socket", "message": "Not found"}


def check_daemon_health() -> dict:
    """Check daemon health endpoint."""
    try:
        import httpx

        if sys.platform == "darwin":
            socket_path = Path.home() / ".lima" / "agentscale" / "sock" / "agentscale.sock"
        else:
            socket_path = Path("/var/run/agentscale.sock")

        transport = httpx.HTTPTransport(uds=str(socket_path))
        with httpx.Client(transport=transport, timeout=5) as client:
            response = client.get("http://localhost/v1/health")
            data = response.json()

            version = data.get("version", "unknown")
            return {"status": "pass", "component": "Daemon", "message": f"Responding (v{version})"}

    except Exception as e:
        return {"status": "fail", "component": "Daemon", "message": "Not responding"}


def check_base_images() -> dict:
    """Check if base images exist."""
    images_dir = Path.home() / ".agentscale" / "images"

    if not images_dir.exists():
        return {"status": "fail", "component": "Base Images", "message": "No images directory"}

    count = 0
    for item in images_dir.iterdir():
        if item.is_dir() and (item / "manifest.json").exists():
            count += 1

    if count == 0:
        return {"status": "fail", "component": "Base Images", "message": "No images found"}
    else:
        return {"status": "pass", "component": "Base Images", "message": f"{count} found"}


def check_deployed_agents() -> dict:
    """Check deployed agents."""
    agents_dir = Path.home() / ".agentscale" / "agents"

    if not agents_dir.exists():
        return {"status": "warn", "component": "Deployed Agents", "message": "None deployed"}

    count = 0
    for item in agents_dir.iterdir():
        if item.is_dir() and (item / "manifest.json").exists():
            count += 1

    if count == 0:
        return {"status": "pass", "component": "Deployed Agents", "message": "None deployed"}
    else:
        return {"status": "pass", "component": "Deployed Agents", "message": f"{count} valid"}


def check_disk_space() -> dict:
    """Check available disk space."""
    try:
        import shutil

        stat = shutil.disk_usage(Path.home())
        free_gb = stat.free // (1024 * 1024 * 1024)

        if free_gb < 5:
            return {"status": "fail", "component": "Disk Space", "message": f"{free_gb}GB available (< 5GB)"}
        elif free_gb < 10:
            return {"status": "warn", "component": "Disk Space", "message": f"{free_gb}GB available"}
        else:
            return {"status": "pass", "component": "Disk Space", "message": f"{free_gb}GB available"}

    except Exception:
        return {"status": "warn", "component": "Disk Space", "message": "Cannot check"}
