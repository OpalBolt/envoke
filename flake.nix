{
  description = "envoke — unified secret environment loader (env vars and kubeconfigs)";

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
        # self.lastModifiedDate is "YYYYMMDDHHmmss"; reformat to ISO 8601 to match `make build` output.
        buildDate =
          let raw = self.lastModifiedDate or "unknown"; in
          if raw == "unknown" then "unknown"
          else
            "${builtins.substring 0 4 raw}-${builtins.substring 4 2 raw}-${builtins.substring 6 2 raw}T${builtins.substring 8 2 raw}:${builtins.substring 10 2 raw}:${builtins.substring 12 2 raw}Z";

        common = {
          src = ./.;
          vendorHash = "sha256-U5ObZjq8TzaBKP8AbmoX/3Ylt5feuNMXM7JfGXF2NyA=";
        };

        envoke = pkgs.buildGoModule (common // {
          pname = "envoke";
          version = releaseVersion;
          subPackages = [ "cmd/envoke" ];
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
          inherit envoke;
          default = envoke;
        };

        apps = {
          envoke = flake-utils.lib.mkApp { drv = envoke; };
          # nix run → envoke
          default = flake-utils.lib.mkApp { drv = envoke; };

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

          govulncheck = mkApp "govulncheck" [ pkgs.govulncheck ] ''
            govulncheck ./...
          '';

          # Outputs text findings to the log. SARIF upload requires GHAS which
          # is not available. The job fails if gosec finds any issues.
          gosec = mkApp "gosec" [ pkgs.gosec ] ''
            gosec -exclude=G304 -fmt text -stdout -verbose=text ./...
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
            govulncheck
            gosec
          ] ++ [ envoke ];

          shellHook = ''
            export CGO_ENABLED=0
          '';
        };
      }
    );
}
