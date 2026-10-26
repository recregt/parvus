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

      checks = forEach (
        pkgs:
        {
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
        }
      );

      apps = forEach (pkgs: {
        fuzz =
          let
            skipModerncCoverage = pkgs.writeShellApplication {
              name = "parvus-fuzz-skip-modernc-coverage";
              text = ''
                tool=$1
                shift
                if [[ ''${1-} == -V=full ]]; then
                  echo "$("$tool" "$@") skip-modernc-coverage"
                  exit
                fi
                if [[ ''${tool##*/} == compile && " $* " == *" -p modernc.org/"* ]]; then
                  args=()
                  for arg in "$@"; do
                    [[ $arg == -d=libfuzzer ]] || args+=("$arg")
                  done
                  set -- "''${args[@]}"
                fi
                exec "$tool" "$@"
              '';
            };
          in
          {
            type = "app";
            meta.description = "Fuzz a store against the model until interrupted";
            program = pkgs.lib.getExe (
              pkgs.writeShellApplication {
                name = "parvus-fuzz";
                runtimeInputs = [
                  pkgs.go
                  pkgs.git
                  pkgs.postgresql
                ];
                text = ''
                  cd "$(git rev-parse --show-toplevel)"
                  store=''${1:-sqlite}
                  shift || true
                  if [[ $store != sqlite && $store != postgres ]]; then
                    echo "usage: nix run .#fuzz -- [sqlite|postgres] [go test flags]" >&2
                    exit 2
                  fi
                  package=internal/store/$store
                  report=fuzz-failures/$store
                  rm -rf "$report"

                  if [[ -d /dev/shm ]]; then
                    scratch=$(mktemp -d -p /dev/shm)
                  else
                    scratch=$(mktemp -d)
                  fi
                  trap 'rm -rf "$scratch"' EXIT
                  export TMPDIR=$scratch

                  tags=()
                  if [[ $store == postgres ]]; then
                    server=$scratch/postgres
                    trap 'pg_ctl --pgdata="$server/data" --wait stop >/dev/null; rm -rf "$scratch"' EXIT
                    initdb --pgdata="$server/data" --auth=trust --username=parvus >/dev/null
                    pg_ctl --pgdata="$server/data" --wait --log="$server/postgres.log" \
                      --options="-c listen_addresses= -c unix_socket_directories=$server -c log_min_messages=log -c log_checkpoints=off" \
                      start >/dev/null
                    export PARVUS_TEST_POSTGRES="host=$server user=parvus"
                    tags=(-tags postgres)
                  fi

                  trap 'interrupted=1' INT
                  if go test -toolexec ${pkgs.lib.getExe skipModerncCoverage} "''${tags[@]}" \
                    -run '^$' -fuzz '^FuzzAgreesWithTheModel$' "$@" "./$package" 2>&1 | tee -i "$scratch/fuzz.log"; then
                    exit
                  fi
                  if [[ -v interrupted ]]; then
                    exit 130
                  fi

                  mkdir -p "$report"
                  cp "$scratch/fuzz.log" "$report"
                  if [[ $store == postgres ]]; then
                    cp "$server/postgres.log" "$report"
                  fi
                  sed -n 's/^ *Failing input written to //p' "$scratch/fuzz.log" | while read -r input; do
                    mkdir -p "$report/$(dirname "$input")"
                    cp "$package/$input" "$report/$input"
                  done
                  echo "failure report: $report" >&2
                  exit 1
                '';
              }
            );
          };
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
