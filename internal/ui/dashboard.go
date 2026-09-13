package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (s *AppState) ShowDashboard() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}

	s.Main = container.NewStack()

	user := s.Session.User

	title := widget.NewLabelWithStyle(
		"Todo Master",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	admin := widget.NewButton(
		"Admin / Users",
		func() {
			s.ShowUsers()
		},
	)

	if !s.Session.IsAdmin() {
		admin.Disable()
	}

	profile := widget.NewButton(
		"User / Profile",
		func() {
			s.ShowCurrentUser()
		},
	)

	todo := widget.NewButton(
		fmt.Sprintf("Todo: %s", user.Name),
		func() {
			s.ShowTodo()
		},
	)

	logout := widget.NewButton(
		"Logout",
		func() {
			s.Session.Logout()
			s.ShowLogin()
		},
	)

	sidebar := container.NewVBox(
		title,
		widget.NewSeparator(),

		widget.NewLabel(
			fmt.Sprintf(
				"%s\n[%s]",
				user.Name,
				user.Role,
			),
		),

		widget.NewSeparator(),

		admin,
		profile,
		todo,

		widget.NewSeparator(),
		logout,
	)

	menu := widget.NewButtonWithIcon("", theme.MenuIcon(), func() {
		if sidebar.Visible() {
			sidebar.Hide()
		} else {
			sidebar.Show()
		}
	})
	menu.Importance = widget.LowImportance

	body := container.NewBorder(
		nil,
		nil,
		sidebar,
		nil,
		s.Main,
	)
	sidebar.Hide()

	root := container.NewBorder(
		container.NewHBox(menu),
		nil,
		nil,
		nil,
		body,
	)

	s.setContent(root)

	s.ShowTodo()
}
