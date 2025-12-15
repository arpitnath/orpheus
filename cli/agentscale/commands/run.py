"""Run command implementation."""

import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Optional

import typer

from agentscale.utils.output import print_json, print_error

app = typer.Typer()


def find_runtime_binary() -> Optional[Path]:
    """Find the agentscale-runtime binary."""
    # Check common locations
    locations = [
        # Relative to CLI package (development)
        Path(__file__).parent.parent.parent.parent / "bin" / "agentscale-runtime",
        # ~/.agentscale/bin (installed)
        Path.home() / ".agentscale" / "bin" / "agentscale-runtime",
        # System PATH
        "agentscale-runtime",
    ]

    for loc in locations:
        if isinstance(loc, Path):
            if loc.exists():
                return loc
        else:
            # Check if it's in PATH
            from shutil import which
            if which(loc):
                return Path(which(loc))

    return None


@app.command()
def run(
    agent_dir: str = typer.Argument(..., help="Path to agent directory"),
    memory: Optional[int] = typer.Option(None, "--memory", "-m", help="Override memory limit (MB)"),
    timeout: Optional[int] = typer.Option(None, "--timeout", "-t", help="Override timeout (seconds)"),
    no_isolate: bool = typer.Option(False, "--no-isolate", help="Skip container isolation"),
    async_mode: bool = typer.Option(False, "--async", help="Use async template"),
    keep_entrypoint: bool = typer.Option(False, "--keep-entrypoint", help="Keep generated _entrypoint.py"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON without formatting"),
) -> None:
    """Run an agent with input from stdin."""

    # Find runtime binary
    runtime = find_runtime_binary()
    if not runtime:
        print_error(
            "agentscale-runtime binary not found",
            "Run 'make build-runtime' or install AgentScale properly"
        )
        raise typer.Exit(1)

    # Build command
    cmd = [str(runtime), "run", agent_dir]

    if memory:
        cmd.extend(["--memory", str(memory)])
    if timeout:
        cmd.extend(["--timeout", str(timeout)])
    if no_isolate:
        cmd.append("--no-isolate")
    if async_mode:
        cmd.append("--async")
    if keep_entrypoint:
        cmd.append("--keep-entrypoint")

    # Read stdin
    stdin_data = ""
    if not sys.stdin.isatty():
        stdin_data = sys.stdin.read()

    # Execute runtime
    try:
        result = subprocess.run(
            cmd,
            input=stdin_data,
            capture_output=True,
            text=True
        )

        # Parse output
        try:
            output = json.loads(result.stdout)
        except json.JSONDecodeError:
            output = {"raw_output": result.stdout, "status": "unknown"}

        # Handle stderr
        if result.stderr:
            output["stderr"] = result.stderr

        # Print output
        if raw:
            print(json.dumps(output))
        else:
            status = output.get("status", "unknown")
            title = f"Agent Result ({status})"
            print_json(output, title=title)

        # Exit with appropriate code
        raise typer.Exit(result.returncode)

    except FileNotFoundError:
        print_error(f"Runtime binary not found: {runtime}")
        raise typer.Exit(1)
    except subprocess.SubprocessError as e:
        print_error(f"Failed to execute runtime: {e}")
        raise typer.Exit(1)
