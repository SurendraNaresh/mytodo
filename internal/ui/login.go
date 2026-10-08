package ui

import (
	"database/sql"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/clientdata"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

const loginFieldWidth float32 = 360
const loginWidth float32 = 460

func (s *AppState) ShowLogin() {
	email := widget.NewEntry()
	email.SetPlaceHolder("user@example.com")
	pass := widget.NewPasswordEntry()
	apiURL := widget.NewEntry()
	apiURL.SetText(api.CurrentURL())
	limitEntry(email, 50)
	limitEntry(pass, 50)
	status := widget.NewLabel("")
	status.Alignment = fyne.TextAlignCenter

	login := widget.NewButton("Login", func() {
		if api.Enabled() {
			if err := api.Configure(apiURL.Text); err != nil {
				status.SetText(err.Error())
				return
			}
			if err := clientdata.SaveSetting("api_url", apiURL.Text); err != nil {
				status.SetText(err.Error())
				return
			}
		}
		err := s.Session.Login(
			email.Text,
			pass.Text,
		)

		if err != nil {
			status.SetText(err.Error())
			return
		}
		if api.Enabled() {
			if err := clientdata.SaveUser(s.Session.User); err != nil {
				status.SetText(err.Error())
				s.Session.Logout()
				return
			}
		}

		s.ShowDashboard()
	})

	login.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Email", container.NewGridWrap(fyne.NewSize(loginFieldWidth, email.MinSize().Height), email)),
		widget.NewFormItem("Password", container.NewGridWrap(fyne.NewSize(loginFieldWidth, pass.MinSize().Height), pass)),
	)

	needsSetup := false
	if !api.Enabled() {
		users, _ := model.GetUsers()
		needsSetup = len(users) == 0
	}

	var bootstrap *widget.Button

	if needsSetup {
		bootstrap = widget.NewButton(
			"Create first Admin",
			func() {
				s.showBootstrapAdmin()
			},
		)
	}

	items := []fyne.CanvasObject{
		widget.NewLabelWithStyle(
			"Todo Master",
			fyne.TextAlignCenter,
			fyne.TextStyle{Bold: true},
		),
		form,
		login,
		status,
	}

	cardContent := container.NewVBox(items...)
	if api.Enabled() {
		var connect *widget.Button
		connect = widget.NewButton("Check server", func() {
			if err := api.Configure(apiURL.Text); err != nil {
				status.SetText(err.Error())
				return
			}
			if err := clientdata.SaveSetting("api_url", apiURL.Text); err != nil {
				status.SetText(err.Error())
				return
			}
			client, err := api.Default()
			if err != nil {
				status.SetText(err.Error())
				return
			}
			connect.Disable()
			status.SetText("Checking server...")
			go func() {
				needsSetup, err := client.NeedsSetup()
				fyne.Do(func() {
					connect.Enable()
					if err != nil {
						status.SetText(err.Error())
						return
					}
					status.SetText("Server connected")
					if needsSetup && bootstrap == nil {
						bootstrap = widget.NewButton("Create first Admin", func() { s.showBootstrapAdmin() })
						cardContent.Add(bootstrap)
					}
				})
			}()
		})
		cardContent.Add(widget.NewForm(widget.NewFormItem("API server URL", apiURL)))
		cardContent.Add(connect)
	}
	if bootstrap != nil {
		cardContent.Add(bootstrap)
	}

	card := widget.NewCard(
		"Login",
		"",
		cardContent,
	)

	s.setContent(container.NewPadded(container.NewCenter(card)))
	if s.FirstRun {
		s.FirstRun = false
		s.showRestoreDatabase(false)
	}
}

func limitEntry(entry *widget.Entry, max int) {
	entry.OnChanged = func(value string) {
		runes := []rune(value)
		if len(runes) > max {
			entry.SetText(string(runes[:max]))
		}
	}
}

func (s *AppState) showRestoreDatabase(required bool) {
	message := "A new database will be created in the default application folder if you continue without selecting a backup."
	if required {
		message = "Select a SQLite database backup to replace the current database."
	}

	choose := widget.NewButton("Select database backup", func() {
		picker := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, s.Window)
				return
			}
			if reader == nil {
				return
			}
			path := reader.URI().Path()
			_ = reader.Close()

			if err := db.Close(); err != nil {
				dialog.ShowError(err, s.Window)
				return
			}
			if err := db.RestoreDatabase(path); err != nil {
				dialog.ShowError(err, s.Window)
				_ = db.Open()
				return
			}
			if err := db.Open(); err != nil {
				dialog.ShowError(err, s.Window)
				return
			}
			dialog.ShowInformation("Database restored", "The selected database is now active.", s.Window)
			s.ShowLogin()
		}, s.Window)
		picker.SetTitleText("Restore database")
		picker.SetConfirmText("Restore")
		picker.Show()
	})

	items := []fyne.CanvasObject{
		widget.NewLabelWithStyle("Restore database", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(message),
		choose,
	}
	if !required {
		items = append(items, widget.NewButton("Continue with new database", nil))
	}
	setupDialog := dialog.NewCustomWithoutButtons("Database setup", container.NewVBox(items...), s.Window)
	setupDialog.Show()
	if !required {
		continueButton := items[len(items)-1].(*widget.Button)
		continueButton.OnTapped = setupDialog.Dismiss
	}
}

func (s *AppState) showBootstrapAdmin() {
	name := widget.NewEntry()
	dob := widget.NewEntry()
	email := widget.NewEntry()
	password := widget.NewPasswordEntry()

	form := dialog.NewForm(
		"Create Administrator",
		"Create",
		"Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Name", name),
			widget.NewFormItem("DOB", dob),
			widget.NewFormItem("Email", email),
			widget.NewFormItem("Password", password),
		},
		func(ok bool) {
			if !ok {
				return
			}

			_, err := model.CreateUser(
				name.Text,
				dob.Text,
				email.Text,
				model.RoleAdmin,
				password.Text,
				sql.NullInt64{},
			)

			if err != nil {
				dialog.ShowError(err, s.Window)
				return
			}

			dialog.ShowInformation(
				"Administrator",
				fmt.Sprintf("Admin %s created", name.Text),
				s.Window,
			)

			s.ShowLogin()
		},
		s.Window,
	)

	form.Show()
}
