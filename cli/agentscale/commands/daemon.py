"""Daemon command implementation for Linux."""

import os
import signal
import subprocess
import sys
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_error, print_success, print_info

app = typer.Typer(
    name="daemon",
    help="Manage the agentscale daemon (Linux only)",
)

DEFAULT_SOCKET = "/var/run/agentscale.sock"
PID_FILE = "/var/run/agentscale.pid"


def find_daemon_binary() -> Optional[Path]:
    """Find the agentscale-daemon binary."""
    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "bin" / "agentscale-daemon",
        # ~/.agentscale/bin (installed)
        Path.home() / ".agentscale" / "bin" / "agentscale-daemon",
        # System PATH
        "agentscale-daemon",
    ]

    for loc in locations:
        if isinstance(loc, Path):
            if loc.exists():
                return loc
        else:
            from shutil import which
            if which(loc):
                return Path(which(loc))

    return None


def get_daemon_pid() -> Optional[int]:
    """Get the daemon PID if running."""
    pid_file = Path(PID_FILE)
    if not pid_file.exists():
        return None

    try:
        pid = int(pid_file.read_text().strip())
        # Check if process exists
        os.kill(pid, 0)
        return pid
    except (ValueError, ProcessLookupError, PermissionError):
        # PID file exists but process doesn't
        return None


def is_daemon_running() -> bool:
    """Check if daemon is running."""
    return get_daemon_pid() is not None


@app.command()
def start(
    socket: str = typer.Option(DEFAULT_SOCKET, "--socket", "-s", help="Unix socket path"),
    foreground: bool = typer.Option(False, "--foreground", "-f", help="Run in foreground"),
) -> None:
    """Start the agentscale daemon.

    On macOS, use 'agentscale vm start' instead (daemon runs inside Lima VM).
    """
    if sys.platform == "darwin":
        print_error(
            "Use 'agentscale vm start' on macOS",
            "The daemon runs inside the Lima VM on macOS"
        )
        raise typer.Exit(1)

    # Check if already running
    if is_daemon_running():
        print_info("Daemon is already running")
        return

    # Find daemon binary
    daemon_binary = find_daemon_binary()
    if not daemon_binary:
        print_error(
            "agentscale-daemon binary not found",
            "Install AgentScale or run 'make build-daemon'"
        )
        raise typer.Exit(1)

    if foreground:
        # Run in foreground (for debugging)
        print_info(f"Starting daemon on {socket} (foreground)")
        try:
            subprocess.run([str(daemon_binary), "--socket", socket], check=True)
        except KeyboardInterrupt:
            print_info("Daemon stopped")
        except subprocess.CalledProcessError as e:
            print_error(f"Daemon exited with code {e.returncode}")
            raise typer.Exit(1)
    else:
        # Run in background
        print_info(f"Starting daemon on {socket}")
        try:
            proc = subprocess.Popen(
                [str(daemon_binary), "--socket", socket],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                start_new_session=True,
            )

            # Write PID file
            Path(PID_FILE).write_text(str(proc.pid))
            print_success(f"Daemon started (PID: {proc.pid})")

        except PermissionError:
            print_error(
                "Permission denied",
                f"Cannot write to {socket}. Try with sudo or use --socket /tmp/agentscale.sock"
            )
            raise typer.Exit(1)
        except Exception as e:
            print_error(f"Failed to start daemon: {e}")
            raise typer.Exit(1)


@app.command()
def stop() -> None:
    """Stop the agentscale daemon."""
    if sys.platform == "darwin":
        print_error(
            "Use 'agentscale vm stop' on macOS",
            "The daemon runs inside the Lima VM on macOS"
        )
        raise typer.Exit(1)

    pid = get_daemon_pid()
    if pid is None:
        print_info("Daemon is not running")
        return

    print_info(f"Stopping daemon (PID: {pid})")
    try:
        os.kill(pid, signal.SIGTERM)
        # Clean up PID file
        Path(PID_FILE).unlink(missing_ok=True)
        print_success("Daemon stopped")
    except ProcessLookupError:
        print_info("Daemon was not running")
        Path(PID_FILE).unlink(missing_ok=True)
    except PermissionError:
        print_error("Permission denied", "Try with sudo")
        raise typer.Exit(1)


@app.command()
def status() -> None:
    """Check daemon status."""
    if sys.platform == "darwin":
        print_info("On macOS, use 'agentscale vm status'")
        return

    pid = get_daemon_pid()
    if pid:
        print_success(f"Daemon is running (PID: {pid})")

        # Check socket
        socket_path = Path(DEFAULT_SOCKET)
        if socket_path.exists():
            print_info(f"Socket: {socket_path}")
        else:
            print_info("Socket: not found (daemon may be starting)")
    else:
        print_info("Daemon is not running")
        print_info("Start with: agentscale daemon start")
