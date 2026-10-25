// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Secret         []byte
	Pepper         []byte
	RPID           string
	RPName         string
	Origins        []string
	DB             string
	Listen         string
	MaxCredentials int
}

type Lookup func(string) (string, bool)

type ReadFile func(string) ([]byte, error)

func Load(lookup Lookup, readFile ReadFile) (Config, error) {
	var problems []error
	text := func(name string) string {
		value, err := value(lookup, readFile, name)
		if err != nil {
			problems = append(problems, err)
		}
		return value
	}
	cfg := Config{
		Secret:  []byte(text("PARVUS_SECRET")),
		Pepper:  []byte(text("PARVUS_PEPPER")),
		RPID:    text("PARVUS_RP_ID"),
		RPName:  text("PARVUS_RP_NAME"),
		DB:      text("PARVUS_DB"),
		Listen:  text("PARVUS_LISTEN"),
		Origins: origins(text("PARVUS_ORIGINS")),
	}
	limit := text("PARVUS_MAX_CREDENTIALS")
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 {
			problems = append(problems, errors.New("PARVUS_MAX_CREDENTIALS must be a positive integer"))
		}
		cfg.MaxCredentials = n
	}
	return cfg, errors.Join(problems...)
}

func value(lookup Lookup, readFile ReadFile, name string) (string, error) {
	direct, hasDirect := lookup(name)
	path, hasFile := lookup(name + "_FILE")
	switch {
	case hasDirect && hasFile:
		return "", fmt.Errorf("set only one of %s and %s_FILE", name, name)
	case hasFile:
		data, err := readFile(path)
		if err != nil {
			return "", fmt.Errorf("%s_FILE: %w", name, err)
		}
		direct = strings.TrimRight(string(data), "\r\n")
	case !hasDirect:
		return "", fmt.Errorf("%s is required", name)
	}
	if direct == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	return direct, nil
}

func origins(list string) []string {
	var out []string
	for _, item := range strings.Split(list, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func FromEnvironment() (Config, error) {
	return Load(os.LookupEnv, os.ReadFile)
}
