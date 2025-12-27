"""Output formatting utilities."""

import json
from rich.console import Console
from rich.panel import Panel
from rich.syntax import Syntax
from rich.progress import Progress, SpinnerColumn, TextColumn, BarColumn, TaskProgressColumn, TimeRemainingColumn, TransferSpeedColumn
from rich.live import Live
from rich.spinner import Spinner
from rich.text import Text

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


def create_deploy_progress() -> Progress:
    """Create progress tracker for deploy operations."""
    return Progress(
        SpinnerColumn(),
        TextColumn("[bold blue]{task.description}"),
        BarColumn(),
        TaskProgressColumn(),
        TransferSpeedColumn(),
        TimeRemainingColumn(),
        console=console,
    )


def show_spinner(message: str):
    """Show a spinner with message for operations of unknown duration."""
    return Live(
        Spinner("dots", text=Text(message, style="blue")),
        console=console,
        refresh_per_second=10,
    )
