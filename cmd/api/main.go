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

	address := os.Getenv("MYTODO_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8081"
	}
	log.Printf("mytodo API listening on http://%s", address)
	log.Fatal(http.ListenAndServe(address, server.New()))
}
