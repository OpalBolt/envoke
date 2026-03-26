"""
vault_client.py — Read and write secrets using HashiCorp Vault via hvac.

Prerequisites:
    pip install -r requirements.txt
    export VAULT_ADDR="https://vault.example.com:8200"
    export VAULT_TOKEN="<YOUR_VAULT_TOKEN>"  # or use vault login first

Usage:
    python vault_client.py
"""

import os
import sys
import hvac


def create_client() -> hvac.Client:
    """Create and authenticate a Vault client from environment variables."""
    vault_addr = os.environ.get("VAULT_ADDR", "http://127.0.0.1:8200")
    vault_token = os.environ.get("VAULT_TOKEN")

    client = hvac.Client(url=vault_addr, token=vault_token)

    if not client.is_authenticated():
        print("ERROR: Not authenticated to Vault.", file=sys.stderr)
        print("Set VAULT_ADDR and VAULT_TOKEN environment variables.", file=sys.stderr)
        sys.exit(1)

    return client


def write_secret(client: hvac.Client, path: str, data: dict) -> None:
    """Write a secret to Vault KV v2."""
    # path is relative to the KV mount, e.g. "myproject/db"
    client.secrets.kv.v2.create_or_update_secret(path=path, secret=data)
    print(f"✅ Secret written to: {path}")


def read_secret(client: hvac.Client, path: str) -> dict:
    """Read a secret from Vault KV v2. Returns the data dict."""
    response = client.secrets.kv.v2.read_secret_version(path=path, raise_on_deleted_version=True)
    return response["data"]["data"]


def read_field(client: hvac.Client, path: str, field: str) -> str:
    """Read a single field from a KV v2 secret."""
    data = read_secret(client, path)
    if field not in data:
        raise KeyError(f"Field '{field}' not found at path '{path}'")
    return data[field]


def list_secrets(client: hvac.Client, path: str) -> list[str]:
    """List secret keys at a path."""
    response = client.secrets.kv.v2.list_secrets(path=path)
    return response["data"]["keys"]


def delete_secret(client: hvac.Client, path: str) -> None:
    """Soft-delete the latest version of a secret."""
    client.secrets.kv.v2.delete_latest_version_of_secret(path=path)
    print(f"🗑️  Secret deleted (soft): {path}")


def demo() -> None:
    client = create_client()

    # Write
    write_secret(client, "myproject/demo", {
        "username": "app_user",
        "password": "<DEMO_PASSWORD>",
    })

    # Read
    data = read_secret(client, "myproject/demo")
    print(f"Read secret: username={data['username']}")

    # Read single field
    password = read_field(client, "myproject/demo", "password")
    print(f"Password field retrieved (length: {len(password)})")

    # List
    keys = list_secrets(client, "myproject/")
    print(f"Secrets at myproject/: {keys}")

    # Clean up
    delete_secret(client, "myproject/demo")


if __name__ == "__main__":
    demo()
