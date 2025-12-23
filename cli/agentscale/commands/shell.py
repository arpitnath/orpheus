"""Shell command - Interactive shell access to VM."""

import sys
import subprocess
import typer

from agentscale.utils.output import print_error, print_info


def shell() -> None:
    """Open interactive shell in AgentScale VM.

    Opens a bash shell inside the Lima VM for debugging and inspection.
    Only available on macOS with Lima VM.

    Examples:
        agentscale shell
    """
    if sys.platform != "darwin":
        print_error(
            "Shell command only available on macOS",
            "On Linux, the daemon runs natively. Use your regular shell."
        )
        raise typer.Exit(1)

    # Check if Lima is installed
    if not subprocess.run(
        ["which", "limactl"],
        capture_output=True
    ).returncode == 0:
        print_error(
            "Lima is not installed",
            "Install it with: brew install lima"
        )
        raise typer.Exit(1)

    # Check if VM is running
    result = subprocess.run(
        ["limactl", "list"],
        capture_output=True,
        text=True
    )

    if "agentscale" not in result.stdout or "Running" not in result.stdout:
        print_error(
            "AgentScale VM is not running",
            "Start it with: agentscale vm start"
        )
        raise typer.Exit(1)

    # Open shell
    print_info("Opening shell in AgentScale VM...")
    print_info("Type 'exit' to return to your terminal")
    print("")

    try:
        subprocess.run(["limactl", "shell", "agentscale"])
    except KeyboardInterrupt:
        print("")
        print_info("Shell session ended")
    except Exception as e:
        print_error("Failed to open shell", str(e))
        raise typer.Exit(1)
