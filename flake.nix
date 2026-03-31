{
  description = "Secure handling of secrets – renv and kctx Go CLI tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };

        # Shared buildGoModule arguments
        commonArgs = {
          src = ./.;
          # vendor/ directory is committed — nix uses it directly (no network needed)
          vendorHash = null;
          CGO_ENABLED = "0";
          ldflags = [ "-trimpath" "-s" "-w" ];
        };

        renv = pkgs.buildGoModule (commonArgs // {
          pname = "renv";
          version = "dev";
          subPackages = [ "cmd/renv" ];
          meta = with pkgs.lib; {
            description = "Resolve bw:// and vault:// secret refs in .env and YAML files";
            license = licenses.mit;
            mainProgram = "renv";
          };
        });

        kctx = pkgs.buildGoModule (commonArgs // {
          pname = "kctx";
          version = "dev";
          subPackages = [ "cmd/kctx" ];
          meta = with pkgs.lib; {
            description = "Ephemeral kubeconfig switching via Vault or Bitwarden";
            license = licenses.mit;
            mainProgram = "kctx";
          };
        });
      in
      {
        # nix build .#renv  →  builds renv binary
        # nix build .#kctx  →  builds kctx binary
        # nix build          →  builds both (default = combined package)
        packages = {
          inherit renv kctx;
          default = pkgs.symlinkJoin {
            name = "secure-handling-of-secrets";
            paths = [ renv kctx ];
          };
        };

        # nix develop  →  full dev environment
        devShells.default = pkgs.mkShell {
          packages = [
            # Go toolchain
            pkgs.go
            pkgs.gopls
            pkgs.go-tools          # staticcheck
            pkgs.gotools            # goimports, gofmt etc.
            pkgs.goreleaser
            pkgs.gnumake

            # Secret manager CLIs (for integration tests)
            pkgs.bitwarden-cli
            pkgs.vault

            # Utilities
            pkgs.git
            pkgs.jq
          ];

          # Suppress Node.js deprecation warnings from bitwarden-cli (Electron/inquirer)
          env.NODE_OPTIONS = "--no-deprecation";

          shellHook = ''
            echo "secure-handling-of-secrets dev shell ready"
            echo "  go $(go version | cut -d' ' -f3)  •  bw  •  vault  •  goreleaser"
          '';
        };
      });
}
