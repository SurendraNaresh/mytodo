//go:build !js

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	appconfig "github.com/SurendraNaresh/mytodo/internal/config"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/server"
)

func main() {
	config, err := appconfig.Load()
	if err != nil {
		log.Fatal(err)
	}
	if backupPath := os.Getenv("MYTODO_RESTORE_FROM"); backupPath != "" {
		if err := db.RestoreDatabaseAt(backupPath, config.DBFilename); err != nil {
			log.Fatal(err)
		}
	} else if err := db.CopyStarterIfMissing(filepath.Join(".", db.DBName), config.DBFilename); err != nil {
		log.Fatal(err)
	}
	if err := db.OpenAtPath(config.DBFilename); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	apiServer := server.New()
	mux.Handle("/api/v1/", apiServer)
	mux.Handle("/healthz", apiServer)
	address := ":" + config.Port
	log.Printf("mytodo API listening on %s (database: %s)", address, config.DBFilename)
	log.Fatal(http.ListenAndServe(address, server.WithCORS(mux, config.CORSOrigins)))
}
