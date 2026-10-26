# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

import pytest

from support.machine import PEPPER, SECRET, SETTINGS, SYSTEMD_DISTROS

KEPT = (SETTINGS, SECRET, PEPPER)


def test_an_upgrade_keeps_the_settings_and_both_secrets(machine):
    machine.install("released")
    machine.configure()
    before = {path: machine.digest_of(path) for path in KEPT}

    assert machine.install("new").returncode == 0

    assert {path: machine.digest_of(path) for path in KEPT} == before


def test_an_upgrade_leaves_the_pepper_out_of_the_backends_reach(machine):
    machine.install("released")
    machine.add_backend()

    machine.install("new")

    assert machine.mode_of(PEPPER) == machine.distro.pepper
    assert not machine.backend_reads(PEPPER)


@pytest.mark.parametrize("distro", SYSTEMD_DISTROS, ids=lambda distro: distro.name, indirect=True)
def test_an_upgrade_restarts_a_running_service_on_the_new_binary(machine):
    machine.install("released")
    machine.configure()
    machine.add_backend()
    machine.start()
    before = machine.main_pid()

    machine.install("new")

    assert machine.main_pid() not in {before, "0"}
    assert machine.backend_health() == "204"
