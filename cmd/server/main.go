package main

import (
	"flag"
	"log"

	"github.com/simp-lee/gobase/internal/app"
	"github.com/simp-lee/gobase/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal("failed to load config: ", err)
	}

	a, err := app.New(cfg)
	if err != nil {
		log.Fatal("failed to create app: ", err)
	}

	if err := a.Run(); err != nil {
		log.Fatal("server error: ", err)
	}
}
