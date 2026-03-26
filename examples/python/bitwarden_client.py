"""
bitwarden_client.py — Retrieve secrets from Bitwarden Secrets Manager via the official SDK.

This example uses the Bitwarden Secrets Manager product, which is designed for
machine-to-machine access (CI/CD, applications). It uses access tokens — NOT
the BW_SESSION used by the bw CLI for the personal vault.

Prerequisites:
    pip install -r requirements.txt

    # In Bitwarden: Organisation → Secrets Manager → Service Accounts
    # Create a service account, generate an access token, note the org ID.
    export BWS_ACCESS_TOKEN="0.your-access-token..."
    export BWS_ORGANIZATION_ID="your-org-uuid"

Usage:
    python bitwarden_client.py

Docs: https://bitwarden.com/help/secrets-manager-overview/
SDK:  https://github.com/bitwarden/sdk-python
"""

import os
import sys

from bitwarden_sdk import BitwardenClient, client_settings_from_dict


def _make_client(
    api_url: str = "https://api.bitwarden.com",
    identity_url: str = "https://identity.bitwarden.com",
) -> BitwardenClient:
    """Create and authenticate a Bitwarden Secrets Manager client."""
    access_token = os.environ.get("BWS_ACCESS_TOKEN", "")
    if not access_token:
        raise EnvironmentError(
            "BWS_ACCESS_TOKEN is not set.\n"
            "Generate one under: Organisation → Secrets Manager → Service Accounts"
        )

    client = BitwardenClient(
        settings=client_settings_from_dict({
            "apiUrl": api_url,
            "identityUrl": identity_url,
            "userAgent": "bitwarden-sdk-python-example/1.0",
            "deviceType": "SDK",
        })
    )

    # state_file persists the auth state between calls (optional, pass "" to disable)
    state_file = os.environ.get("BWS_STATE_FILE", "")
    client.auth().login_access_token(access_token, state_file)
    return client


def get_secret_by_id(client: BitwardenClient, secret_id: str) -> str:
    """Retrieve a secret value by its UUID."""
    response = client.secrets().get(secret_id)
    return response.data.value


def get_secret_by_key(client: BitwardenClient, organization_id: str, key: str) -> str:
    """Retrieve a secret value by its human-readable key name.

    Requires listing all secrets first. Prefer get_secret_by_id when the
    UUID is known — it avoids the extra API call.
    """
    response = client.secrets().list(organization_id)
    for summary in response.data.data:
        if summary.key == key:
            return get_secret_by_id(client, summary.id)
    raise KeyError(f"Secret with key '{key}' not found in organisation.")


def list_secrets(client: BitwardenClient, organization_id: str) -> list[dict]:
    """Return a list of {id, key} dicts for all secrets in the organisation.

    Values are NOT included — fetch individually with get_secret_by_id.
    """
    response = client.secrets().list(organization_id)
    return [{"id": s.id, "key": s.key} for s in response.data.data]


def create_secret(
    client: BitwardenClient,
    organization_id: str,
    key: str,
    value: str,
    note: str = "",
    project_ids: list[str] | None = None,
) -> str:
    """Create a new secret and return its UUID."""
    response = client.secrets().create(
        key=key,
        value=value,
        note=note,
        organization_id=organization_id,
        project_ids=project_ids or [],
    )
    return response.data.id


def update_secret(
    client: BitwardenClient,
    secret_id: str,
    organization_id: str,
    key: str,
    value: str,
    note: str = "",
    project_ids: list[str] | None = None,
) -> None:
    """Update an existing secret by UUID."""
    client.secrets().update(
        id=secret_id,
        key=key,
        value=value,
        note=note,
        organization_id=organization_id,
        project_ids=project_ids or [],
    )


def delete_secrets(client: BitwardenClient, secret_ids: list[str]) -> None:
    """Delete one or more secrets by UUID list."""
    client.secrets().delete(secret_ids)


def demo() -> None:
    org_id = os.environ.get("BWS_ORGANIZATION_ID", "")
    if not org_id:
        sys.exit(
            "ERROR: BWS_ORGANIZATION_ID is not set.\n"
            "Find it under: Organisation Settings in the Bitwarden web app."
        )

    try:
        client = _make_client()
    except EnvironmentError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)

    # List all secrets (keys only — values are not included in the listing)
    secrets = list_secrets(client, org_id)
    print(f"Organisation has {len(secrets)} secret(s):")
    for s in secrets:
        print(f"  {s['key']}  ({s['id']})")

    if secrets:
        # Retrieve the full value for the first secret
        first = secrets[0]
        value = get_secret_by_id(client, first["id"])
        print(f"\nFirst secret '{first['key']}' value length: {len(value)}")


if __name__ == "__main__":
    demo()
