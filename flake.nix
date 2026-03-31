{
  description = "Secure handling of secrets – dev environment";

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
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            # Go toolchain
            pkgs.go
            pkgs.gopls
            pkgs.go-tools        # staticcheck
            pkgs.gotools          # goimports, gofmt etc.
            pkgs.goreleaser
            pkgs.gnumake

            # Secret manager CLIs (for integration tests)
            pkgs.bitwarden-cli
            pkgs.vault

            # Utilities
            pkgs.git
            pkgs.jq
          ];

          env.NODE_OPTIONS = "--no-deprecation";

          shellHook = ''
            echo "secrets dev shell ready – bw, vault, go, goreleaser available"
          '';
        };
      });
}
