---
title: Going to production
weight: 8
---

The notes app is ready for real users, and Parvus is about to run somewhere that matters. On your laptop it got by on example values typed into a terminal. In production, every one of those values deserves a moment of thought, and a few of them deserve a backup.

## Two secrets

Parvus has two secrets, and they protect different things.

`PARVUS_SECRET` is shared between Parvus and your backend. Anyone who holds it can do everything your backend can, so treat it like a database password. If it leaks, set a new value on both sides and restart them.

`PARVUS_PEPPER` belongs to Parvus alone. It is the key behind every stored recovery code. Back it up, but never in the same place as your database backups: a database without the pepper reveals no codes, and that only holds while the two stay apart. If the pepper is lost, every recovery code stops working. Nothing else breaks, and people can ask for fresh codes once they are signed in.

Any long random value works for either. One way to make one:

```sh
openssl rand -base64 32
```

## Keeping secrets out of the environment

Environment variables leak more easily than they seem: into process listings, crash reports and container inspectors. Every Parvus setting can instead be read from a file. Set `PARVUS_PEPPER_FILE=/path/to/file` in place of `PARVUS_PEPPER`, and Parvus reads the file, ignoring a trailing newline. Setting both forms of the same variable is an error, so there is never a question of which one won.

## The short way: a package

On Debian, Ubuntu, Fedora, RHEL, openSUSE and Alpine, a package does all of the setup in this chapter for you.

```sh
curl -LO https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.deb
sudo apt install ./parvus_linux_amd64.deb
```

On Fedora and its relatives it is one line, `sudo dnf install https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.rpm`. On Alpine, download `parvus_linux_amd64.apk` and run `sudo apk add --allow-untrusted ./parvus_linux_amd64.apk`. Every package also exists for arm64.

The package creates a `parvus` system user, generates the secret and the pepper in `/etc/parvus/`, and installs a service. It cannot know your domain, so the service waits for you. Open `/etc/parvus/parvus.env`, fill in the four empty settings, and start it.

```sh
sudoedit /etc/parvus/parvus.env
sudo systemctl enable --now parvus
```

On Alpine, start it with `sudo rc-update add parvus && sudo rc-service parvus start`. Until the settings are filled in, the service refuses to start and says which setting is missing.

Parvus then listens on the unix socket `/run/parvus/parvus.sock`. Your backend reaches it by joining the `parvus` group, which also lets it read the shared secret from `/etc/parvus/secret`. The pepper stays readable by root alone.

Upgrading is installing the newer package the same way. Your settings, the secret and the pepper stay as they are, and a running service restarts on the new version. Removing the package stops the service and leaves `/etc/parvus` behind, so nothing that protects your users' recovery codes disappears by accident.

## A service by hand

On a system without a package, the same setup takes a few steps. On a systemd machine, credentials are the cleanest home for the secret files. Give Parvus a system user of its own first.

```sh
sudo useradd --system --no-create-home parvus
```

This unit runs Parvus as that user, gives it a private directory for its database and another for its socket, and hands it the two secrets without ever putting them in the environment.

```ini
[Unit]
Description=Parvus passkey engine
After=network.target

[Service]
ExecStart=/usr/local/bin/parvus
User=parvus
Group=parvus
StateDirectory=parvus
RuntimeDirectory=parvus
RuntimeDirectoryMode=0750
LoadCredential=secret:/etc/parvus/secret
LoadCredential=pepper:/etc/parvus/pepper
Environment=PARVUS_SECRET_FILE=%d/secret
Environment=PARVUS_PEPPER_FILE=%d/pepper
Environment=PARVUS_RP_ID=notes.example.com
Environment=PARVUS_RP_NAME=Notes
Environment=PARVUS_ORIGINS=https://notes.example.com
Environment=PARVUS_DB=%S/parvus/parvus.db
Environment=PARVUS_LISTEN=unix:%t/parvus/parvus.sock
Environment=PARVUS_MAX_CREDENTIALS=20
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Your backend reaches the socket by joining the `parvus` group. In your backend's own unit, that is one line.

```ini
SupplementaryGroups=parvus
```

`PARVUS_RP_ID` is your site's domain, and `PARVUS_ORIGINS` lists every origin your users' browsers will show, separated by commas. Passkeys made for one domain never work on another, so choose the domain before anyone registers.

When systemd asks Parvus to stop, it finishes the requests it is already serving and exits. If Parvus is ever killed outright, it starts again without fuss.

## In a container

The image `ghcr.io/recregt/parvus` holds the binary and nothing else, and runs as an unprivileged user. Give it a network of its own that only your backend joins, a volume for its database, and the secrets as read-only files.

```sh
docker network create --internal parvus
docker volume create parvus-data

