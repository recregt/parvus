# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

FROM registry.fedoraproject.org/fedora:43@sha256:1965dd267a1be0f695534e132de7719e50b142389528226d49288a8866dbe5ad

RUN dnf install -y -q systemd curl shadow-utils \
    && dnf clean all

STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
