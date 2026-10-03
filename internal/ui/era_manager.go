//go:build !js

package ui

import (
	"encoding/json"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/SurendraNaresh/mytodo/internal/api"
)

func (s *AppState) showEraManager() {
	if !s.Session.IsAdmin() {
		dialog.ShowError(fmt.Errorf("administrator access required"), s.Window)
		return
	}
	client, err := s.uploadClient()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	slug := widget.NewEntry()
	slug.SetPlaceHolder("first-year")
	title := widget.NewEntry()
	title.SetPlaceHolder("First Year")
	sortOrder := widget.NewEntry()
	sortOrder.SetText("1")
	theme := widget.NewMultiLineEntry()
	theme.SetText(`{"accent":"#df6049","bg":"#f5f4ef","font":"Fraunces"}`)
	eraStatus := widget.NewLabel("")
	albumEra := widget.NewSelect(nil, nil)
	albumTitle := widget.NewEntry()
	albumTitle.SetPlaceHolder("First steps")
	albumOrder := widget.NewEntry()
	albumOrder.SetText("1")
	albumStatus := widget.NewLabel("")
	erasByLabel := make(map[string]api.Era)

	refreshEras := func(selectFirst bool) {
		eras, err := client.Eras()
		if err != nil {
			eraStatus.SetText(err.Error())
			return
		}
		labels := make([]string, len(eras))
		erasByLabel = make(map[string]api.Era, len(eras))
		for index, era := range eras {
			label := fmt.Sprintf("%d - %s", era.ID, era.Title)
			labels[index] = label
			erasByLabel[label] = era
		}
		albumEra.Options = labels
		if selectFirst && len(labels) > 0 {
			albumEra.SetSelected(labels[0])
		}
		albumEra.Refresh()
	}
	refreshEras(true)

	createEra := widget.NewButton("Create era", func() {
		order, err := strconv.ParseInt(sortOrder.Text, 10, 64)
		if err != nil || !json.Valid([]byte(theme.Text)) {
			eraStatus.SetText("Sort order must be a number and theme must be valid JSON.")
			return
		}
		_, err = client.CreateEra(api.Era{Slug: slug.Text, Title: title.Text, SortOrder: order, Theme: json.RawMessage(theme.Text)})
		if err != nil {
			eraStatus.SetText(err.Error())
			return
		}
		eraStatus.SetText("Era created.")
		refreshEras(true)
	})
	createAlbum := widget.NewButton("Create album", func() {
		era, ok := erasByLabel[albumEra.Selected]
		if !ok {
			albumStatus.SetText("Choose an era first.")
			return
		}
		order, err := strconv.ParseInt(albumOrder.Text, 10, 64)
		if err != nil {
			albumStatus.SetText("Sort order must be a number.")
			return
		}
		_, err = client.CreateAlbum(era.ID, api.Album{Title: albumTitle.Text, SortOrder: order})
		if err != nil {
			albumStatus.SetText(err.Error())
			return
		}
		albumStatus.SetText("Album created.")
	})

	content := container.NewVBox(
		widget.NewLabel("New era"),
		widget.NewForm(
			widget.NewFormItem("Slug", slug),
			widget.NewFormItem("Title", title),
			widget.NewFormItem("Sort order", sortOrder),
			widget.NewFormItem("Theme (JSON)", theme),
		),
		container.NewHBox(createEra, eraStatus),
		widget.NewSeparator(),
		widget.NewLabel("New album"),
		widget.NewForm(
			widget.NewFormItem("Era", albumEra),
			widget.NewFormItem("Title", albumTitle),
			widget.NewFormItem("Sort order", albumOrder),
		),
		container.NewHBox(createAlbum, albumStatus),
	)
	dialog.NewCustom("Manage archive", "Close", container.NewVScroll(content), s.Window).Show()
}
