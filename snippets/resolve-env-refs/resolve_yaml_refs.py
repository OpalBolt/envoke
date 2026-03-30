#!/usr/bin/env python3
"""
resolve_yaml_refs.py — Drop-in module for resolving bw:// and vault:// secret
references in YAML files. Works with any YAML structure: flat, nested, lists.

Secrets are resolved at load time using the same bw and vault CLIs as the
bash companion (resolve-env-refs.sh). Nothing is written to disk.

─────────────────────────────────────────────────────────────────────────────
Import usage
─────────────────────────────────────────────────────────────────────────────

    from resolve_yaml_refs import load_yaml

    # All bw:// and vault:// refs resolved; returns a plain Python dict.
    config = load_yaml("config.yaml")
    db_password = config["database"]["password"]
    api_key     = config["api"]["stripe_key"]

    # From a string instead of a file:
    from resolve_yaml_refs import load_yaml_string
    config = load_yaml_string(yaml_text)

    # Resolve a single reference string:
    from resolve_yaml_refs import resolve_value
    secret = resolve_value("bw://prod-db/password")

─────────────────────────────────────────────────────────────────────────────
CLI usage
─────────────────────────────────────────────────────────────────────────────

    # Resolved YAML to stdout (pipe to any tool):
    python resolve_yaml_refs.py config.yaml
    python resolve_yaml_refs.py config.yaml | helm upgrade myapp . -f -

    # Single value via dot-notation key:
    python resolve_yaml_refs.py config.yaml --key database.password
    python resolve_yaml_refs.py config.yaml --key api.keys.0   # list index

─────────────────────────────────────────────────────────────────────────────
Reference formats
─────────────────────────────────────────────────────────────────────────────

    bw://item-name              → Bitwarden password field (default)
    bw://item-name/password     → Bitwarden password field
    bw://item-name/username     → Bitwarden username field
    bw://item-name/note         → Bitwarden notes field
    bw://item-name/field:fname  → Bitwarden custom field named "fname"
    vault://secret/path#field   → Vault KV v2 single field

─────────────────────────────────────────────────────────────────────────────
Prerequisites
─────────────────────────────────────────────────────────────────────────────

    pip install pyyaml

    bw://  references: bw CLI installed; BW_SESSION exported
        export BW_SESSION=$(bw unlock --raw)   # or set BW_PASSWORD for non-interactive

    vault:// references: vault CLI installed; VAULT_ADDR and VAULT_TOKEN set
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from typing import Any

try:
    import yaml
except ImportError:
    sys.exit(
        "❌ pyyaml is required: pip install pyyaml\n"
        "   Or: pip install -r requirements.txt"
    )

_BW_PREFIX = "bw://"
_VAULT_PREFIX = "vault://"


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

def _redact_cmd(cmd: tuple[str, ...]) -> str:
    """Return a display-safe version of a command with sensitive args redacted."""
    redacted: list[str] = []
    skip_next = False
    for arg in cmd:
        if skip_next:
            redacted.append("[REDACTED]")
            skip_next = False
        elif arg in ("--session", "--token", "--password"):
            redacted.append(arg)
            skip_next = True
        else:
            redacted.append(arg)
    return " ".join(redacted)


def _run(*cmd: str, env: dict[str, str] | None = None) -> str:
    """Run a command, return stdout with trailing newline stripped, raise on failure.

    Raises:
        RuntimeError: Command not found (binary missing) or non-zero exit.
            Sensitive arguments (--session, --token, --password) are redacted
            in error messages to prevent credential leakage in logs.
    """
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            env={**os.environ, **(env or {})},
        )
    except FileNotFoundError:
        raise RuntimeError(
            f"Command not found: {cmd[0]!r}. Is it installed and on PATH?"
        )
    if result.returncode != 0:
        raise RuntimeError(
            f"Command failed: {_redact_cmd(cmd)}\n"
            f"stderr: {result.stderr.strip()}"
        )
    # Remove exactly one trailing newline (CLI convention) — do NOT strip
    # leading/trailing whitespace, which could silently corrupt secret values.
    output = result.stdout
    if output.endswith("\n"):
        output = output[:-1]
    return output


def _bw_session() -> str:
    """Return the active BW_SESSION, raising EnvironmentError if not set."""
    session = os.environ.get("BW_SESSION", "")
    if not session:
        raise EnvironmentError(
            "BW_SESSION is not set.\n"
            "Run: export BW_SESSION=$(bw unlock --raw)\n"
            "Or set BW_PASSWORD and call: export BW_SESSION=$(bw unlock --passwordenv BW_PASSWORD --raw)"
        )
    return session


def _resolve_bw(item_name: str, field_spec: str = "password") -> str:
    """Resolve a Bitwarden reference using the bw CLI."""
    session = _bw_session()

    if field_spec in ("password", ""):
        return _run("bw", "get", "password", item_name, "--session", session)
    elif field_spec == "username":
        return _run("bw", "get", "username", item_name, "--session", session)
    elif field_spec in ("note", "notes"):
        return _run("bw", "get", "notes", item_name, "--session", session)
    elif field_spec.startswith("field:"):
        fname = field_spec[len("field:"):]
        item_json = _run("bw", "get", "item", item_name, "--session", session)
        item = json.loads(item_json)
        for f in item.get("fields", []):
            if f.get("name") == fname:
                return f["value"]
        raise KeyError(
            f"Custom field '{fname}' not found in Bitwarden item '{item_name}'"
        )
    else:
        raise ValueError(
            f"Unknown bw field spec '{field_spec}'. "
            "Use: password, username, note, field:<name>"
        )


def _resolve_vault(path: str, field: str) -> str:
    """Resolve a Vault KV v2 reference using the vault CLI."""
    if not os.environ.get("VAULT_ADDR"):
        raise EnvironmentError(
            "VAULT_ADDR is not set. "
            "Example: export VAULT_ADDR=https://vault.example.com:8200"
        )
    return _run("vault", "kv", "get", f"-field={field}", path)


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def resolve_value(value: str) -> str:
    """Resolve a single bw:// or vault:// reference string.

    Returns the secret value as a string. Non-reference strings are returned
    unchanged, so it is safe to call this on any YAML scalar.

    Args:
        value: A reference string such as "bw://myitem/password" or
               "vault://secret/myapp#db_password", or any plain string.

    Returns:
        The resolved secret value, or the original string if not a reference.

    Raises:
        EnvironmentError: Missing BW_SESSION or VAULT_ADDR.
        RuntimeError: bw/vault CLI call failed.
        KeyError: Bitwarden custom field not found.
        ValueError: Unsupported reference format (e.g. vault:// without #field).
    """
    if value.startswith(_BW_PREFIX):
        ref = value[len(_BW_PREFIX):]
        if "/" in ref:
            item_name, field_spec = ref.split("/", 1)
        else:
            item_name, field_spec = ref, "password"
        return _resolve_bw(item_name, field_spec)

    if value.startswith(_VAULT_PREFIX):
        ref = value[len(_VAULT_PREFIX):]
        if "#" not in ref:
            raise ValueError(
                f"vault:// reference must include a field after '#': vault://path#field\n"
                f"Got: {value!r}"
            )
        path, field = ref.split("#", 1)
        return _resolve_vault(path, field)

    return value


def resolve_data(data: Any) -> Any:
    """Recursively resolve all bw:// and vault:// references in a data structure.

    Walks dicts, lists, and scalar strings. Any string that is a reference is
    resolved; all other values are returned unchanged.

    Args:
        data: Any Python object produced by yaml.safe_load.

    Returns:
        A new structure with all reference strings replaced by their secrets.
    """
    if isinstance(data, str):
        return resolve_value(data)
    if isinstance(data, dict):
        return {k: resolve_data(v) for k, v in data.items()}
    if isinstance(data, list):
        return [resolve_data(item) for item in data]
    return data


def load_yaml(path: str) -> Any:
    """Load a YAML file and resolve all bw:// and vault:// references.

    Args:
        path: Path to the YAML file.

    Returns:
        The parsed YAML data structure with all references resolved.

    Raises:
        FileNotFoundError: If the file does not exist.
        yaml.YAMLError: If the file is not valid YAML.
        EnvironmentError / RuntimeError / KeyError / ValueError: From resolve_value.
    """
    with open(path) as f:
        data = yaml.safe_load(f)
    return resolve_data(data)


def load_yaml_string(yaml_str: str) -> Any:
    """Parse a YAML string and resolve all bw:// and vault:// references.

    Args:
        yaml_str: A YAML-formatted string.

    Returns:
        The parsed YAML data structure with all references resolved.
    """
    data = yaml.safe_load(yaml_str)
    return resolve_data(data)


def _get_nested(data: Any, key_path: str) -> Any:
    """Navigate a nested data structure using dot-notation key path.

    List indices are specified as integers in the dot path: "items.0.name"

    Raises:
        KeyError: If a dict key is not found.
        IndexError: If a list index is out of range.
        TypeError: If traversal hits a non-container type.
    """
    v = data
    for part in key_path.split("."):
        if isinstance(v, dict):
            if part not in v:
                raise KeyError(f"Key '{part}' not found (full path: {key_path!r})")
            v = v[part]
        elif isinstance(v, list):
            try:
                v = v[int(part)]
            except ValueError:
                raise TypeError(
                    f"Expected list index, got '{part}' in path '{key_path}'"
                )
            except IndexError:
                raise IndexError(
                    f"List index {part} out of range in path '{key_path}'"
                )
        else:
            raise TypeError(
                f"Cannot traverse into {type(v).__name__!r} "
                f"with key '{part}' in path '{key_path}'"
            )
    return v


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> None:
    import argparse

    parser = argparse.ArgumentParser(
        description=(
            "Resolve bw:// and vault:// secret references in a YAML file.\n"
            "Outputs resolved YAML to stdout, or a single value with --key."
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Examples:\n"
            "  # Resolved YAML to stdout:\n"
            "  python resolve_yaml_refs.py config.yaml\n\n"
            "  # Single value via dot-notation key:\n"
            "  python resolve_yaml_refs.py config.yaml --key database.password\n\n"
            "  # Pipe resolved YAML to helm:\n"
            "  python resolve_yaml_refs.py values.yaml | helm upgrade myapp . -f -\n\n"
            "  # Use as a library:\n"
            "  from resolve_yaml_refs import load_yaml\n"
            "  config = load_yaml('config.yaml')\n"
        ),
    )
    parser.add_argument("file", help="YAML file to resolve")
    parser.add_argument(
        "--key",
        metavar="PATH",
        help="Dot-notation path to extract a single value (e.g. database.password)",
    )
    args = parser.parse_args()

    try:
        data = load_yaml(args.file)
    except FileNotFoundError:
        print(f"❌ File not found: {args.file}", file=sys.stderr)
        sys.exit(1)
    except yaml.YAMLError as e:
        print(f"❌ YAML parse error in {args.file}: {e}", file=sys.stderr)
        sys.exit(1)
    except (EnvironmentError, RuntimeError, KeyError, ValueError) as e:
        print(f"❌ {e}", file=sys.stderr)
        sys.exit(1)

    if args.key:
        try:
            value = _get_nested(data, args.key)
        except (KeyError, IndexError, TypeError) as e:
            print(f"❌ {e}", file=sys.stderr)
            sys.exit(1)
        # Print without trailing newline for easy shell capture: $(python ... --key x)
        print(value, end="")
    else:
        # Dump resolved YAML — no aliases, unicode preserved
        print(yaml.dump(data, default_flow_style=False, allow_unicode=True), end="")


if __name__ == "__main__":
    main()
