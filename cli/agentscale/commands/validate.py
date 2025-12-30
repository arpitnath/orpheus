"""Validate command - Validate agent before deployment."""

from pathlib import Path
import typer
import yaml

from agentscale.utils.output import print_error, print_info, print_success, print_warning


def validate(
    agent_path: str = typer.Argument(..., help="Path to agent directory"),
) -> None:
    """Validate agent configuration before deployment.

    Checks agent.yaml structure, module files, and requirements.txt
    to catch errors before deploying.

    Examples:
        agentscale validate ./my-agent
        agentscale validate ../calculator-agent
    """
    agent_dir = Path(agent_path)

    print_info(f"Validating agent at {agent_path}...")
    print("")

    errors = []
    warnings = []

    # Check 1: Directory exists
    if not agent_dir.exists():
        print_error("Agent directory not found", f"Path: {agent_path}")
        raise typer.Exit(1)

    if not agent_dir.is_dir():
        print_error("Agent path must be a directory", f"Path: {agent_path}")
        raise typer.Exit(1)

    # Check 2: agent.yaml exists
    agent_yaml = agent_dir / "agent.yaml"
    if not agent_yaml.exists():
        errors.append("agent.yaml not found")
    else:
        print_success("✓ agent.yaml found")

        # Check 3: Valid YAML
        try:
            config = yaml.safe_load(agent_yaml.read_text())

            # Check 4: Required fields
            required_fields = ["name", "runtime", "module", "entrypoint"]
            missing = [f for f in required_fields if f not in config]

            if missing:
                errors.append(f"Missing required fields: {', '.join(missing)}")
            else:
                print_success(f"✓ Required fields present")

            # Check 5: Module file exists
            if "module" in config:
                module_name = config["module"].rstrip(".py")
                module_file = agent_dir / f"{module_name}.py"

                if module_file.exists():
                    print_success(f"✓ Module file exists: {module_name}.py")
                else:
                    errors.append(f"Module file not found: {module_name}.py")

            # Check 6: Entrypoint specified
            if "entrypoint" in config:
                print_success(f"✓ Entrypoint function: {config['entrypoint']}")

            # Check 7: Runtime supported
            supported_runtimes = ["python3", "nodejs20"]
            if "runtime" in config and config["runtime"] not in supported_runtimes:
                errors.append(f"Unsupported runtime: {config['runtime']} (supported: {', '.join(supported_runtimes)})")

        except yaml.YAMLError as e:
            errors.append(f"Invalid YAML: {e}")

    # Check 8: requirements.txt (optional)
    req_file = agent_dir / "requirements.txt"
    if req_file.exists():
        try:
            reqs = req_file.read_text().strip().split("\n")
            reqs = [r.strip() for r in reqs if r.strip() and not r.startswith("#")]

            if reqs:
                print_success(f"✓ requirements.txt valid ({len(reqs)} package(s))")

                # Warn about large dependencies
                large_deps = ["tensorflow", "torch", "transformers", "opencv"]
                for dep in reqs:
                    dep_name = dep.split("==")[0].split(">=")[0].split("<=")[0].lower()
                    if any(large in dep_name for large in large_deps):
                        warnings.append(f"Large dependency detected: {dep_name} (may increase image size)")

            else:
                warnings.append("requirements.txt is empty")

        except Exception as e:
            warnings.append(f"Error reading requirements.txt: {e}")

    # Display summary
    print("")

    if errors:
        print_error("Validation failed", f"{len(errors)} error(s) found:")
        for error in errors:
            print(f"  ✗ {error}")
        print("")
        raise typer.Exit(1)

    if warnings:
        for warning in warnings:
            print_warning("⚠", warning)
        print("")

    print_success("Agent is ready to deploy!")
    print("")
    print("Deploy with:")
    print(f"  agentscale deploy {agent_path}")
    print("")
