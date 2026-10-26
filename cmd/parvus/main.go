// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/recregt/parvus/internal/api"
	"github.com/recregt/parvus/internal/config"
	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/store"
	"github.com/recregt/parvus/internal/store/postgres"
	"github.com/recregt/parvus/internal/store/sqlite"
)

const (
	unixPrefix   = "unix:"
	socketMode   = 0o660
	privateFiles = 0o077
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	switch {
	case *showVersion:
		fmt.Println(currentVersion())
	case flag.NArg() > 0:
		slog.Error("parvus takes no arguments; configure it with PARVUS_* environment variables", "arguments", flag.Args())
		os.Exit(1)
	default:
		if err := run(); err != nil {
			slog.Error("parvus stopped", "error", err)
			os.Exit(1)
		}
	}
}

func run() error {
	restrictNewFiles()
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := openStore(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	eng, err := engine.New(db, engine.Config{
		RPID:           cfg.RPID,
		RPName:         cfg.RPName,
		Origins:        cfg.Origins,
		Pepper:         cfg.Pepper,
		MaxCredentials: cfg.MaxCredentials,
	})
	if err != nil {
		return err
	}
	listener, err := listen(cfg.Listen, log)
	if err != nil {
		return err
	}

	//nolint:gosec // only the authenticated host connects, so slow-client timeouts are not needed
	server := &http.Server{Handler: api.New(eng, cfg.Secret, log)}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	log.Info("listening", "address", listener.Addr().String(), "version", currentVersion())

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	if err := server.Shutdown(context.Background()); err != nil {
		return err
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func currentVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return version
}

func restrictNewFiles() {
	syscall.Umask(privateFiles)
}

type closableStore interface {
	store.Store
	Close() error
}

func openStore(ctx context.Context, location string) (closableStore, error) {
	if strings.HasPrefix(location, "postgres://") || strings.HasPrefix(location, "postgresql://") {
		return postgres.Open(ctx, location)
	}
	return sqlite.Open(ctx, location)
}

func listenUnix(path string) (net.Listener, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
		if conn, err := net.Dial("unix", path); err == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("%s is served by another process", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, socketMode); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}

func listen(address string, log *slog.Logger) (net.Listener, error) {
	if path, ok := strings.CutPrefix(address, unixPrefix); ok {
		return listenUnix(path)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("PARVUS_LISTEN: %w", err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Warn("listening beyond loopback; the engine speaks plain HTTP", "host", host)
	}
	return net.Listen("tcp", address)
}
