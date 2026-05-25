{
  description = "envoke — unified secret environment loader (env vars and kubeconfigs)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, gomod2nix }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        inherit (gomod2nix.legacyPackages.${system}) mkGoEnv buildGoApplication;

        go = pkgs.go_1_25;

        # Single source of truth for the version number — kept in sync with git tags.
        # `git tag v0.2.0 && echo -n 0.2.0 > VERSION` is the release workflow.
        releaseVersion = builtins.replaceStrings [ "\n" " " ] [ "" "" ] (builtins.readFile ./VERSION);
        versionPkg = "github.com/opalbolt/envoke/internal/version";

        # self.shortRev is the 7-char git commit hash; falls back to "dirty" when the
        # working tree has uncommitted changes (Nix won't set rev on a dirty tree).
        commitHash = self.shortRev or "dirty";
        # Dev builds embed the commit so `envoke --version` shows e.g. "0.1.0-dev+aeda2e9".
        # Goreleaser handles tagged release builds separately (see .goreleaser.yaml).
        nixVersion = "${releaseVersion}-dev+${commitHash}";
        # self.lastModifiedDate is "YYYYMMDDHHmmss"; reformat to ISO 8601 to match `make build` output.
        buildDate =
          let raw = self.lastModifiedDate or "unknown"; in
          if raw == "unknown" then "unknown"
          else
            "${builtins.substring 0 4 raw}-${builtins.substring 4 2 raw}-${builtins.substring 6 2 raw}T${builtins.substring 8 2 raw}:${builtins.substring 10 2 raw}:${builtins.substring 12 2 raw}Z";

        envoke = buildGoApplication {
          pname = "envoke";
          version = releaseVersion;
          src = ./.;
          modules = ./gomod2nix.toml;
          subPackages = [ "cmd/envoke" ];
          inherit go;
          ldflags = [
            "-s" "-w"
            "-X ${versionPkg}.Version=${nixVersion}"
            "-X ${versionPkg}.Commit=${commitHash}"
            "-X ${versionPkg}.BuildDate=${buildDate}"
          ];
        };
      in
      {
        # `nix build` / `nix profile install` / `nix run`
        packages = {
          inherit envoke;
          default = envoke;
        };

        apps = {
          envoke = flake-utils.lib.mkApp { drv = envoke; };
          default = flake-utils.lib.mkApp { drv = envoke; };
        };

        # `nix develop` — provides the full Go toolchain and dev tools.
        # Run make targets directly: make build, make test-race, make lint, etc.
        devShells.default = pkgs.mkShell {
          packages = [
            go
            gomod2nix.packages.${system}.default
          ] ++ (with pkgs; [
            gotools       # goimports, etc.
            gopls
            go-tools      # staticcheck
            goreleaser
            govulncheck
            gosec
            gnumake
            shellcheck
          ]);

          shellHook = ''
            export CGO_ENABLED=0
            export PATH="$PWD/bin:$PATH"
            make build
            eval "$(envoke shell-init)"
          '';
        };
      }
    );
}
