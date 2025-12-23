"""VM command implementation for Lima VM management on macOS."""

import os
import subprocess
import sys
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_error, print_success, print_info

app = typer.Typer(
    name="vm",
    help="Manage Lima VM for macOS agent execution",
)


def find_lima_template() -> Optional[Path]:
    """Find the Lima template (agentscale.yaml)."""
    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "lima" / "agentscale.yaml",
        # ~/.agentscale/lima (installed)
        Path.home() / ".agentscale" / "lima" / "agentscale.yaml",
    ]

    for loc in locations:
        if loc.exists():
            return loc

    return None


def find_daemon_binary_linux() -> Optional[Path]:
    """Find the Linux daemon binary for Lima VM."""
    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "bin" / "agentscale-daemon-linux",
        # ~/.agentscale/bin (installed)
        Path.home() / ".agentscale" / "bin" / "agentscale-daemon-linux",
    ]

    for loc in locations:
        if loc.exists():
            return loc

    return None


def copy_daemon_to_vm() -> bool:
    """Copy Linux daemon binary into VM."""
    daemon_binary = find_daemon_binary_linux()
    if not daemon_binary:
        print_error(
            "Linux daemon binary not found",
            "Build with: cd agentscale && GOOS=linux GOARCH=arm64 go build -o bin/agentscale-daemon-linux ./cmd/agentscale-daemon"
        )
        return False

    try:
        # Copy binary to VM
        print_info(f"Copying daemon binary from {daemon_binary}")
        subprocess.run([
            "limactl", "copy",
            str(daemon_binary),
            "agentscale:/tmp/agentscale-daemon"
        ], check=True, capture_output=True)

        # Move to /usr/local/bin with sudo
        subprocess.run([
            "limactl", "shell", "agentscale", "--",
            "sudo", "mv", "/tmp/agentscale-daemon", "/usr/local/bin/agentscale-daemon"
        ], check=True, capture_output=True)

        # Make executable
        subprocess.run([
            "limactl", "shell", "agentscale", "--",
            "sudo", "chmod", "+x", "/usr/local/bin/agentscale-daemon"
        ], check=True, capture_output=True)

        return True
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
            "limactl", "shell", "agentscale", "--",
            "sudo", "pkill", "-f", "agentscale-daemon"
        ], capture_output=True)  # Ignore errors if no process

        # Start daemon with environment variables
        cmd = f"{env_prefix}nohup /usr/local/bin/agentscale-daemon --socket /var/run/agentscale.sock > /var/log/agentscale-daemon.log 2>&1 &"
        subprocess.run([
            "limactl", "shell", "agentscale", "--",
            "sudo", "bash", "-c", cmd
        ], check=True, capture_output=True)

        # Wait a moment for socket to be created
        time.sleep(1)

        # Fix socket permissions so Lima can forward it
        subprocess.run([
            "limactl", "shell", "agentscale", "--",
            "sudo", "chmod", "666", "/var/run/agentscale.sock"
        ], capture_output=True)

        return True
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to start daemon: {e}")
        return False


def wait_for_daemon_socket(timeout: int = 10) -> bool:
    """Wait for daemon socket to be available."""
    import time
    socket_path = Path.home() / ".lima" / "agentscale" / "sock" / "agentscale.sock"

    for i in range(timeout):
        if socket_path.exists():
            return True
        time.sleep(1)

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
    """Get the status of the agentscale Lima instance."""
    try:
        result = subprocess.run(
            ["limactl", "list", "--format", "{{ .Name }}\t{{ .Status }}"],
            capture_output=True,
            text=True,
            check=True
        )

        for line in result.stdout.strip().split("\n"):
            if line.startswith("agentscale\t"):
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
            subprocess.run(["limactl", "stop", "agentscale"], check=True)
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
                "Expected at: agentscale/lima/agentscale.yaml"
            )
            raise typer.Exit(1)

        print_info("Creating AgentScale VM (this may take a few minutes on first run)...")
        print_info(f"Using template: {template}")

        try:
            subprocess.run(
                ["limactl", "start", "--tty=false", f"--name=agentscale", str(template)],
                check=True
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
                ["limactl", "start", "--tty=false", "agentscale"],
                check=True
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
        socket_path = Path.home() / ".lima" / "agentscale" / "sock" / "agentscale.sock"
        print_info(f"Socket: {socket_path}")
    else:
        print_error(
            "Daemon socket not found",
            "Check logs with: agentscale vm ssh -- sudo cat /var/log/agentscale-daemon.log"
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
        subprocess.run(["limactl", "stop", "agentscale"], check=True)
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
        print_info("Run 'agentscale vm start' to create the VM")
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
            if line.startswith("agentscale\t"):
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
                    socket_path = home / ".lima" / "agentscale" / "sock" / "agentscale.sock"
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
            "Start it with: agentscale vm start"
        )
        raise typer.Exit(1)

    print_info("Connecting to AgentScale VM...")
    os.execvp("limactl", ["limactl", "shell", "agentscale"])


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
        subprocess.run(["limactl", "delete", "--force", "agentscale"], check=True)
        print_success("VM deleted")
    except subprocess.CalledProcessError as e:
        print_error(f"Failed to delete VM: {e}")
        raise typer.Exit(1)