docker run -d --name parvus --network parvus \
  -v parvus-data:/home/nonroot \
  -v /etc/parvus:/run/secrets:ro \
  -e PARVUS_SECRET_FILE=/run/secrets/secret \
  -e PARVUS_PEPPER_FILE=/run/secrets/pepper \
  -e PARVUS_RP_ID=notes.example.com \
  -e PARVUS_RP_NAME=Notes \
  -e PARVUS_ORIGINS=https://notes.example.com \
  -e PARVUS_DB=/home/nonroot/parvus.db \
  -e PARVUS_LISTEN=0.0.0.0:9000 \
  -e PARVUS_MAX_CREDENTIALS=20 \
  ghcr.io/recregt/parvus:0.1.0
```

Start your backend with `--network parvus` as well, and it reaches Parvus at `http://parvus:9000`. The network is internal, so Parvus itself has no way out, and it needs none unless its database lives elsewhere.

The volume sits on `/home/nonroot`, the one directory the unprivileged user owns, so Parvus can write its database there. The secret files must be readable by that user, whose id inside the container is 65532.

Do not publish Parvus's port with `-p`. Parvus puts no time limit on reading a request, so anything that can reach it can hold a connection open without ever presenting the secret. On a network that only your backend joins, that never matters.

## Where Parvus listens

Parvus speaks plain HTTP and expects to be reachable only by your backend. `PARVUS_LISTEN` takes either a TCP address or a unix socket.

A unix socket, written as `unix:/run/parvus/parvus.sock`, keeps Parvus off the network entirely, which is why the unit above uses one. Parvus creates the socket so that its own user and its group can connect and nobody else can, so your backend needs either the same user or membership of that group. If a socket is left behind by a crash, the next start removes it, and it refuses to take over a socket another running copy still answers on.

A loopback address such as `127.0.0.1:9000` works too, whenever your backend runs on the same machine. Any process on the machine can then reach Parvus, and only the secret keeps them out.

Any other address works too, for example when Parvus and your backend share a private container network. Parvus logs a warning when it listens beyond loopback, because the shared secret then crosses a network in plain text. Do it only on a network that nothing else can reach.

## Where the data lives

`PARVUS_DB` decides the store. A file path means SQLite, and a `postgres://` or `postgresql://` address means Postgres.

SQLite suits a single copy of Parvus. Parvus holds the file exclusively while it runs, so nothing else can open it at the same time, not even a backup tool. Back it up by stopping Parvus, copying the database file, and starting it again.

Postgres suits everything else: more than one copy of Parvus, backups while it runs, or a database you already operate. Point every copy at the same database. When several copies start at the same moment on an empty database, exactly one creates the tables and the others wait for it.

Both stores pass the same set of rules in Parvus's own tests, including the ones about two requests arriving at once, so moving from one to the other changes how you operate Parvus, not how it behaves.

## Watching it run

`GET /healthz` answers `204` without asking for the secret, which is all a load balancer or a container runtime needs. Parvus writes one line of JSON to standard output for every request, with the method, the path and the status, and one more line with the reason whenever it refuses a request. User ids appear in some paths. Tokens, recovery codes and secrets never appear in the logs.

On start, Parvus checks the version of the schema it finds. A database written by a newer Parvus is refused instead of being misread.

Parvus is now running, guarded and backed up. The last chapter steps back and asks what all of this actually protects.
