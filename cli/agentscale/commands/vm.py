"""VM command implementation for Lima VM management on macOS."""

import os
import platform
import subprocess
import sys
import time
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_error, print_success, print_info, print_warning

app = typer.Typer(
    name="vm",
    help="Manage Lima VM for macOS agent execution",
)

# Timeout values for Lima operations (in seconds)
LIMA_COMMAND_TIMEOUT = 30   # Quick commands (list, status)
LIMA_START_TIMEOUT = 120    # VM creation/boot
LIMA_STOP_TIMEOUT = 60      # VM shutdown
LIMA_SSH_TIMEOUT = 30       # SSH connection attempts


def get_host_arch() -> str:
    """Get host architecture for binary selection."""
    machine = platform.machine().lower()
    if machine in ("arm64", "aarch64"):
        return "arm64"
    elif machine in ("x86_64", "amd64"):
        return "amd64"
    else:
        raise ValueError(f"Unsupported architecture: {machine}")


def find_lima_template() -> Optional[Path]:
    """Find the Lima template (orpheus.yaml)."""
    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "lima" / "orpheus.yaml",
        # ~/.orpheus/lima (installed)
        Path.home() / ".orpheus" / "lima" / "orpheus.yaml",
    ]

    for loc in locations:
        if loc.exists():
            return loc

    return None


def find_daemon_binary_linux() -> Optional[Path]:
    """Find the Linux daemon binary for Lima VM deployment."""
    arch = get_host_arch()
    binary_name = f"orpheusd-linux-{arch}"

    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "bin" / binary_name,
        # ~/.orpheus/bin (installed)
        Path.home() / ".orpheus" / "bin" / binary_name,
    ]

    for loc in locations:
        if loc.exists():
            return loc

    # Fallback: try without arch suffix (backward compat)
    fallback_locations = [
        Path(__file__).parent.parent.parent.parent / "bin" / "orpheusd-linux",
        Path.home() / ".orpheus" / "bin" / "orpheusd-linux",
    ]

    for loc in fallback_locations:
        if loc.exists():
            return loc

    return None


def copy_daemon_to_vm() -> bool:
    """Copy Linux daemon binary into VM."""
    daemon_binary = find_daemon_binary_linux()
    if not daemon_binary:
        arch = get_host_arch()
        print_error(
            f"Linux daemon binary not found for {arch}",
            f"Build with: cd orpheus && make build-daemon-linux-{arch}"
        )
        return False

    try:
        # Copy binary to VM
        print_info(f"Copying daemon binary from {daemon_binary}")
        subprocess.run([
            "limactl", "copy",
            str(daemon_binary),
            "orpheus:/tmp/orpheusd"
        ], check=True, capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)

        # Move to /usr/local/bin with sudo
        subprocess.run([
            "limactl", "shell", "orpheus", "--",
            "sudo", "mv", "/tmp/orpheusd", "/usr/local/bin/orpheusd"
        ], check=True, capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)

        # Make executable
        subprocess.run([
            "limactl", "shell", "orpheus", "--",
            "sudo", "chmod", "+x", "/usr/local/bin/orpheusd"
        ], check=True, capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)

        return True
    except subprocess.TimeoutExpired:
        print_error("Operation timed out", "Lima VM may be unresponsive")
        return False
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to copy daemon binary: {e}")
        return False


def start_daemon_in_vm() -> bool:
    """Start daemon inside Lima VM with environment variables."""
    import time

    # Build environment variable string for API keys
    env_vars = []
    if os.environ.get("OPENAI_API_KEY"):
        env_vars.append(f"OPENAI_API_KEY={os.environ['OPENAI_API_KEY']}")
    if os.environ.get("ANTHROPIC_API_KEY"):
        env_vars.append(f"ANTHROPIC_API_KEY={os.environ['ANTHROPIC_API_KEY']}")

    env_prefix = " ".join(env_vars) + " " if env_vars else ""

    try:
        # Kill any existing daemon first
        subprocess.run([
            "limactl", "shell", "orpheus", "--",
            "sudo", "pkill", "-f", "orpheusd"
        ], capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)  # Ignore errors if no process

        # Start daemon with environment variables
        cmd = f"{env_prefix}nohup /usr/local/bin/orpheusd --socket /var/run/orpheus.sock > /var/log/orpheusd.log 2>&1 &"
        subprocess.run([
            "limactl", "shell", "orpheus", "--",
            "sudo", "bash", "-c", cmd
        ], check=True, capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)

        # Wait a moment for socket to be created
        time.sleep(1)

        # Fix socket permissions so Lima can forward it
        subprocess.run([
            "limactl", "shell", "orpheus", "--",
            "sudo", "chmod", "666", "/var/run/orpheus.sock"
        ], capture_output=True, timeout=LIMA_COMMAND_TIMEOUT)

        return True
    except subprocess.TimeoutExpired:
        print_error("Operation timed out", "Lima VM may be unresponsive")
        return False
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to start daemon: {e}")
        return False


