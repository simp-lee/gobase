package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/simp-lee/gobase/internal/app"
	"github.com/simp-lee/gobase/internal/config"
)

type appRunner interface {
	Run() error
}

type serverDeps struct {
	loadConfig func(path string) (*config.Config, error)
	migrate    func(cfg *config.Config) error
	newApp     func(cfg *config.Config) (appRunner, error)
}

func defaultServerDeps() serverDeps {
	return serverDeps{
		loadConfig: config.Load,
		migrate:    app.Migrate,
		newApp: func(cfg *config.Config) (appRunner, error) {
			return app.New(cfg)
		},
	}
}

func runServer(args []string, deps serverDeps) error {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "configs/config.yaml", "path to configuration file")
	migrateOnly := fs.Bool("migrate-only", false, "run database migrations and exit")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg, err := deps.loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if *migrateOnly {
		if err := deps.migrate(cfg); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		log.Println("migration completed successfully")
		return nil
	}

	a, err := deps.newApp(cfg)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	if err := a.Run(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func main() {
	if err := runServer(os.Args[1:], defaultServerDeps()); err != nil {
		log.Fatal(err)
	}
}
