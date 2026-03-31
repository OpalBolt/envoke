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

        # Wrap a shell script as a runnable flake app (nix run .#<name>).
        mkApp = name: runtimeInputs: text:
          let
            drv = pkgs.writeShellApplication { inherit name runtimeInputs text; };
          in
          { type = "app"; program = "${drv}/bin/${name}"; };

        # Shared build attributes — dependencies are fetched by Nix (vendor/ is
        # not committed).  CGO_ENABLED=0 is enforced via devShell; buildGoModule
        # pure-Go builds don't link C so CGO is irrelevant at the Nix derivation
        # level.  trimpath is true by default in nixpkgs buildGoModule.
        common = {
          src = ./.;
          vendorHash = "sha256-toMUBMJ/Ky7HglGwhhLVHN+FzUWihwNfKS/XnGIe9aE=";
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

          test = mkApp "test" [ pkgs.go ] ''
            export CGO_ENABLED=0
            go test ./...
          '';

          # Race detector requires CGO — do NOT set CGO_ENABLED=0 here.
          test-race = mkApp "test-race" [ pkgs.go ] ''
            go test -race ./...
          '';

          test-cover = mkApp "test-cover" [ pkgs.go ] ''
            export CGO_ENABLED=0
            go test -coverprofile=coverage.out ./...
            go tool cover -html=coverage.out -o coverage.html
          '';

          lint = mkApp "lint" [ pkgs.go ] ''
            export CGO_ENABLED=0
            go vet ./...
          '';

          fmt = mkApp "fmt" [ pkgs.go ] ''
            gofmt -w .
          '';

          tidy = mkApp "tidy" [ pkgs.go ] ''
            export CGO_ENABLED=0
            go mod tidy
            go mod verify
          '';

          clean = mkApp "clean" [ ] ''
            rm -rf bin coverage.out coverage.html
          '';

          release = mkApp "release" [ pkgs.go pkgs.goreleaser ] ''
            export CGO_ENABLED=0
            goreleaser build --snapshot --clean
          '';
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gotools  # goimports, etc.
            gopls
            go-tools  # staticcheck
            goreleaser
          ];

          shellHook = ''
            export CGO_ENABLED=0
          '';
        };
      }
    );
}