def wait_for_daemon_socket(timeout: int = 10) -> bool:
    """Wait for daemon socket to be available."""
    socket_path = Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"

    for i in range(timeout):
        if socket_path.exists():
            return True
        time.sleep(1)

    return False


def wait_for_ssh_ready(max_attempts: int = 15, interval: float = 2.0) -> bool:
    """
    Wait for SSH to be ready inside the VM.

    Probes SSH by running a simple echo command.
    Returns True if SSH is ready, False if timeout.
    """
    for attempt in range(max_attempts):
        try:
            result = subprocess.run(
                ["limactl", "shell", "orpheus", "--", "echo", "ready"],
                capture_output=True,
                text=True,
                timeout=5,  # Quick timeout for each probe
            )
            if result.returncode == 0 and "ready" in result.stdout:
                return True
        except (subprocess.TimeoutExpired, subprocess.CalledProcessError):
            pass

        time.sleep(interval)

    return False


def check_lima_installed() -> bool:
    """Check if Lima is installed."""
    try:
        subprocess.run(
            ["limactl", "--version"],
            capture_output=True,
            check=True
        )
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        return False


def get_instance_status() -> Optional[str]:
    """Get the status of the orpheus Lima instance."""
    try:
        result = subprocess.run(
            ["limactl", "list", "--format", "{{ .Name }}\t{{ .Status }}"],
            capture_output=True,
            text=True,
            check=True
        )

        for line in result.stdout.strip().split("\n"):
            if line.startswith("orpheus\t"):
                return line.split("\t")[1]

        return None  # Instance doesn't exist
    except (subprocess.CalledProcessError, FileNotFoundError):
        return None


@app.command()
def start(
    force: bool = typer.Option(False, "--force", "-f", help="Force restart if already running"),
) -> None:
    """Start the Lima VM for agent execution.

    Downloads Ubuntu 24.04 and installs runc on first run.
    Subsequent starts are much faster.
    """
    # Check Lima is installed
    if not check_lima_installed():
        print_error(
            "Lima is not installed",
            "Install it with: brew install lima"
        )
        raise typer.Exit(1)

    # Check current status
    status = get_instance_status()

    if status == "Running":
        if force:
            print_info("Stopping existing VM...")
            subprocess.run(["limactl", "stop", "orpheus"], check=True, timeout=LIMA_STOP_TIMEOUT)
        else:
            print_success("VM is already running")
            return

    # Find template
    template = find_lima_template()

    if status is None:
        # Instance doesn't exist - create it
        if not template:
            print_error(
                "Lima template not found",
                "Expected at: orpheus/lima/orpheus.yaml"
            )
            raise typer.Exit(1)

        print_info("Creating AgentScale VM (this may take a few minutes on first run)...")
        print_info(f"Using template: {template}")

        try:
            subprocess.run(
                ["limactl", "start", "--tty=false", f"--name=orpheus", str(template)],
                check=True,
                timeout=LIMA_START_TIMEOUT
            )
            print_success("VM created and started!")
        except subprocess.CalledProcessError as e:
            print_error(f"Failed to create VM: {e}")
            raise typer.Exit(1)
    else:
        # Instance exists but stopped - start it
        print_info("Starting AgentScale VM...")
        try:
            subprocess.run(
                ["limactl", "start", "--tty=false", "orpheus"],
                check=True,
                timeout=LIMA_START_TIMEOUT
            )
            print_success("VM started!")
        except subprocess.CalledProcessError as e:
            print_error(f"Failed to start VM: {e}")
            raise typer.Exit(1)

    # Provision daemon in VM
    print_info("Setting up daemon...")

    # Copy daemon binary
    if not copy_daemon_to_vm():
        raise typer.Exit(1)

    # Start daemon
    print_info("Starting daemon...")
    if not start_daemon_in_vm():
        raise typer.Exit(1)

    # Wait for socket
    print_info("Waiting for daemon socket...")
    if wait_for_daemon_socket(timeout=15):
        print_success("Daemon is ready!")
        socket_path = Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"
        print_info(f"Socket: {socket_path}")
    else:
        print_error(
            "Daemon socket not found",
            "Check logs with: orpheus vm ssh -- sudo cat /var/log/orpheusd.log"
        )


