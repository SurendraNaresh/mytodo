package ui

import (
	"encoding/json"
	"fmt"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
)

func (s *AppState) showLayoutFrames() {
	if !s.Session.IsAdmin() {
		dialog.ShowError(fmt.Errorf("administrator access required"), s.Window)
		return
	}
	regions := []string{"top", "bottom", "left", "right", "center"}
	features := []string{"timeline", "era_title", "album_grid", "media_gallery", "comments", "donate_qr", "empty"}
	region := widget.NewSelect(regions, nil)
	feature := widget.NewSelect(features, nil)
	visible := widget.NewCheck("Visible", nil)
	config := widget.NewMultiLineEntry()
	config.SetPlaceHolder(`{"key":"value"}`)
	status := widget.NewLabel("")

	loadRegion := func(selected string) {
		var frame api.LayoutFrame
		var err error
		if api.Enabled() {
			var frames []api.LayoutFrame
			client, clientErr := api.Default()
			if clientErr == nil {
				frames, clientErr = client.LayoutFrames()
			}
			err = clientErr
			for _, current := range frames {
				if current.Region == selected {
					frame = current
					break
				}
			}
		} else {
			var raw string
			err = db.DB().QueryRow(`SELECT region, feature, visible, config_json FROM layout_frame WHERE region = ?`, selected).
				Scan(&frame.Region, &frame.Feature, &frame.Visible, &raw)
			frame.Config = json.RawMessage(raw)
		}
		if err != nil {
			status.SetText(err.Error())
			return
		}
		feature.SetSelected(frame.Feature)
		visible.SetChecked(frame.Visible)
		config.SetText(string(frame.Config))
		status.SetText("")
	}
	region.OnChanged = loadRegion
	region.SetSelected("top")
	loadRegion("top")

	save := widget.NewButton("Save frame", func() {
		if !json.Valid([]byte(config.Text)) {
			status.SetText("Configuration must be valid JSON.")
			return
		}
		frame := api.LayoutFrame{
			Region: region.Selected, Feature: feature.Selected, Visible: visible.Checked,
			Config: json.RawMessage(config.Text),
		}
		var err error
		if api.Enabled() {
			var client *api.Client
			client, err = api.Default()
			if err == nil {
				err = client.SaveLayoutFrame(frame)
			}
		} else {
			_, err = db.DB().Exec(`INSERT INTO layout_frame (region, feature, visible, config_json) VALUES (?, ?, ?, ?)
				ON CONFLICT(region) DO UPDATE SET feature = excluded.feature, visible = excluded.visible, config_json = excluded.config_json`,
				frame.Region, frame.Feature, frame.Visible, string(frame.Config))
		}
		if err != nil {
			status.SetText(err.Error())
			return
		}
		status.SetText("Frame saved.")
	})

	content := container.NewVBox(
		widget.NewLabel("Canvas layout"),
		widget.NewForm(
			widget.NewFormItem("Region", region),
			widget.NewFormItem("Feature", feature),
			widget.NewFormItem("", visible),
			widget.NewFormItem("Config (JSON)", config),
		),
		container.NewHBox(save, status),
	)
	dialog.NewCustom("Canvas layout", "Close", container.NewVScroll(content), s.Window).Show()
}
