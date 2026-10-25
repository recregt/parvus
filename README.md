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

Download a release for Linux or macOS, on amd64 or arm64, and check it against the published checksums.

```sh
curl -LO https://github.com/recregt/parvus/releases/download/v0.1.0/parvus_0.1.0_linux_amd64.tar.gz
curl -LO https://github.com/recregt/parvus/releases/download/v0.1.0/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf parvus_0.1.0_linux_amd64.tar.gz parvus
./parvus --version
```

Every release file and image is signed with build provenance. With the GitHub CLI you can confirm that a file was built by this repository's release workflow.

```sh
gh attestation verify parvus_0.1.0_linux_amd64.tar.gz --repo recregt/parvus
```

Or run the container image, which holds nothing but the binary.

```sh
docker run --rm ghcr.io/recregt/parvus:0.1.0 --version
```

Or build it with Go.

```sh
go install github.com/recregt/parvus/cmd/parvus@v0.1.0
```

The [guide](https://parvus.recregt.com/guide/) takes it from here.

## License

Parvus is free software under the [Mozilla Public License 2.0](LICENSE).
