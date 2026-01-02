"""Runs command - List recent agent executions."""

import json
import re
import sys
import subprocess
from pathlib import Path
from typing import Optional, List, Dict
import typer

from agentscale.utils.output import print_error, print_info


def runs(
    run_id: Optional[str] = typer.Argument(None, help="Show details for specific run ID"),
    agent: Optional[str] = typer.Option(None, "--agent", help="Filter by agent name"),
    status: Optional[str] = typer.Option(None, "--status", help="Filter by status (success/error)"),
    limit: int = typer.Option(20, "-n", help="Number of runs to show"),
) -> None:
    """List recent agent executions.

    Shows recent agent runs with their status, duration, and timestamps.
    Parses daemon logs to extract execution history.

    Examples:
        orpheus runs                      # Last 20 runs
        orpheus runs --agent calculator   # Runs for specific agent
        orpheus runs --status error       # Failed runs only
        orpheus runs abc123               # Details for specific run
    """
    # Get logs
    log_lines = get_daemon_logs()

    if not log_lines:
        print_info("No runs found in daemon logs")
        return

    # Parse runs from logs
    parsed_runs = parse_runs_from_logs(log_lines)

    # Filter runs
    if agent:
        parsed_runs = [r for r in parsed_runs if agent.lower() in r["agent"].lower()]

    if status:
        parsed_runs = [r for r in parsed_runs if r["status"].lower() == status.lower()]

    # Limit results
    parsed_runs = parsed_runs[:limit]

    if not parsed_runs:
        print_info("No runs match the filters")
        return

    # Show specific run details
    if run_id:
        show_run_details(run_id, parsed_runs)
        return

    # Show runs table
    print("")
    print(f"{'AGENT':<20} {'STATUS':<10} {'DURATION':<12} {'TIME':<20}")
    print("-" * 70)

    for run in parsed_runs:
        agent_name = run["agent"][:19]
        run_status = run["status"][:9]
        duration = run["duration"]
        timestamp = run["timestamp"][:19]

        print(f"{agent_name:<20} {run_status:<10} {duration:<12} {timestamp:<20}")

    print("")
    print(f"Total: {len(parsed_runs)} run(s)")
    print("")


def get_daemon_logs() -> List[str]:
    """Get daemon log lines."""
    try:
        if sys.platform == "darwin":
            result = subprocess.run(
                ["limactl", "shell", "orpheus", "--", "sudo", "cat", "/var/log/orpheusd.log"],
                capture_output=True,
                text=True,
                check=True
            )
            return result.stdout.strip().split("\n")
        else:
            # Try journalctl first
            result = subprocess.run(
                ["journalctl", "-u", "orpheusd", "-n", "1000", "--no-pager"],
                capture_output=True,
                text=True
            )
            if result.returncode == 0:
                return result.stdout.strip().split("\n")

            # Fall back to file
            log_file = Path("/var/log/orpheusd.log")
            if log_file.exists():
                return log_file.read_text().strip().split("\n")

            return []
    except Exception:
        return []


def parse_runs_from_logs(log_lines: List[str]) -> List[Dict]:
    """Parse agent runs from daemon logs."""
    runs = []

    # Pattern to match agent execution logs
    # Example: "Received run request: agent_path=/path/to/agent"
    # Followed by: "Container completed: duration=981ms status=1"

    for i, line in enumerate(log_lines):
        # Look for run request
        if "Received run request" in line or "agent_path=" in line:
            agent_path_match = re.search(r'agent_path=([^\s]+)', line)
            if agent_path_match:
                agent_path = agent_path_match.group(1)
                agent_name = agent_path.split("/")[-1]

                # Extract timestamp
                timestamp = extract_timestamp(line)

                # Look ahead for completion
                status = "running"
                duration = "0ms"

                for j in range(i + 1, min(i + 20, len(log_lines))):
                    next_line = log_lines[j]

                    if "Container completed" in next_line or "duration=" in next_line:
                        # Extract duration
                        duration_match = re.search(r'duration[=:](\d+)ms', next_line)
                        if duration_match:
                            duration = f"{duration_match.group(1)}ms"

                        # Extract status
                        if "status=1" in next_line or "exit status 1" in next_line or "error" in next_line.lower():
                            status = "error"
                        elif "status=0" in next_line or "success" in next_line.lower():
                            status = "success"

                        break

                runs.append({
                    "agent": agent_name,
                    "status": status,
                    "duration": duration,
                    "timestamp": timestamp,
                })

    # Reverse to show most recent first
    return list(reversed(runs))


def extract_timestamp(log_line: str) -> str:
    """Extract timestamp from log line."""
    # Try to extract timestamp (format: 2025/12/23 18:38:02)
    timestamp_match = re.match(r'(\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})', log_line)
    if timestamp_match:
        return timestamp_match.group(1).replace("/", "-")

    return "Unknown"


def show_run_details(run_id: str, runs: List[Dict]) -> None:
    """Show details for a specific run."""
    print_info(f"Run details not yet implemented for: {run_id}")
    print("")
    print("For now, use:")
    print("  orpheus logs --grep <agent-name>")
