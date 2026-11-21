# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

FROM registry.fedoraproject.org/fedora:46@sha256:fe07221648d8016255ff68e4c59b138d988ab3517a4ae552e799b2fb47e98175

RUN dnf install -y -q systemd curl shadow-utils \
    && dnf clean all

STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
