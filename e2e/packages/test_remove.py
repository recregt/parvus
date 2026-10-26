# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

from support.machine import PEPPER, SECRET


def test_removing_the_package_stops_the_service_and_keeps_both_secrets(machine):
    machine.install("new")
    machine.configure()
    machine.start()
    secret, pepper = machine.digest_of(SECRET), machine.digest_of(PEPPER)

    assert machine.remove().returncode == 0

    assert not machine.is_running()
    assert machine.digest_of(SECRET) == secret
    assert machine.digest_of(PEPPER) == pepper


def test_the_package_installs_on_a_machine_without_an_init_system(bare_machine):
    assert bare_machine.install("new").returncode == 0
    assert bare_machine.run("id", "parvus").returncode == 0
