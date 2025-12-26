"""Output formatting utilities."""

import json
from rich.console import Console
from rich.panel import Panel
from rich.syntax import Syntax

console = Console()
error_console = Console(stderr=True)


def print_json(data: dict, title: str = None) -> None:
    """Print formatted JSON output."""
    json_str = json.dumps(data, indent=2)
    syntax = Syntax(json_str, "json", theme="monokai", line_numbers=False)

    if title:
        console.print(Panel(syntax, title=title, border_style="green"))
    else:
        console.print(syntax)


def print_error(message: str, details: str = None) -> None:
    """Print error message."""
    error_console.print(f"[red]Error:[/red] {message}")
    if details:
        error_console.print(f"[dim]{details}[/dim]")


def print_success(message: str) -> None:
    """Print success message."""
    console.print(f"[green]✓[/green] {message}")


def print_info(message: str) -> None:
    """Print info message."""
    console.print(f"[blue]ℹ[/blue] {message}")


def print_warning(message: str, details: str = None) -> None:
    """Print warning message."""
    console.print(f"[yellow]⚠[/yellow] {message}")
    if details:
        console.print(f"[dim]{details}[/dim]")
