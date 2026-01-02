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
                ["orpheus", "vm", "start"],
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
            ["limactl", "shell", "orpheus", "sudo", "pkill", "-f", "orpheusd"],
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
        ["limactl", "shell", "orpheus", "sudo", "mkdir", "-p", "/root/.orpheus"],
        check=True,
        capture_output=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    # Remove old database if exists
    subprocess.run(
        ["limactl", "shell", "orpheus", "sudo", "rm", "-f", "/root/.orpheus/keys.db"],
        check=False,
        capture_output=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    print_success("Auth database directory ready")

    # Step 3: Start daemon in TCP mode (FIXED: proper shell nesting for redirection)
    print_info(f"Starting daemon on TCP port {port}...")
    cmd = f"/usr/local/bin/orpheusd --tcp-bind :{port} > /var/log/orpheusd.log 2>&1 & echo $!"
    result = subprocess.run(
        ["limactl", "shell", "orpheus", "sudo", "bash", "-c", cmd],
        capture_output=True,
        text=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    if result.returncode != 0:
        print_error("Failed to start daemon")
        if result.stderr:
            print(result.stderr)
        raise typer.Exit(1)

    daemon_pid = result.stdout.strip()
    print_success(f"Daemon started (PID: {daemon_pid})")

    # Step 4: Wait for daemon health via Unix socket (NO AUTH required)
    print_info("Waiting for daemon to become healthy...")

    deadline = time.time() + 30  # 30 second timeout
    attempt = 0
    last_error = None

    while time.time() < deadline:
        attempt += 1
        try:
            # Check health via Unix socket (doesn't require auth, but needs sudo for socket access)
            health_check = subprocess.run(
                ["limactl", "shell", "orpheus", "--",
                 "sudo", "curl", "-s", "--unix-socket", "/var/run/orpheus.sock",
                 "http://localhost/v1/health"],
                capture_output=True,
                text=True,
                timeout=5,
            )

            if health_check.returncode == 0 and health_check.stdout:
                import json
                try:
                    health_data = json.loads(health_check.stdout)
                    if health_data.get("status") == "healthy":
                        print_success(f"Daemon is healthy (verified in {attempt} attempts)")

                        # Also check logs for debugging (non-fatal if missing)
                        log_check = subprocess.run(
                            ["limactl", "shell", "orpheus", "sudo", "tail", "-5", "/var/log/orpheusd.log"],
                            capture_output=True,
                            text=True,
                            timeout=5,
                        )
                        if "MCP endpoints enabled" in log_check.stdout and "Listening on TCP" in log_check.stdout:
                            print_success("TCP server and MCP endpoints confirmed in logs")

                        break
                except json.JSONDecodeError:
                    last_error = f"Invalid JSON response: {health_check.stdout}"
            else:
                last_error = f"Health check failed: {health_check.stderr or 'empty response'}"
        except subprocess.TimeoutExpired:
            last_error = "Health check timeout"
        except Exception as e:
            last_error = str(e)

        time.sleep(0.5)  # Poll every 500ms
    else:
        # Timeout reached without success
        print_error(f"Daemon did not become healthy within 30s ({attempt} attempts)")
        print_error(f"Last error: {last_error}")
        print("")
        print("Check daemon logs:")
        print(f"  limactl shell orpheus sudo tail -20 /var/log/orpheusd.log")
        raise typer.Exit(1)

    # Step 5: List or create API keys
    print("")
    print_info("Checking API keys...")

    result = subprocess.run(
        ["limactl", "shell", "orpheus", "sudo", "/usr/local/bin/orpheusd", "list-keys"],
        capture_output=True,
        text=True,
        timeout=SERVER_COMMAND_TIMEOUT,
    )

    if "Total: 0 key(s)" in result.stdout or "No API keys found" in result.stdout:
        if create_key:
            print_info("No API keys found. Creating one...")
            result = subprocess.run(
                ["limactl", "shell", "orpheus", "sudo", "/usr/local/bin/orpheusd",
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
                    print(f"  2. Configure CLI:  orpheus login --server http://localhost:{port} --auth-key $API_KEY")
                    print(f"  3. Deploy agents:  orpheus deploy . --remote")
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
        print(f"  3. orpheus login --server http://localhost:{port} --auth-key $API_KEY")
        print(f"  4. orpheus deploy . --remote")
        print("")


@app.command()
def stop():
    """Stop AgentScale server."""
    if sys.platform != "darwin":
        print_error("This command is for macOS only")
        raise typer.Exit(1)

    print_info("Stopping AgentScale server...")

    subprocess.run(
        ["limactl", "shell", "orpheus", "sudo", "pkill", "-f", "orpheusd"],
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
        ["limactl", "shell", "orpheus", "ps", "aux"],
        capture_output=True,
        text=True,
    )

    daemon_procs = [line for line in result.stdout.split('\n') if 'orpheusd' in line and 'grep' not in line]

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
        print("Start with: orpheus server start")
