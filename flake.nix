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

        python = pkgs.python3.withPackages (ps: [
          ps.hvac
        ]);
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            # Secret manager CLIs
            pkgs.bitwarden-cli
            pkgs.vault

            # Python
            python
            pkgs.python3Packages.pip

            # Go
            pkgs.go

            # TypeScript / Node.js
            pkgs.nodejs_24
            pkgs.typescript

            # Utilities
            pkgs.git
            pkgs.jq
          ];

          shellHook = ''
            echo "secrets dev shell ready – bw, vault, python, go, node/ts available"
          '';
        };
      });
}
