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

          test = mkApp "test" [ pkgs.go pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make test
          '';

          # Race detector requires CGO — make test-race overrides CGO_ENABLED=0.
          test-race = mkApp "test-race" [ pkgs.go pkgs.gnumake ] ''
            make test-race
          '';

          test-cover = mkApp "test-cover" [ pkgs.go pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make test-cover
          '';

          test-e2e = mkApp "test-e2e" [ pkgs.go pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make test-e2e
          '';

          lint = mkApp "lint" [ pkgs.go pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make lint
          '';

          fmt = mkApp "fmt" [ pkgs.go pkgs.gnumake ] ''
            make fmt
          '';

          fmt-check = mkApp "fmt-check" [ pkgs.go pkgs.gnumake ] ''
            make fmt-check
          '';

          shellcheck = mkApp "shellcheck" [ pkgs.shellcheck pkgs.gnumake ] ''
            make shellcheck
          '';

          tidy = mkApp "tidy" [ pkgs.go pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make tidy
          '';

          govulncheck = mkApp "govulncheck" [ pkgs.govulncheck ] ''
            govulncheck ./...
          '';

          # Outputs text findings to the log. The job fails if gosec finds any issues.
          # Excluded rules:
          #   G304 — file inclusion via variable: intentional, reads user-supplied config/.env paths
          #   G104 — unhandled errors: project convention allows best-effort cleanup (slog.Warn)
          #   G204 — subprocess with variable: by design, this tool runs bw/vault with user args
          #   G706 — log injection via slog: slog is not susceptible to this injection vector
          #   G703 — path traversal: os.Remove calls are guarded by IsManaged()/isManagedKubeconfig()
          #   G115 — integer overflow: int(fd) safe (fds are small non-negative), uint32(pid) safe
          #          (Linux PID max 4194304 < 2^32), byte(padding) safe (PKCS7 padding 1-16)
          gosec = mkApp "gosec" [ pkgs.gosec ] ''
            gosec -exclude=G304,G104,G204,G706,G703,G115 -fmt text -stdout -verbose=text ./...
          '';

          clean = mkApp "clean" [ pkgs.gnumake ] ''
            make clean
          '';

          release = mkApp "release" [ pkgs.go pkgs.goreleaser pkgs.gnumake ] ''
            export CGO_ENABLED=0
            make release
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
            gnumake
          ] ++ [ envoke ];

          shellHook = ''
            export CGO_ENABLED=0
          '';
        };
      }
    );
}