@app.command()
def stop() -> None:
    """Stop the Lima VM."""
    if not check_lima_installed():
        print_error("Lima is not installed")
        raise typer.Exit(1)

    status = get_instance_status()

    if status is None:
        print_info("VM does not exist")
        return

    if status != "Running":
        print_info("VM is already stopped")
        return

    print_info("Stopping AgentScale VM...")
    try:
        subprocess.run(["limactl", "stop", "orpheus"], check=True)
        print_success("VM stopped")
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to stop VM: {e}")
        raise typer.Exit(1)


@app.command()
def status() -> None:
    """Show Lima VM status."""
    if not check_lima_installed():
        print_error(
            "Lima is not installed",
            "Install it with: brew install lima"
        )
        raise typer.Exit(1)

    vm_status = get_instance_status()

    if vm_status is None:
        print_info("VM Status: Not Created")
        print_info("Run 'orpheus vm start' to create the VM")
        return

    # Get detailed info
    try:
        result = subprocess.run(
            ["limactl", "list", "--format",
             "{{ .Name }}\t{{ .Status }}\t{{ .Arch }}\t{{ .CPUs }}\t{{ .Memory }}\t{{ .Disk }}"],
            capture_output=True,
            text=True,
            check=True
        )

        for line in result.stdout.strip().split("\n"):
            if line.startswith("orpheus\t"):
                parts = line.split("\t")
                name, status, arch, cpus, memory, disk = parts[:6]

                if status == "Running":
                    print_success(f"VM Status: {status}")
                else:
                    print_info(f"VM Status: {status}")

                print_info(f"Architecture: {arch}")
                print_info(f"CPUs: {cpus}")
                print_info(f"Memory: {memory}")
                print_info(f"Disk: {disk}")

                # Show socket path if running
                if status == "Running":
                    home = Path.home()
                    socket_path = home / ".lima" / "orpheus" / "sock" / "orpheus.sock"
                    print_info(f"Socket: {socket_path}")

                return

        print_info(f"VM Status: {vm_status}")

    except subprocess.CalledProcessError as e:
        print_info(f"VM Status: {vm_status}")


@app.command()
def ssh() -> None:
    """SSH into the Lima VM."""
    if not check_lima_installed():
        print_error("Lima is not installed")
        raise typer.Exit(1)

    status = get_instance_status()

    if status != "Running":
        print_error(
            "VM is not running",
            "Start it with: orpheus vm start"
        )
        raise typer.Exit(1)

    # Wait for SSH to be ready
    print_info("Checking SSH availability...")
    if not wait_for_ssh_ready(max_attempts=15, interval=2.0):
        print_error("SSH not ready after 30 seconds")
        print_info("VM may still be booting. Try again in a few seconds.")
        raise typer.Exit(1)

    print_info("Connecting to AgentScale VM...")
    os.execvp("limactl", ["limactl", "shell", "orpheus"])


@app.command()
def delete(
    force: bool = typer.Option(False, "--force", "-f", help="Force delete without confirmation"),
) -> None:
    """Delete the Lima VM and all its data."""
    if not check_lima_installed():
        print_error("Lima is not installed")
        raise typer.Exit(1)

    status = get_instance_status()

    if status is None:
        print_info("VM does not exist")
        return

    if not force:
        confirm = typer.confirm("Are you sure you want to delete the VM? This cannot be undone.")
        if not confirm:
            print_info("Cancelled")
            return

    print_info("Deleting AgentScale VM...")
    try:
        subprocess.run(["limactl", "delete", "--force", "orpheus"], check=True)
        print_success("VM deleted")
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to delete VM: {e}")
        raise typer.Exit(1)
