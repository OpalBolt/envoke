{
  description = "secure-handling-of-secrets — renv and kctx CLI tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Shared build attributes — vendor/ dir is committed so no hash needed.
        # CGO_ENABLED=0 is enforced via devShell; buildGoModule pure-Go builds
        # don't link C so CGO is irrelevant at the Nix derivation level.
        # trimpath is true by default in nixpkgs buildGoModule.
        common = {
          src = ./.;
          vendorHash = null;
        };

        renv = pkgs.buildGoModule (common // {
          pname = "renv";
          version = "0.1.0";
          subPackages = [ "cmd/renv" ];
        });

        kctx = pkgs.buildGoModule (common // {
          pname = "kctx";
          version = "0.1.0";
          subPackages = [ "cmd/kctx" ];
        });

        # Default package builds both binaries
        all = pkgs.buildGoModule (common // {
          pname = "secure-handling-of-secrets";
          version = "0.1.0";
          subPackages = [ "cmd/renv" "cmd/kctx" ];
        });
      in
      {
        packages = {
          inherit renv kctx;
          default = all;
        };

        apps = {
          renv = flake-utils.lib.mkApp { drv = renv; };
          kctx = flake-utils.lib.mkApp { drv = kctx; };
          # nix run → renv
          default = flake-utils.lib.mkApp { drv = renv; };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gotools  # goimports, etc.
            gopls
            go-tools # staticcheck
          ];

          shellHook = ''
            export CGO_ENABLED=0
          '';
        };
      }
    );
}
