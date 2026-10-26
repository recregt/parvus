# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

import fcntl
import platform
import shutil
import subprocess
import urllib.request
from pathlib import Path

import pytest
from periplu import container as periplu
from periplu import settings

from support.machine import DISTROS, Machine

HERE = Path(__file__).parent
REPO_ROOT = HERE.parents[1]
LATEST = "https://github.com/recregt/parvus/releases/latest/download"
ARCHITECTURES = {"x86_64": "amd64", "aarch64": "arm64"}
FORMATS = ("deb", "rpm", "apk")
IDLE = ["--entrypoint", '["tail", "-f", "/dev/null"]']
SNAPSHOT = [
    "nix",
    "shell",
    "--inputs-from",
    ".",
    "nixpkgs#goreleaser",
    "nixpkgs#go",
    "--command",
    "goreleaser",
    "release",
    "--snapshot",
    "--clean",
    "--skip=docker,publish,sign,homebrew",
]


def _file_name(package: str) -> str:
    return f"parvus_linux_{ARCHITECTURES[platform.machine()]}.{package}"


def _gather(directory: Path) -> None:
    subprocess.run(SNAPSHOT, cwd=REPO_ROOT, check=True, capture_output=True)
    for package in FORMATS:
        shutil.copy(REPO_ROOT / "dist" / _file_name(package), directory / f"new.{package}")
        urllib.request.urlretrieve(
            f"{LATEST}/{_file_name(package)}", directory / f"released.{package}"
        )


@pytest.fixture(scope="session")
def packages() -> Path:
    cache = settings.current().cache
    directory = cache / f"packages-{settings.session()}"
    cache.mkdir(parents=True, exist_ok=True)
    with open(cache / "packages.lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if not (directory / "ready").exists():
            for stale in cache.glob("packages-*"):
                shutil.rmtree(stale)
            directory.mkdir()
            _gather(directory)
            (directory / "ready").touch()
    return directory


def _started(request, packages: Path, image: str, systemd: bool, variant: str):
    run_args = ["--volume", f"{packages}:/packages:ro"]
    if not systemd:
        run_args += ["--init", *IDLE]
    return periplu.admitted(
        request.node.nodeid, image, run_args=run_args, systemd=systemd, variant=variant
    )


@pytest.fixture(params=DISTROS, ids=lambda distro: distro.name)
def distro(request):
    return request.param


@pytest.fixture
def machine(request, distro, packages):
    image = periplu.image(HERE / distro.containerfile)
    with _started(request, packages, image, distro.systemd, "fresh") as name:
        booted = Machine(name, distro)
        if not distro.systemd:
            booted.start_openrc()
        yield booted


@pytest.fixture
def bare_machine(request, distro, packages):
    with _started(request, packages, distro.bare_image, False, "bare") as name:
        yield Machine(name, distro)
