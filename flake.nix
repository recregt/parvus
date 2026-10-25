# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

{
  description = "Parvus";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forEach = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      sourceOf =
        pkgs:
        pkgs.lib.fileset.toSource {
          root = ./.;
          fileset = pkgs.lib.fileset.unions [
            ./go.mod
            ./go.sum
            ./.golangci.yml
            ./internal
            (pkgs.lib.fileset.maybeMissing ./cmd)
          ];
        };

      vendorHash = "sha256-m6w28qCHfZl8v8F4/QIfCMCwJeEGQK9Ld5CwQOVDD1M=";

      goCheck =
        pkgs: attrs:
        pkgs.buildGoModule (
          {
            pname = "parvus-check";
            version = "0";
            src = sourceOf pkgs;
            inherit vendorHash;
            doCheck = false;
            installPhase = "touch $out";
          }
          // attrs
        );
    in
    {
      packages = forEach (pkgs: {
        default = pkgs.buildGoModule {
          pname = "parvus";
          version = "0";
          src = sourceOf pkgs;
          inherit vendorHash;
          subPackages = [ "cmd/parvus" ];
          env.CGO_ENABLED = "0";
          ldflags = [
            "-s"
            "-w"
          ];
          doCheck = false;
          meta = {
            mainProgram = "parvus";
            license = pkgs.lib.licenses.mpl20;
          };
        };
      });

      checks = forEach (pkgs: {
        tests = goCheck pkgs {
          doCheck = true;
          env.CGO_ENABLED = "1";
          checkFlags = [
            "-race"
            "-count=1"
          ];
        };

        lint = goCheck pkgs {
          nativeBuildInputs = [ pkgs.golangci-lint ];
          buildPhase = ''
            export HOME=$TMPDIR
            golangci-lint run ./...
            golangci-lint fmt --diff ./...
          '';
        };

        license = pkgs.runCommand "parvus-license" { nativeBuildInputs = [ pkgs.addlicense ]; } ''
          cd ${./.}
          addlicense -check \
            -ignore 'LICENSE' -ignore '**/*.mod' -ignore '**/*.sum' -ignore 'flake.lock' \
            -ignore '.git/**' .
          touch $out
        '';
      });

      devShells = forEach (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.golangci-lint
            pkgs.govulncheck
            pkgs.addlicense
            pkgs.lefthook
          ];
        };
      });
    };
}
