/**
 * vault-client.ts — Read and write secrets using HashiCorp Vault (node-vault).
 *
 * Prerequisites:
 *   npm install
 *   export VAULT_ADDR="https://vault.example.com:8200"
 *   export VAULT_TOKEN="<YOUR_VAULT_TOKEN>"
 *
 * Usage:
 *   npx ts-node vault-client.ts
 */

import nodeVault from "node-vault";

interface VaultClient {
  write(path: string, data: Record<string, unknown>): Promise<unknown>;
  read(path: string): Promise<{ data: { data: Record<string, string> } }>;
  list(path: string): Promise<{ data: { keys: string[] } }>;
  delete(path: string): Promise<unknown>;
}

function createClient(): VaultClient {
  const addr = process.env.VAULT_ADDR ?? "http://127.0.0.1:8200";
  const token = process.env.VAULT_TOKEN;

  if (!token) {
    console.error("ERROR: VAULT_TOKEN is not set.");
    process.exit(1);
  }

  return nodeVault({ apiVersion: "v1", endpoint: addr, token }) as VaultClient;
}

/**
 * Write a secret to Vault KV v2.
 * Path is relative to the KV mount, e.g. "myproject/db".
 */
async function writeSecret(
  client: VaultClient,
  mountPath: string,
  secretPath: string,
  data: Record<string, string>
): Promise<void> {
  await client.write(`${mountPath}/data/${secretPath}`, { data });
  console.log(`✅ Secret written to: ${mountPath}/data/${secretPath}`);
}

/**
 * Read a KV v2 secret and return its data object.
 */
async function readSecret(
  client: VaultClient,
  mountPath: string,
  secretPath: string
): Promise<Record<string, string>> {
  const response = await client.read(`${mountPath}/data/${secretPath}`);
  return response.data.data;
}

/**
 * Read a single field from a KV v2 secret.
 */
async function readField(
  client: VaultClient,
  mountPath: string,
  secretPath: string,
  field: string
): Promise<string> {
  const data = await readSecret(client, mountPath, secretPath);
  if (!(field in data)) {
    throw new Error(`Field "${field}" not found at ${mountPath}/${secretPath}`);
  }
  return data[field];
}

/**
 * List secret keys at a KV v2 path.
 */
async function listSecrets(
  client: VaultClient,
  mountPath: string,
  prefix: string
): Promise<string[]> {
  const response = await client.list(`${mountPath}/metadata/${prefix}`);
  return response.data.keys;
}

/**
 * Soft-delete the latest version of a KV v2 secret.
 */
async function deleteSecret(
  client: VaultClient,
  mountPath: string,
  secretPath: string
): Promise<void> {
  await client.delete(`${mountPath}/data/${secretPath}`);
  console.log(`🗑️  Secret deleted: ${mountPath}/${secretPath}`);
}

async function main(): Promise<void> {
  const client = createClient();
  const mount = "secret";
  const path = "myproject/demo";

  await writeSecret(client, mount, path, {
    username: "app_user",
    password: "<DEMO_PASSWORD>",
  });

  const data = await readSecret(client, mount, path);
  console.log(`Read secret: username=${data.username}`);

  const password = await readField(client, mount, path, "password");
  console.log(`Password field retrieved (length: ${password.length})`);

  const keys = await listSecrets(client, mount, "myproject/");
  console.log(`Secrets at myproject/: ${keys.join(", ")}`);

  await deleteSecret(client, mount, path);
}

main().catch((err) => {
  console.error("Fatal error:", err.message);
  process.exit(1);
});
