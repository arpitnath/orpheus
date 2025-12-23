"""AgentScale CLI main entry point."""

import typer

from agentscale import __version__
from agentscale.commands.run import run
from agentscale.commands.client import invoke, stats, health
from agentscale.commands.deploy import deploy
from agentscale.commands import vm
from agentscale.commands import daemon

app = typer.Typer(
    name="agentscale",
    help="AgentScale - Infrastructure for AI agents",
    add_completion=False,
)


def version_callback(value: bool) -> None:
    """Print version and exit."""
    if value:
        print(f"agentscale {__version__}")
        raise typer.Exit()


@app.callback()
def main(
    version: bool = typer.Option(
        False,
        "--version",
        "-v",
        callback=version_callback,
        is_eager=True,
        help="Show version and exit",
    ),
) -> None:
    """AgentScale - Infrastructure for AI agents, like Docker but for agents."""
    pass


# Register commands
app.command()(run)
app.command()(deploy)
app.command()(invoke)
app.command()(stats)
app.command()(health)

# Register VM sub-app (Lima management for macOS)
app.add_typer(vm.app, name="vm")

# Register daemon sub-app (Linux daemon management)
app.add_typer(daemon.app, name="daemon")


if __name__ == "__main__":
    app()
