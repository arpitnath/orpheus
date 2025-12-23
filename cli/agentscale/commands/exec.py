"""Exec command - Execute commands in VM."""

import sys
import subprocess
import typer

from agentscale.utils.output import print_error


def exec_command(
    command: str = typer.Argument(..., help="Command to execute in VM"),
) -> None:
    """Execute a command in the AgentScale VM.

    Runs a one-off command inside the VM and returns the output.
    Useful for debugging and inspection without opening an interactive shell.

    Examples:
        agentscale exec "runc list"
        agentscale exec "ps aux | grep python"
        agentscale exec "cat /var/log/agentscale-daemon.log | tail -20"
    """
    if sys.platform == "darwin":
        # macOS: Execute in Lima VM
        result = subprocess.run(
            ["limactl", "shell", "agentscale", "--", "sudo", "bash", "-c", command],
            text=True
        )
        raise typer.Exit(result.returncode)

    else:
        # Linux: Execute locally
        result = subprocess.run(
            ["bash", "-c", command],
            text=True
        )
        raise typer.Exit(result.returncode)
