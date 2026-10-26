# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

from dataclasses import dataclass

from periplu.container import Container

SETTINGS = "/etc/parvus/parvus.env"
SECRET = "/etc/parvus/secret"
PEPPER = "/etc/parvus/pepper"
SOCKET = "/run/parvus/parvus.sock"
BACKEND = "backend"
CONFIGURED = [
    "PARVUS_RP_ID=example.com",
    "PARVUS_RP_NAME=Example",
    "PARVUS_ORIGINS=https://example.com",
    "PARVUS_MAX_CREDENTIALS=20",
]


@dataclass(frozen=True)
class Distro:
    name: str
    containerfile: str
    bare_image: str
    package: str
    systemd: bool
    install: tuple[str, ...]
    remove: tuple[str, ...]
    add_backend: tuple[str, ...]
    pepper: str


DEBIAN = Distro(
    name="debian",
    containerfile="images/debian.Containerfile",
    bare_image="docker.io/library/debian:13@sha256:4c25275446ca371766b8b093c696782eabce06263fccacb8d5d813b3dc44b308",
    package="deb",
    systemd=True,
    install=("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y"),
    remove=("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", "parvus"),
    add_backend=("useradd", "--groups", "parvus", BACKEND),
    pepper="root:root 600",
)

FEDORA = Distro(
    name="fedora",
    containerfile="images/fedora.Containerfile",
    bare_image="registry.fedoraproject.org/fedora:43@sha256:1965dd267a1be0f695534e132de7719e50b142389528226d49288a8866dbe5ad",
    package="rpm",
    systemd=True,
    install=("dnf", "install", "-y"),
    remove=("dnf", "remove", "-y", "parvus"),
    add_backend=("useradd", "--groups", "parvus", BACKEND),
    pepper="root:root 600",
)

ALPINE = Distro(
    name="alpine",
    containerfile="images/alpine.Containerfile",
    bare_image="docker.io/library/alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f",
    package="apk",
    systemd=False,
    install=("apk", "add", "--allow-untrusted"),
    remove=("apk", "del", "parvus"),
    add_backend=("adduser", "-D", "-G", "parvus", BACKEND),
    pepper="parvus:parvus 400",
)

DISTROS = [DEBIAN, FEDORA, ALPINE]
SYSTEMD_DISTROS = [distro for distro in DISTROS if distro.systemd]


class Machine(Container):
    def __init__(self, name: str, distro: Distro):
        super().__init__(name)
        self.distro = distro

    def run(self, *args: str, user: str | None = None):
        result = self.exec(*args, user=user)
        self.record(" ".join(args), result.returncode, result.stdout, result.stderr)
        return result

    def start_openrc(self) -> None:
        self.run("mkdir", "-p", "/run/openrc")
        self.run("touch", "/run/openrc/softlevel")
        self.run("openrc", "default")

    def install(self, package: str):
        return self.run(*self.distro.install, f"/packages/{package}.{self.distro.package}")

    def remove(self):
        return self.run(*self.distro.remove)

    def configure(self) -> None:
        self.run("sh", "-c", f'printf "%s\\n" "$@" >> {SETTINGS}', "sh", *CONFIGURED)

    def start(self):
        if self.distro.systemd:
            return self.run("systemctl", "enable", "--now", "parvus")
        return self.run("sh", "-c", "rc-update add parvus && rc-service parvus start")

    def is_running(self) -> bool:
        if self.distro.systemd:
            return self.run("systemctl", "is-active", "parvus").stdout.strip() == "active"
        return self.run("rc-service", "parvus", "status").returncode == 0

    def add_backend(self) -> None:
        self.run(*self.distro.add_backend)

    def mode_of(self, path: str) -> str:
        return self.run("stat", "-c", "%U:%G %a", path).stdout.strip()

    def backend_reads(self, path: str) -> bool:
        return self.run("cat", path, user=BACKEND).returncode == 0

    def backend_health(self) -> str:
        return self.run(
            "curl",
            "--silent",
            "--output",
            "/dev/null",
            "--write-out",
            "%{http_code}",
            "--unix-socket",
            SOCKET,
            "http://parvus/healthz",
            user=BACKEND,
        ).stdout

    def digest_of(self, path: str) -> str:
        return self.run("sha256sum", path).stdout.split()[0]

    def main_pid(self) -> str:
        return self.run(
            "systemctl", "show", "--property", "MainPID", "--value", "parvus"
        ).stdout.strip()
