# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

import pytest

from support.machine import PEPPER, SECRET, SYSTEMD_DISTROS


def test_a_fresh_install_creates_the_user_and_both_secrets(machine):
    assert machine.install("new").returncode == 0

    assert machine.run("id", "parvus").returncode == 0
    assert machine.mode_of(SECRET) == "root:parvus 640"
    assert machine.mode_of(PEPPER) == machine.distro.pepper


def test_the_service_refuses_to_start_until_it_is_configured(machine):
    machine.install("new")

    assert machine.start().returncode != 0
    assert not machine.is_running()


def test_the_backend_can_read_the_secret_but_never_the_pepper(machine):
    machine.install("new")
    machine.add_backend()

    assert machine.backend_reads(SECRET)
    assert not machine.backend_reads(PEPPER)


@pytest.mark.parametrize("distro", SYSTEMD_DISTROS, ids=lambda distro: distro.name, indirect=True)
def test_the_backend_reaches_parvus_as_soon_as_the_start_returns(machine):
    machine.install("new")
    machine.configure()
    machine.add_backend()

    assert machine.start().returncode == 0
    assert machine.backend_health() == "204"
