"""Server management commands for AgentScale daemon."""

import subprocess
import sys
import time
from pathlib import Path

import typer

from agentscale.utils.output import print_error, print_success, print_info

app = typer.Typer(
    name="server",
    help="Manage AgentScale server (daemon in TCP mode)",
)

# Timeout values for server operations (in seconds)
SERVER_COMMAND_TIMEOUT = 30  # Quick commands
SERVER_START_TIMEOUT = 120   # Full startup including VM


@app.command()
def start(
    port: int = typer.Option(7777, "--port", "-p", help="TCP port to bind"),
    tcp: bool = typer.Option(True, "--tcp/--no-tcp", help="Enable TCP mode"),
    create_key: bool = typer.Option(True, "--create-key/--no-create-key", help="Create API key if none exist"),
):
    """
    Start AgentScale daemon in TCP mode with authentication.

    This sets up the complete server infrastructure:
    - Stops any running daemon
    - Starts daemon with TCP + auth
    - Creates API key if needed
    - Shows connection details
    """
    if sys.platform != "darwin":
        print_error("This command is for macOS only", "Linux users should start daemon directly")
        raise typer.Exit(1)

    print_info("Starting AgentScale server setup...")
    print("")

    # Step 0: Ensure VM is running
    print_info("Checking VM status...")
    vm_check = subprocess.run(
        ["limactl", "list", "--json"],
        capture_output=True,
        text=True,
        timeout=10,
    )

    if vm_check.returncode != 0 or "Running" not in vm_check.stdout:
        print_info("VM not running. Starting VM...")
        try:
            subprocess.run(
                ["agentscale", "vm", "start"],
                check=True,
                timeout=SERVER_START_TIMEOUT,
            )
        except subprocess.TimeoutExpired:
            print_error("VM start timed out after 120s")
            raise typer.Exit(1)
        except subprocess.CalledProcessError:
            print_error("Failed to start VM")
            raise typer.Exit(1)

    # Step 1: Stop any running daemon
    print_info("Stopping existing daemon...")
    try:
        subprocess.run(
            ["limactl", "shell", "agentscale", "sudo", "pkill", "-f", "agentscale-daemon"],
            capture_output=True,
            check=False,
            timeout=SERVER_COMMAND_TIMEOUT,
        )
        time.sleep(2)
        print_success("Daemon stopped")
    except Exception as e:
        print_info("No daemon to stop")

    # Step 2: Setup auth database directory and fix permissions
    print_info("Setting up authentication database...")

    # Create directory and set permissions
    subprocess.run(
        ["limactl", "shell", "agentscale", "sudo", "mkdir", "-p", "/root/.agentscale"],
        check=True,
        capture_output=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    # Remove old database if exists
    subprocess.run(
        ["limactl", "shell", "agentscale", "sudo", "rm", "-f", "/root/.agentscale/keys.db"],
        check=False,
        capture_output=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    print_success("Auth database directory ready")

    # Step 3: Start daemon in TCP mode
    print_info(f"Starting daemon on TCP port {port}...")
    cmd = f"sudo /usr/local/bin/agentscale-daemon --tcp-bind :{port} > /tmp/daemon.log 2>&1 &"
    subprocess.run(
        ["limactl", "shell", "agentscale", "bash", "-c", cmd],
        check=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )
    time.sleep(3)
    print_success(f"Daemon started on port {port}")

    # Step 4: Check daemon logs
    result = subprocess.run(
        ["limactl", "shell", "agentscale", "tail", "-10", "/tmp/daemon.log"],
        capture_output=True,
        text=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    if "MCP endpoints enabled" in result.stdout and "Listening on TCP" in result.stdout:
        print_success("TCP server and MCP endpoints active")
    else:
        print_error("Daemon may not have started correctly")
        print("")
        print("Daemon logs:")
        print(result.stdout)
        raise typer.Exit(1)

    # Step 5: List or create API keys
    print("")
    print_info("Checking API keys...")

    result = subprocess.run(
        ["limactl", "shell", "agentscale", "sudo", "/usr/local/bin/agentscale-daemon", "list-keys"],
        capture_output=True,
        text=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    if "Total: 0 key(s)" in result.stdout or "No API keys found" in result.stdout:
        if create_key:
            print_info("No API keys found. Creating one...")
            result = subprocess.run(
                ["limactl", "shell", "agentscale", "sudo", "/usr/local/bin/agentscale-daemon",
                 "create-key", "--name", "default", "--rpm", "100"],
                capture_output=True,
                text=True,
                timeout=SERVER_COMMAND_TIMEOUT,
            )
            print("")
            print(result.stdout)

            # Extract key from output
            for line in result.stdout.split('\n'):
                if 'Key:' in line:
                    api_key = line.split('Key:')[1].strip()
                    print("")
                    print_success("Server is ready!")
                    print("")
                    print("Next steps:")
                    print(f"  1. Set API key:    export API_KEY={api_key}")
                    print(f"  2. Configure CLI:  agentscale login --server http://localhost:{port} --auth-key $API_KEY")
                    print(f"  3. Deploy agents:  agentscale deploy . --remote")
                    print("")
                    return
        else:
            print_error("No API keys found", "Run with --create-key to create one")
            raise typer.Exit(1)
    else:
        print_success("API keys found")
        print("")
        print(result.stdout)
        print("")
        print("Next steps:")
        print(f"  1. Copy an API key from above")
        print(f"  2. export API_KEY=agsk_your_key_here")
        print(f"  3. agentscale login --server http://localhost:{port} --auth-key $API_KEY")
        print(f"  4. agentscale deploy . --remote")
        print("")


@app.command()
def stop():
    """Stop AgentScale server."""
    if sys.platform != "darwin":
        print_error("This command is for macOS only")
        raise typer.Exit(1)

    print_info("Stopping AgentScale server...")

    subprocess.run(
        ["limactl", "shell", "agentscale", "sudo", "pkill", "-f", "agentscale-daemon"],
        capture_output=True,
        check=False,
    )

    print_success("Server stopped")


@app.command()
def status():
    """Show AgentScale server status."""
    if sys.platform != "darwin":
        print_error("This command is for macOS only")
        raise typer.Exit(1)

    result = subprocess.run(
        ["limactl", "shell", "agentscale", "ps", "aux"],
        capture_output=True,
        text=True,
    )

    daemon_procs = [line for line in result.stdout.split('\n') if 'agentscale-daemon' in line and 'grep' not in line]

    if daemon_procs:
        print_success("Server is running")
        print("")
        for proc in daemon_procs:
            parts = proc.split()
            if len(parts) > 10:
                cmd = ' '.join(parts[10:])
                print(f"  PID: {parts[1]}")
                print(f"  Command: {cmd}")
    else:
        print_info("Server is not running")
        print("")
        print("Start with: agentscale server start")
