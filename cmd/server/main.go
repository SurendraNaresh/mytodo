//go:build !js

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/server"
)

func main() {
	if err := db.OpenWithRestore(os.Getenv("MYTODO_RESTORE_FROM")); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	apiServer := server.New()
	mux.Handle("/api/v1/", apiServer)
	mux.Handle("/healthz", apiServer)
	webDir := os.Getenv("MYTODO_WEB_DIR")
	if webDir == "" {
		webDir = "web"
	}
	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	address := os.Getenv("MYTODO_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	log.Printf("mytodo server listening on http://%s (web root: %s)", address, webDir)
	log.Fatal(http.ListenAndServe(address, mux))
}
