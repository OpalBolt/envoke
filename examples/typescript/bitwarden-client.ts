/**
 * bitwarden-client.ts — Retrieve secrets from Bitwarden via the bw CLI.
 *
 * Prerequisites:
 *   npm install
 *   bw login (or bw login --apikey)
 *   export BW_SESSION=$(bw unlock --raw)
 *
 * Usage:
 *   npx ts-node bitwarden-client.ts
 */

import { execSync, ExecSyncOptionsWithStringEncoding } from "child_process";

interface BitwardenLogin {
  username?: string;
  password?: string;
}

interface BitwardenField {
  name: string;
  value: string;
  type: number;
}

interface BitwardenItem {
  id: string;
  name: string;
  notes?: string;
  login?: BitwardenLogin;
  fields?: BitwardenField[];
}

class BitwardenClient {
  private readonly session: string;

  constructor(session?: string) {
    this.session = session ?? process.env.BW_SESSION ?? "";
    if (!this.session) {
      throw new Error(
        "BW_SESSION is not set — run: export BW_SESSION=$(bw unlock --raw)"
      );
    }
    this.verifyUnlocked();
  }

  private run(args: string[]): string {
    const opts: ExecSyncOptionsWithStringEncoding = {
      encoding: "utf-8",
      env: { ...process.env, BW_SESSION: this.session },
      stdio: ["pipe", "pipe", "pipe"],
    };
    try {
      return execSync(
        ["bw", ...args, "--session", this.session, "--nointeraction"].join(" "),
        opts
      ).trim();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(`bw ${args[0]} failed: ${msg}`);
    }
  }

  private verifyUnlocked(): void {
    const status = this.run(["status"]);
    if (!status.includes('"status":"unlocked"')) {
      throw new Error("Bitwarden vault is locked or session key is invalid.");
    }
  }

  sync(): void {
    this.run(["sync"]);
  }

  getItem(itemName: string): BitwardenItem {
    const json = this.run(["get", "item", `"${itemName}"`]);
    return JSON.parse(json) as BitwardenItem;
  }

  getPassword(itemName: string): string {
    const result = this.run(["get", "password", `"${itemName}"`]);
    if (!result) {
      throw new Error(`Password for "${itemName}" not found.`);
    }
    return result;
  }

  getUsername(itemName: string): string {
    return this.run(["get", "username", `"${itemName}"`]);
  }

  getField(itemName: string, fieldName: string): string {
    const item = this.getItem(itemName);
    const field = item.fields?.find((f) => f.name === fieldName);
    if (!field) {
      throw new Error(`Field "${fieldName}" not found in item "${itemName}".`);
    }
    return field.value;
  }

  getNote(itemName: string): string {
    return this.run(["get", "notes", `"${itemName}"`]);
  }

  listItems(search?: string): BitwardenItem[] {
    const args = ["list", "items"];
    if (search) args.push("--search", search);
    const json = this.run(args);
    return JSON.parse(json) as BitwardenItem[];
  }
}

function main(): void {
  let client: BitwardenClient;
  try {
    client = new BitwardenClient();
  } catch (err) {
    console.error(`ERROR: ${(err as Error).message}`);
    process.exit(1);
  }

  const items = client.listItems("github");
  console.log(`Found ${items.length} item(s) matching 'github'`);

  if (items.length > 0) {
    const item = items[0];
    console.log(`First match: ${item.name}`);

    try {
      const password = client.getPassword(item.name);
      console.log(`Password retrieved (length: ${password.length})`);
    } catch (err) {
      console.log(`Could not retrieve password: ${(err as Error).message}`);
    }
  }
}

main();
