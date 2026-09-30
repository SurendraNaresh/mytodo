//go:build !js

package main

import (
	"log"
	"os"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	if err := api.ConfigureFromEnvironment(); err != nil {
		log.Fatal(err)
	}
	model.UseRemoteAPI(api.Enabled())
	if !api.Enabled() {
		if err := db.OpenWithRestore(os.Getenv("MYTODO_RESTORE_FROM")); err != nil {
			log.Fatal(err)
		}
		defer db.Close()
	}

	application := ui.NewApp()
	application.Window.ShowAndRun()
}
