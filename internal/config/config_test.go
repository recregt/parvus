// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package config_test

import (
	"errors"
	"io/fs"
	"maps"
	"testing"

	"github.com/recregt/parvus/internal/config"
)

func environment(extra map[string]string) config.Lookup {
	values := map[string]string{
		"PARVUS_SECRET":          "host-secret",
		"PARVUS_PEPPER":          "pepper",
		"PARVUS_RP_ID":           "example.com",
		"PARVUS_RP_NAME":         "Example",
		"PARVUS_ORIGINS":         "https://example.com, https://app.example.com",
		"PARVUS_DB":              "/data/parvus.db",
		"PARVUS_LISTEN":          "unix:/run/parvus.sock",
		"PARVUS_MAX_CREDENTIALS": "20",
	}
	maps.Copy(values, extra)
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func files(contents map[string]string) config.ReadFile {
	return func(path string) ([]byte, error) {
		content, ok := contents[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(content), nil
	}
}

func TestSecretsCanComeFromFilesWithoutTheirTrailingNewline(t *testing.T) {
	lookup := without(environment(map[string]string{"PARVUS_PEPPER_FILE": "/run/secrets/pepper"}), "PARVUS_PEPPER")
	cfg, err := config.Load(lookup, files(map[string]string{"/run/secrets/pepper": "from-file\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.Pepper) != "from-file" {
		t.Fatalf("pepper = %q", cfg.Pepper)
	}
	if len(cfg.Origins) != 2 || cfg.Origins[1] != "https://app.example.com" {
		t.Fatalf("origins = %q", cfg.Origins)
	}
}

func without(lookup config.Lookup, name string) config.Lookup {
	return func(key string) (string, bool) {
		if key == name {
			return "", false
		}
		return lookup(key)
	}
}

func TestEachSettingComesFromExactlyOneSource(t *testing.T) {
	both := environment(map[string]string{"PARVUS_SECRET_FILE": "/run/secrets/secret"})
	if _, err := config.Load(both, files(map[string]string{"/run/secrets/secret": "x"})); err == nil {
		t.Fatal("a setting given both directly and as a file was accepted")
	}
	if _, err := config.Load(without(environment(nil), "PARVUS_PEPPER"), files(nil)); err == nil {
		t.Fatal("a missing setting was accepted")
	}
	unreadable := without(environment(map[string]string{"PARVUS_SECRET_FILE": "/nope"}), "PARVUS_SECRET")
	if _, err := config.Load(unreadable, files(nil)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unreadable secret file: %v", err)
	}
}
