"""AgentScale CLI main entry point."""

import typer

from agentscale import __version__
from agentscale.commands.run import run
from agentscale.commands.client import invoke, stats, health
from agentscale.commands.deploy import deploy
from agentscale.commands.shell import shell
from agentscale.commands.exec import exec_command
from agentscale.commands.status import status
from agentscale.commands.logs import logs
from agentscale.commands.list import list_agents
from agentscale.commands.ps import ps
from agentscale.commands.runs import runs
from agentscale.commands.inspect import inspect
from agentscale.commands.undeploy import undeploy
from agentscale.commands.validate import validate
from agentscale.commands.healthcheck import health as healthcheck
from agentscale.commands.test import test
from agentscale.commands.login import login
from agentscale.commands import vm
from agentscale.commands import daemon
from agentscale.commands import server

app = typer.Typer(
    name="orpheus",
    help="AgentScale - Infrastructure for AI agents",
    add_completion=False,
)


def version_callback(value: bool) -> None:
    """Print version and exit."""
    if value:
        print(f"orpheus {__version__}")
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
app.command()(shell)
app.command(name="exec")(exec_command)
app.command()(status)
app.command()(logs)
app.command(name="list")(list_agents)
app.command()(ps)
app.command()(runs)
app.command()(inspect)
app.command()(undeploy)
app.command()(validate)
app.command()(healthcheck)
app.command()(test)
app.command()(login)

# Register VM sub-app (Lima management for macOS)
app.add_typer(vm.app, name="vm")

# Register daemon sub-app (Linux daemon management)
app.add_typer(daemon.app, name="daemon")

# Register server sub-app (TCP server with auth)
app.add_typer(server.app, name="server")


if __name__ == "__main__":
    app()
