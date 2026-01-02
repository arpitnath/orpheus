"""Test command - Verbose agent testing."""

import json
import sys
import time
from pathlib import Path
import typer

from agentscale.utils.output import print_error, print_info, print_success, print_json


def test(
    agent_name: str = typer.Argument(..., help="Agent name to test"),
    input_data: str = typer.Argument(..., help="JSON input for the agent"),
    verbose: bool = typer.Option(False, "--verbose", "-v", help="Show detailed execution logs"),
) -> None:
    """Test an agent with verbose output.

    Runs an agent and displays detailed timing breakdown and execution logs.
    Similar to invoke but with more diagnostic information.

    Examples:
        orpheus test calculator-agent '{"query": "test"}'
        orpheus test calculator-agent '{"query": "What is 5+3?"}' --verbose
    """
    # Get socket path
    if sys.platform == "darwin":
        socket_path = Path.home() / ".lima" / "orpheus" / "sock" / "orpheus.sock"
    else:
        socket_path = Path("/var/run/orpheus.sock")

    # Check socket exists
    if not socket_path.exists():
        print_error(
            "Daemon not running",
            "Start with: orpheus vm start" if sys.platform == "darwin" else "orpheus daemon start"
        )
        raise typer.Exit(1)

    # Find agent
    agents_dir = Path.home() / ".orpheus" / "agents"
    agent_dir = agents_dir / agent_name

    if not agent_dir.exists():
        print_error(
            f"Agent '{agent_name}' not found",
            "List agents with: orpheus list"
        )
        raise typer.Exit(1)

    # Parse input JSON
    try:
        input_json = json.loads(input_data)
    except json.JSONDecodeError as e:
        print_error("Invalid JSON input", str(e))
        raise typer.Exit(1)

    # Display test header
    print("")
    print(f"Testing: {agent_name}")
    print("")

    if verbose:
        print_info("[0.000s] Preparing request...")

    start_time = time.time()

    # Make request
    try:
        import httpx

        transport = httpx.HTTPTransport(uds=str(socket_path))

        with httpx.Client(transport=transport, timeout=600) as client:
            if verbose:
                elapsed = time.time() - start_time
                print_info(f"[{elapsed:.3f}s] Sending request to daemon...")

            response = client.post(
                "http://localhost/v1/agents/run",
                json={
                    "agent_path": str(agent_dir),
                    "input": input_json
                },
                timeout=600
            )

            result = response.json()

            total_duration = time.time() - start_time

            if verbose:
                print_info(f"[{total_duration:.3f}s] Response received")
                print("")

            # Display results
            status = result.get("status", "unknown")
            duration_ms = result.get("duration_ms", 0)

            if status == "success":
                print_success(f"Status: {status}")
            elif status == "error":
                print_error("Status", status)
            else:
                print_info(f"Status: {status}")

            print(f"Duration: {duration_ms}ms")
            print("")

            # Show output
            if "output" in result and result["output"]:
                print("Output:")
                print_json(result["output"])
            elif "raw_output" in result:
                print("Raw Output:")
                print(result["raw_output"])

            # Show error if any
            if "error" in result and result["error"]:
                print("")
                print_error("Error", result["error"])

            # Show stderr if verbose
            if verbose and "stderr" in result and result["stderr"]:
                print("")
                print("Stderr:")
                print(result["stderr"])

            print("")

            # Exit with error code if agent failed
            if status == "error":
                raise typer.Exit(1)

    except httpx.ConnectError:
        print_error(
            "Cannot connect to daemon",
            "Start with: orpheus vm start" if sys.platform == "darwin" else "orpheus daemon start"
        )
        raise typer.Exit(1)
    except httpx.TimeoutException:
        print_error("Request timed out", "Agent execution exceeded 600s timeout")
        raise typer.Exit(1)
    except Exception as e:
        print_error("Test failed", str(e))
        raise typer.Exit(1)
