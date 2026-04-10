# Nix integration

envoke works in Nix environments and detects them automatically. When `IN_NIX_SHELL` is set, `envoke resolve` and `renv resolve` skip emitting their EXIT trap, since Nix manages the shell lifecycle.

## Running envoke in a nix-shell

If you launch a `nix-shell` or `nix develop` session that has envoke on `PATH`, you can load secrets normally:

```bash
nix develop
envoke resolve .env
```

The secrets are available for the duration of the nix-shell session. When you exit, they are gone with the shell.

## Using envoke inside a nix flake devShell

You can install envoke as part of your project's development shell:

```nix
# flake.nix
{
  inputs = {
    nixpkgs.url     = "github:NixOS/nixpkgs/nixos-unstable";
    envoke.url      = "github:eficode/envoke";
  };

  outputs = { nixpkgs, envoke, ... }:
    let
      system = "x86_64-linux";
      pkgs   = nixpkgs.legacyPackages.${system};
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = [
          envoke.packages.${system}.envoke
          pkgs.bitwarden-cli
          pkgs.vault
          pkgs.kubectl
        ];

        shellHook = ''
          eval "$(envoke shell-init)"
        '';
      };
    };
}
```

The `shellHook` runs shell-init so that `envoke resolve`, auto-unload on exit, and lock detection all work inside the dev shell.

## Building envoke with Nix

```bash
nix build                 # builds envoke (default package)
nix run .                 # run envoke directly
```

## Development shell

The repository's own dev shell includes Go tools, goreleaser, `bw`, `vault`, and `envoke`:

```bash
nix develop
make build    # outputs to bin/envoke
make test
make lint
```

## Available Nix apps

```bash
nix run .#test         # run Go tests
nix run .#test-race    # run tests with race detector
nix run .#lint         # run go vet
nix run .#fmt-check    # check gofmt formatting
nix run .#shellcheck   # lint shell scripts in snippets/
```

## Notes

- The `IN_NIX_SHELL` variable is set by `nix-shell` but **not** by `nix develop`. If you want the same no-trap behaviour inside `nix develop`, set `IN_NIX_SHELL=1` in your `shellHook`, or rely on the EXIT trap from shell-init.
- For NixOS system-wide installation, add `envoke.packages.${system}.envoke` to `environment.systemPackages`.
- The `vendorHash` in `flake.nix` must be updated after any `go.mod` change. Set it to `pkgs.lib.fakeHash`, run `nix build`, and replace with the hash printed in the error.
