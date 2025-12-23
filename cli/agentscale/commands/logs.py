"""Logs command - View daemon logs."""

import sys
import subprocess
from typing import Optional
import typer

from agentscale.utils.output import print_error, print_info


def logs(
    follow: bool = typer.Option(False, "-f", "--follow", help="Follow logs in real-time"),
    tail: int = typer.Option(50, "-n", "--tail", help="Number of lines to show"),
    grep: Optional[str] = typer.Option(None, "--grep", help="Filter logs by pattern"),
) -> None:
    """View AgentScale daemon logs.

    Shows logs from the AgentScale daemon for debugging and monitoring.
    On macOS, reads from Lima VM. On Linux, reads from local logs.

    Examples:
        agentscale logs              # Last 50 lines
        agentscale logs -f           # Follow in real-time
        agentscale logs -n 100       # Last 100 lines
        agentscale logs --grep error # Filter for errors
    """
    if sys.platform == "darwin":
        view_logs_macos(follow, tail, grep)
    else:
        view_logs_linux(follow, tail, grep)


def view_logs_macos(follow: bool, tail: int, grep: Optional[str]) -> None:
    """View logs from Lima VM (macOS)."""
    # Build command
    if follow:
        cmd = "sudo tail -f /var/log/agentscale-daemon.log"
    else:
        cmd = f"sudo tail -n {tail} /var/log/agentscale-daemon.log"

    if grep:
        cmd += f" | grep --color=auto -i '{grep}'"

    # Execute in VM
    try:
        subprocess.run(
            ["limactl", "shell", "agentscale", "--", "bash", "-c", cmd]
        )
    except KeyboardInterrupt:
        print("")
        print_info("Stopped following logs")
    except Exception as e:
        print_error("Failed to view logs", str(e))
        raise typer.Exit(1)


def view_logs_linux(follow: bool, tail: int, grep: Optional[str]) -> None:
    """View logs from local daemon (Linux)."""
    # Try journalctl first (systemd), fall back to file
    try:
        # Check if using systemd
        systemd_check = subprocess.run(
            ["systemctl", "is-active", "agentscale-daemon"],
            capture_output=True
        )

        if systemd_check.returncode == 0:
            # Using systemd - use journalctl
            cmd = ["journalctl", "-u", "agentscale-daemon", "-n", str(tail)]

            if follow:
                cmd.append("-f")

            if grep:
                proc = subprocess.Popen(cmd, stdout=subprocess.PIPE)
                subprocess.run(["grep", "-i", "--color=auto", grep], stdin=proc.stdout)
                proc.wait()
            else:
                subprocess.run(cmd)
        else:
            # Not using systemd - read from file
            if follow:
                cmd = ["tail", "-f", "-n", str(tail), "/var/log/agentscale-daemon.log"]
            else:
                cmd = ["tail", "-n", str(tail), "/var/log/agentscale-daemon.log"]

            if grep:
                proc = subprocess.Popen(cmd, stdout=subprocess.PIPE)
                subprocess.run(["grep", "-i", "--color=auto", grep], stdin=proc.stdout)
                proc.wait()
            else:
                subprocess.run(cmd)

    except KeyboardInterrupt:
        print("")
        print_info("Stopped following logs")
    except Exception as e:
        print_error("Failed to view logs", str(e))
        raise typer.Exit(1)
