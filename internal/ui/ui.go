package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/auth"
	"github.com/SurendraNaresh/mytodo/internal/db"
)

type lightGreenTheme struct {
	fyne.Theme
}

func (t lightGreenTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.White
	case theme.ColorNameButton:
		return color.NRGBA{R: 232, G: 245, B: 233, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 46, G: 125, B: 50, A: 255}
	default:
		return t.Theme.Color(name, variant)
	}
}

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
	a.Settings().SetTheme(lightGreenTheme{Theme: theme.LightTheme()})
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
	if !api.Enabled() {
		if exists, err := db.DatabaseExists(); err == nil {
			state.FirstRun = !exists
		}
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
