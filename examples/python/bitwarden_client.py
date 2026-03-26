"""
bitwarden_client.py — Retrieve secrets from Bitwarden via the bw CLI.

Prerequisites:
    pip install -r requirements.txt
    bw login (or bw login --apikey)
    export BW_SESSION=$(bw unlock --raw)

Usage:
    python bitwarden_client.py
"""

import json
import os
import subprocess
import sys


class BitwardenClient:
    """Thin wrapper around the Bitwarden CLI (`bw`)."""

    def __init__(self, session: str | None = None):
        self.session = session or os.environ.get("BW_SESSION", "")
        if not self.session:
            raise EnvironmentError(
                "BW_SESSION is not set. Run: export BW_SESSION=$(bw unlock --raw)"
            )
        self._verify_unlocked()

    def _run(self, args: list[str], check: bool = True) -> subprocess.CompletedProcess:
        """Run a bw CLI command with the session key."""
        cmd = ["bw"] + args + ["--session", self.session, "--nointeraction"]
        return subprocess.run(cmd, capture_output=True, text=True, check=check)

    def _verify_unlocked(self) -> None:
        result = self._run(["status"], check=False)
        if result.returncode != 0 or '"status":"unlocked"' not in result.stdout:
            raise PermissionError("Bitwarden vault is locked or session key is invalid.")

    def sync(self) -> None:
        """Sync vault from server."""
        self._run(["sync"])

    def get_password(self, item_name: str) -> str:
        """Retrieve the password for a login item by name."""
        result = self._run(["get", "password", item_name], check=False)
        if result.returncode != 0 or not result.stdout.strip():
            raise KeyError(f"Item '{item_name}' not found or has no password.")
        return result.stdout.strip()

    def get_username(self, item_name: str) -> str:
        """Retrieve the username for a login item by name."""
        result = self._run(["get", "username", item_name])
        return result.stdout.strip()

    def get_item(self, item_name: str) -> dict:
        """Retrieve the full item JSON by name."""
        result = self._run(["get", "item", item_name], check=False)
        if result.returncode != 0:
            raise KeyError(f"Item '{item_name}' not found.")
        return json.loads(result.stdout)

    def get_field(self, item_name: str, field_name: str) -> str:
        """Retrieve a custom field value from an item."""
        item = self.get_item(item_name)
        fields = item.get("fields") or []
        for field in fields:
            if field.get("name") == field_name:
                return field.get("value", "")
        raise KeyError(f"Field '{field_name}' not found in item '{item_name}'.")

    def get_note(self, item_name: str) -> str:
        """Retrieve the notes of an item (e.g. a certificate or config blob)."""
        result = self._run(["get", "notes", item_name])
        return result.stdout.strip()

    def list_items(self, search: str | None = None) -> list[dict]:
        """List vault items, optionally filtered by a search term."""
        args = ["list", "items"]
        if search:
            args += ["--search", search]
        result = self._run(args)
        return json.loads(result.stdout)


def demo() -> None:
    try:
        client = BitwardenClient()
    except (EnvironmentError, PermissionError) as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)

    # Search for items
    items = client.list_items(search="github")
    print(f"Found {len(items)} item(s) matching 'github'")

    if items:
        item = items[0]
        print(f"First match: {item.get('name')}")

        # Get password (don't print in real code)
        try:
            password = client.get_password(item["name"])
            print(f"Password retrieved (length: {len(password)})")
        except KeyError as e:
            print(f"Could not retrieve password: {e}")


if __name__ == "__main__":
    demo()
