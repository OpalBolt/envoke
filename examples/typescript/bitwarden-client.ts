/**
 * bitwarden-client.ts — Retrieve secrets from Bitwarden Secrets Manager via the official SDK.
 *
 * This example uses the Bitwarden Secrets Manager product, designed for
 * machine-to-machine access (CI/CD, applications). It uses access tokens —
 * NOT the BW_SESSION used by the bw CLI for the personal vault.
 *
 * Prerequisites:
 *   npm install
 *
 *   # In Bitwarden: Organisation → Secrets Manager → Service Accounts
 *   # Create a service account, generate an access token, note the org ID.
 *   export BWS_ACCESS_TOKEN="0.your-access-token..."
 *   export BWS_ORGANIZATION_ID="your-org-uuid"
 *
 * Usage:
 *   npx ts-node bitwarden-client.ts
 *
 * Docs: https://bitwarden.com/help/secrets-manager-overview/
 * SDK:  https://github.com/bitwarden/sdk (languages/js)
 */

import * as bitwarden from "@bitwarden/sdk-napi";

interface SecretSummary {
  id: string;
  key: string;
}

class BitwardenSecretsClient {
  private client: bitwarden.BitwardenClient;
  private readonly orgId: string;

  constructor(client: bitwarden.BitwardenClient, orgId: string) {
    this.client = client;
    this.orgId = orgId;
  }

  static async create(
    apiUrl = "https://api.bitwarden.com",
    identityUrl = "https://identity.bitwarden.com",
  ): Promise<BitwardenSecretsClient> {
    const accessToken = process.env.BWS_ACCESS_TOKEN;
    if (!accessToken) {
      throw new Error(
        "BWS_ACCESS_TOKEN is not set.\n" +
          "Generate one under: Organisation → Secrets Manager → Service Accounts",
      );
    }

    const orgId = process.env.BWS_ORGANIZATION_ID;
    if (!orgId) {
      throw new Error(
        "BWS_ORGANIZATION_ID is not set.\n" +
          "Find it under: Organisation Settings in the Bitwarden web app.",
      );
    }

    const client = new bitwarden.BitwardenClient({
      apiUrl,
      identityUrl,
      userAgent: "bitwarden-sdk-ts-example/1.0",
      deviceType: bitwarden.DeviceType.SDK,
    });

    // stateFile persists auth state between calls; pass "" to disable.
    const stateFile = process.env.BWS_STATE_FILE ?? "";
    await client.auth().loginAccessToken(accessToken, stateFile);

    return new BitwardenSecretsClient(client, orgId);
  }

  /** Retrieve a secret value by its UUID. */
  async getSecretById(secretId: string): Promise<string> {
    const response = await this.client.secrets().get(secretId);
    return response.value;
  }

  /**
   * Retrieve a secret value by its human-readable key name.
   * Prefer getSecretById when the UUID is known — it avoids an extra API call.
   */
  async getSecretByKey(key: string): Promise<string> {
    const secrets = await this.listSecrets();
    const match = secrets.find((s) => s.key === key);
    if (!match) {
      throw new Error(`Secret with key "${key}" not found in organisation.`);
    }
    return this.getSecretById(match.id);
  }

  /** List all secrets (key + ID only; values are NOT included). */
  async listSecrets(): Promise<SecretSummary[]> {
    const response = await this.client.secrets().list(this.orgId);
    return response.data.map((s) => ({ id: s.id, key: s.key }));
  }

  /** Create a new secret and return its UUID. */
  async createSecret(
    key: string,
    value: string,
    note = "",
    projectIds: string[] = [],
  ): Promise<string> {
    const response = await this.client
      .secrets()
      .create(this.orgId, key, value, note, projectIds);
    return response.id;
  }

  /** Update an existing secret by UUID. */
  async updateSecret(
    secretId: string,
    key: string,
    value: string,
    note = "",
    projectIds: string[] = [],
  ): Promise<void> {
    await this.client
      .secrets()
      .update(this.orgId, secretId, key, value, note, projectIds);
  }

  /** Delete one or more secrets by UUID list. */
  async deleteSecrets(secretIds: string[]): Promise<void> {
    await this.client.secrets().delete(secretIds);
  }
}

async function main(): Promise<void> {
  let bws: BitwardenSecretsClient;
  try {
    bws = await BitwardenSecretsClient.create();
  } catch (err) {
    console.error(`ERROR: ${(err as Error).message}`);
    process.exit(1);
  }

  // List all secrets (keys only — values not included in listing)
  const secrets = await bws.listSecrets();
  console.log(`Organisation has ${secrets.length} secret(s):`);
  for (const s of secrets) {
    console.log(`  ${s.key}  (${s.id})`);
  }

  if (secrets.length > 0) {
    const first = secrets[0];
    try {
      const value = await bws.getSecretById(first.id);
      console.log(`\nFirst secret '${first.key}' value length: ${value.length}`);
    } catch (err) {
      console.log(`Could not retrieve secret: ${(err as Error).message}`);
    }
  }
}

main();
