package main

import (
	"log"
	"os"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	if err := db.OpenWithRestore(os.Getenv("MYTODO_RESTORE_FROM")); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	application := ui.NewApp()
	application.Window.ShowAndRun()
}
