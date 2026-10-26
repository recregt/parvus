# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

FROM registry.fedoraproject.org/fedora:46@sha256:02031526999d46c9f9c6490eeccf4cafd735438b9d0fb5e1d9fc941d7886e2ff

RUN dnf install -y -q systemd curl shadow-utils \
    && dnf clean all

STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
