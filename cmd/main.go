package main

import (
	"log"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	//if err := db.Open("tasks.db"); err != nil {
	// Creates mytodo.db if it does not exist.
	// No administrator/root privileges required.
	if err := db.Open(); err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	
	todo := ui.NewApp()

	todo.Window.ShowAndRun()
}
