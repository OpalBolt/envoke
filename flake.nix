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

        # Single source of truth for the version number — kept in sync with git tags.
        # `git tag v0.2.0 && echo -n 0.2.0 > VERSION` is the release workflow.
        releaseVersion = builtins.replaceStrings [ "\n" " " ] [ "" "" ] (builtins.readFile ./VERSION);
        versionPkg = "github.com/eficode/secure-handling-of-secrets/internal/version";

        # self.shortRev is the 7-char git commit hash; falls back to "dirty" when the
        # working tree has uncommitted changes (Nix won't set rev on a dirty tree).
        commitHash = self.shortRev or "dirty";
        # Dev builds embed the commit so `renv --version` shows e.g. "0.1.0-dev+aeda2e9".
        # Goreleaser handles tagged release builds separately (see .goreleaser.yaml).
        nixVersion = "${releaseVersion}-dev+${commitHash}";
        buildDate = self.lastModifiedDate or "unknown";

        common = {
          src = ./.;
          vendorHash = "sha256-toMUBMJ/Ky7HglGwhhLVHN+FzUWihwNfKS/XnGIe9aE=";
        };

        renv = pkgs.buildGoModule (common // {
          pname = "renv";
          version = releaseVersion;
          subPackages = [ "cmd/renv" ];
          ldflags = [
            "-s" "-w"
            "-X ${versionPkg}.Version=${nixVersion}"
            "-X ${versionPkg}.Commit=${commitHash}"
            "-X ${versionPkg}.BuildDate=${buildDate}"
          ];
        });

        kctx = pkgs.buildGoModule (common // {
          pname = "kctx";
          version = releaseVersion;
          subPackages = [ "cmd/kctx" ];
          ldflags = [
            "-s" "-w"
            "-X ${versionPkg}.Version=${nixVersion}"
            "-X ${versionPkg}.Commit=${commitHash}"
            "-X ${versionPkg}.BuildDate=${buildDate}"
          ];
        });

        # Default package builds both binaries
        all = pkgs.buildGoModule (common // {
          pname = "secure-handling-of-secrets";
          version = releaseVersion;
          subPackages = [ "cmd/renv" "cmd/kctx" ];
          ldflags = [
            "-s" "-w"
            "-X ${versionPkg}.Version=${nixVersion}"
            "-X ${versionPkg}.Commit=${commitHash}"
            "-X ${versionPkg}.BuildDate=${buildDate}"
          ];
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

          fmt-check = mkApp "fmt-check" [ pkgs.go ] ''
            unformatted=$(gofmt -l .)
            if [ -n "$unformatted" ]; then
              echo "The following files need formatting:"
              echo "$unformatted"
              exit 1
            fi
          '';

          shellcheck = mkApp "shellcheck" [ pkgs.shellcheck ] ''
            find snippets -name '*.sh' -print0 | xargs -0 shellcheck --severity=warning
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
          ] ++ [ renv kctx ];

          shellHook = ''
            export CGO_ENABLED=0
          '';
        };
      }
    );
}
