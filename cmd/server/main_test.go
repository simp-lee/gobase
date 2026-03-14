package main

import (
	"errors"
	"testing"

	"github.com/simp-lee/gobase/internal/config"
)

type fakeAppRunner struct {
	runCalls int
	runErr   error
}

func (f *fakeAppRunner) Run() error {
	f.runCalls++
	return f.runErr
}

func TestRunServer_MigrateOnly_ExitsBeforeAppStartup(t *testing.T) {
	var loadPath string
	migrateCalls := 0
	newAppCalls := 0

	deps := serverDeps{
		loadConfig: func(path string) (*config.Config, error) {
			loadPath = path
			return &config.Config{}, nil
		},
		migrate: func(cfg *config.Config) error {
			migrateCalls++
			return nil
		},
		newApp: func(cfg *config.Config) (appRunner, error) {
			newAppCalls++
			return &fakeAppRunner{}, nil
		},
	}

	err := runServer([]string{"-config", "configs/test.yaml", "-migrate-only"}, deps)
	if err != nil {
		t.Fatalf("runServer returned error: %v", err)
	}

	if loadPath != "configs/test.yaml" {
		t.Fatalf("expected config path %q, got %q", "configs/test.yaml", loadPath)
	}
	if migrateCalls != 1 {
		t.Fatalf("expected migrate to be called once, got %d", migrateCalls)
	}
	if newAppCalls != 0 {
		t.Fatalf("expected newApp to not be called in migrate-only mode, got %d", newAppCalls)
	}
}

func TestRunServer_MigrateOnly_PropagatesMigrationError(t *testing.T) {
	expectedErr := errors.New("boom")
	deps := serverDeps{
		loadConfig: func(path string) (*config.Config, error) {
			return &config.Config{}, nil
		},
		migrate: func(cfg *config.Config) error {
			return expectedErr
		},
		newApp: func(cfg *config.Config) (appRunner, error) {
			return &fakeAppRunner{}, nil
		},
	}

	err := runServer([]string{"-migrate-only"}, deps)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
