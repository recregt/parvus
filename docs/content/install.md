---
title: Install
type: docs
---

Parvus runs on Linux and macOS. For ARM, swap `amd64` for `arm64` in any file name.

## Linux package

The package sets up a service and generates its secrets.

{{< tabs >}}
{{< tab name="Debian and Ubuntu" >}}
```sh
curl -LO https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.deb
sudo apt install ./parvus_linux_amd64.deb
```
{{< /tab >}}
{{< tab name="Fedora and RHEL" >}}
```sh
sudo dnf install https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.rpm
```
{{< /tab >}}
{{< tab name="Alpine" >}}
```sh
curl -LO https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.apk
sudo apk add --allow-untrusted ./parvus_linux_amd64.apk
```
{{< /tab >}}
{{< /tabs >}}

Set your domain in `/etc/parvus/parvus.env`, then start it:

{{< tabs >}}
{{< tab name="systemd" >}}
```sh
sudo systemctl enable --now parvus
```
{{< /tab >}}
{{< tab name="OpenRC" >}}
```sh
sudo rc-update add parvus && sudo rc-service parvus start
```
{{< /tab >}}
{{< /tabs >}}

## Homebrew

```sh
brew install go-parvus/tap/parvus
```

## Container

```sh
docker pull ghcr.io/recregt/parvus
```

## Binary

```sh
curl -L https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.tar.gz | tar -xz parvus
```

## Go

```sh
go install github.com/recregt/parvus/cmd/parvus@latest
```

## Verify

```sh
gh attestation verify <file> --repo recregt/parvus
```

Next: [connect your backend]({{< relref "/guide/your-side-of-the-wire" >}}).
