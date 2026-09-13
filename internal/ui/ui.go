package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	"github.com/SurendraNaresh/mytodo/internal/auth"
	"github.com/SurendraNaresh/mytodo/internal/db"
)

type AppState struct {
	App    fyne.App
	Window fyne.Window

	Session *auth.Session

	Content  *fyne.Container
	Main     *fyne.Container
	FirstRun bool
}

func NewApp() *AppState {
	a := app.NewWithID("com.mytodo.desktop")
	w := a.NewWindow("Todo Master")

	if !fyne.CurrentDevice().IsMobile() {
		w.Resize(fyne.NewSize(900, 600))
	}
	state := &AppState{
		App:     a,
		Window:  w,
		Session: auth.NewSession(),
		Content: container.NewStack(),
		Main:    container.NewStack(),
	}
	if exists, err := db.DatabaseExists(); err == nil {
		state.FirstRun = !exists
	}

	state.ShowLogin()

	w.SetContent(state.Content)

	return state
}

func (s *AppState) setContent(obj fyne.CanvasObject) {
	s.Content.Objects = []fyne.CanvasObject{obj}
	s.Content.Refresh()
}

func (s *AppState) setMain(obj fyne.CanvasObject) {
	s.Main.Objects = []fyne.CanvasObject{obj}
	s.Main.Refresh()
}
