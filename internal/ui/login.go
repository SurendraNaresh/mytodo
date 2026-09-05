package ui

import (
	"database/sql"
	"fmt"

	"fyne.io/fyne/v2"	
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/model"
)
const loginFieldWidth float32 = 360
const loginWidth float32 = 460

func (s *AppState) ShowLogin() {
	email := widget.NewEntry()
	email.SetPlaceHolder("user@example.com")
	email.Resize(fyne.NewSize(loginFieldWidth, email.MinSize().Height))
	pass := widget.NewPasswordEntry()
	email.Resize(fyne.NewSize(loginFieldWidth, pass.MinSize().Height))
	status := widget.NewLabel("")
	status.Alignment = fyne.TextAlignCenter

	login := widget.NewButton("Login", func() {
		err := s.Session.Login(
			email.Text,
			pass.Text,
		)

		if err != nil {
			status.SetText(err.Error())
			return
		}

		s.ShowDashboard()
	})

	login.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Email", 	email),
		widget.NewFormItem("Password", 	pass),
	)

	users, _ := model.GetUsers()

	var bootstrap *widget.Button

	if len(users) == 0 {
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

	if bootstrap != nil {
		items = append(items, bootstrap)
	}

	card := widget.NewCard(
		"Login",
		"",
		container.NewVBox(items...),
	)

	// Give the card a sensible fixed size.
	card.Resize(fyne.NewSize(640, 540))

	// Center the complete card inside the available window.
	content := container.NewCenter(card)
	
	s.setContent(content)
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

	form.Resize(fyne.NewSize(450, 300))
	form.Show()
}
