<!--
This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
-->

# Parvus

Passkey sign-in for any backend, in one small binary. No passwords, no email, no text messages.

Parvus sits behind your backend on a private socket. Your backend relays a few small requests, and Parvus does the careful part: WebAuthn ceremonies, sessions, one recovery code per account, and resets. It stores everything in a SQLite file, or in Postgres when you run more than one copy.

**Documentation: <https://parvus.recregt.com>**

## Install

On Debian and Ubuntu, the package installs Parvus as a systemd service with its own user and freshly generated secrets.

```sh
curl -LO https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.deb
sudo apt install ./parvus_linux_amd64.deb
```

On Fedora, RHEL and openSUSE:

```sh
sudo dnf install https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.rpm
```

On Alpine, with OpenRC:

```sh
curl -LO https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.apk
sudo apk add --allow-untrusted ./parvus_linux_amd64.apk
```

Then set your domain in `/etc/parvus/parvus.env` and start the service with `sudo systemctl enable --now parvus`, or `sudo rc-update add parvus && sudo rc-service parvus start` on Alpine. Every package also comes for arm64: replace `amd64` with `arm64`.

On macOS or Linux with Homebrew:

```sh
brew install recregt/tap/parvus
```

As a container:

```sh
docker run --rm ghcr.io/recregt/parvus --version
```

As a plain binary, for Linux or macOS on amd64 or arm64:

```sh
curl -L https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.tar.gz | tar -xz parvus
```

With Go:

```sh
go install github.com/recregt/parvus/cmd/parvus@latest
```

Every release file and image is signed with build provenance, so you can confirm that a download was built by this repository's release workflow.

```sh
gh attestation verify parvus_linux_amd64.deb --repo recregt/parvus
```

The [guide](https://parvus.recregt.com/guide/) takes it from here.

## License

Parvus is free software under the [Mozilla Public License 2.0](LICENSE).
