//go:build js

package main

import (
	"log"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/model"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	if err := api.ConfigureFromBrowser(); err != nil {
		log.Fatal(err)
	}
	model.UseRemoteAPI(true)
	application := ui.NewApp()
	application.Window.ShowAndRun()
}
