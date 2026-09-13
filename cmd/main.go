package main

import (
	"log"
	"os"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	//if err := db.Open("tasks.db"); err != nil {
	// Creates mytodo.db if it does not exist.
	// No administrator/root privileges required.
	if err := db.OpenWithRestore(os.Getenv("MYTODO_RESTORE_FROM")); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	todo := ui.NewApp()

	todo.Window.ShowAndRun()
}
