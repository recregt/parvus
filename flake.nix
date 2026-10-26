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
            ./cmd
            ./examples
            ./e2e
          ];
        };

      vendorHash = "sha256-uiTJVRnKnnlmhUe4cN0eygLpIBJ3Cb5Kam6/KLMvU2k=";

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

      docsThemes =
        pkgs:
        pkgs.linkFarm "parvus-docs-themes" [
          {
            name = "hextra";
            path = pkgs.fetchFromGitHub {
              owner = "imfing";
              repo = "hextra";
              rev = "v0.13.0";
              hash = "sha256-7wS2xUnpSkWfc0aJ9YBc7VasXZDS0m/xX3F0+cbZzeA=";
            };
          }
        ];

      docsSite =
        pkgs:
        pkgs.runCommand "parvus-docs" { nativeBuildInputs = [ pkgs.hugo ]; } ''
          cp -r ${./docs} $TMPDIR/docs
          chmod -R u+w $TMPDIR/docs
          hugo --source $TMPDIR/docs --themesDir ${docsThemes pkgs} \
            --cacheDir $TMPDIR/hugo-cache --destination $out --minify --panicOnWarning
        '';

      goProgram =
        pkgs: name: path:
        pkgs.buildGoModule {
          pname = name;
          version = "0";
          src = sourceOf pkgs;
          inherit vendorHash;
          subPackages = [ path ];
          env.CGO_ENABLED = "0";
          ldflags = [
            "-s"
            "-w"
          ];
          doCheck = false;
          meta = {
            mainProgram = name;
            license = pkgs.lib.licenses.mpl20;
          };
        };
    in
    {
      packages = forEach (pkgs: {
        default = goProgram pkgs "parvus" "cmd/parvus";
        docs = docsSite pkgs;
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

        postgres = goCheck pkgs {
          doCheck = true;
          env.CGO_ENABLED = "1";
          nativeCheckInputs = [ pkgs.postgresql ];
          checkPhase = ''
            runHook preCheck
            export PGDATA=$TMPDIR/postgres
            initdb --auth=trust --username=parvus >/dev/null
            pg_ctl --wait --log=$TMPDIR/postgres.log \
              --options="-c listen_addresses= -c unix_socket_directories=$TMPDIR" start
            PARVUS_TEST_POSTGRES="host=$TMPDIR user=parvus" \
              go test -tags postgres -race -count=1 ./internal/store/postgres
            pg_ctl --wait stop
            runHook postCheck
          '';
        };

        docs = docsSite pkgs;

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
            -ignore '**/uv.lock' -ignore '**/.python-version' -ignore '**/.venv/**' \
            -ignore 'docs/assets/vendor/**' \
            -ignore '.git/**' .
          touch $out
        '';

        packages-lint =
          pkgs.runCommand "parvus-packages-lint"
            {
              nativeBuildInputs = [ pkgs.ruff ];
              RUFF_NO_CACHE = "true";
            }
            ''
              cd ${./e2e/packages}
              ruff check .
              ruff format --check .
              touch $out
            '';
      }
      // pkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
        browser =
          pkgs.runCommand "parvus-browser"
            {
              nativeBuildInputs = [
                (goProgram pkgs "parvus" "cmd/parvus")
                (goProgram pkgs "relay-go" "examples/relay-go")
                pkgs.nodejs
                pkgs.chromium
                pkgs.curl
              ];
              FONTCONFIG_FILE = pkgs.makeFontsConf { fontDirectories = [ pkgs.dejavu_fonts ]; };
            }
            ''
              export HOME=$TMPDIR
              bash ${sourceOf pkgs}/e2e/browser/run.sh
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
            pkgs.hugo
          ];
          HUGO_THEMESDIR = docsThemes pkgs;
          PYTHONDONTWRITEBYTECODE = "1";
          RUFF_NO_CACHE = "true";
          PYTEST_ADDOPTS = "-p no:cacheprovider";
        };
      });
    };
}
