# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

FROM registry.fedoraproject.org/fedora:46@sha256:dea8962dbcab334036946698c513d18a6dff49c4ce63765353018a899bce9f89

RUN dnf install -y -q systemd curl shadow-utils \
    && dnf clean all

STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
