# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

FROM registry.fedoraproject.org/fedora:43@sha256:6bcda9dfa23df4cb53a019ef2e480a54db32f688fa514aacfbb5f050814a03c8

RUN dnf install -y -q systemd curl shadow-utils \
    && dnf clean all

STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
