package ui

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/model"
)

func (s *AppState) ShowUsers() {
	if !s.Session.IsAdmin() {
		s.setMain(
			container.NewCenter(
				widget.NewLabel(
					"Administrator access required",
				),
			),
		)
		return
	}

	users, err := model.GetUsers()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	allUsers := users

	var selected *model.User

	name := widget.NewEntry()
	dob := widget.NewDateEntry()
	email := widget.NewEntry()
	password := widget.NewPasswordEntry()

	dob.SetPlaceHolder("YYYY-MM-DD")
	password.SetPlaceHolder("unchanged when blank")

	roleNames := make([]string, 0)

	for _, role := range model.Roles() {
		roleNames = append(roleNames, string(role))
	}

	roleSelect := widget.NewSelect(
		roleNames,
		nil,
	)

	roleSelect.SetSelected(string(model.RoleMember))

	parentMap := map[string]int64{
		"<none>": 0,
	}

	parentNames := []string{"<none>"}

	for _, u := range users {
		label := fmt.Sprintf(
			"%s [%d]",
			u.Name,
			u.ID,
		)

		parentNames = append(parentNames, label)
		parentMap[label] = u.ID
	}

	parentSelect := widget.NewSelect(
		parentNames,
		nil,
	)

	parentSelect.SetSelected("<none>")

	status := widget.NewLabel("")

	clearForm := func() {
		selected = nil

		name.SetText("")
		dob.SetDate(nil)
		email.SetText("")
		password.SetText("")

		roleSelect.SetSelected(
			string(model.RoleMember),
		)

		parentSelect.SetSelected("<none>")
	}

	fillForm := func(u model.User) {
		copyUser := u
		selected = &copyUser

		name.SetText(u.Name)
		if date, ok := parseStoredDate(u.DOB); ok {
			dob.SetDate(&date)
		} else {
			dob.SetDate(nil)
		}
		email.SetText(u.Email)
		password.SetText("")
		roleSelect.SetSelected(string(u.Role))

		parentSelect.SetSelected("<none>")

		if u.ParentID.Valid {
			for label, id := range parentMap {
				if id == u.ParentID.Int64 {
					parentSelect.SetSelected(label)
					break
				}
			}
		}
	}

	list := widget.NewList(
		func() int {
			return len(users)
		},

		func() fyne.CanvasObject {
			return container.NewVBox(widget.NewLabel(""), widget.NewLabel(""))
		},

		func(id widget.ListItemID, obj fyne.CanvasObject) {
			u := users[id]

			box := obj.(*fyne.Container)

			box.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%d | %s", u.ID, u.Role))
			box.Objects[1].(*widget.Label).SetText(u.Email)
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(users) {
			return
		}

		fillForm(users[id])
	}

	dobFrom := widget.NewDateEntry()
	dobTo := widget.NewDateEntry()
	filterStatus := widget.NewLabel("")
	applyDateFilter := func() {
		if !dateRangeValid(dobFrom.Date, dobTo.Date) {
			users = nil
			filterStatus.SetText("Start date must be on or before end date")
			list.Refresh()
			return
		}
		users = make([]model.User, 0, len(allUsers))
		for _, user := range allUsers {
			if matchesDateRange(user.DOB, dobFrom.Date, dobTo.Date) {
				users = append(users, user)
			}
		}
		filterStatus.SetText(fmt.Sprintf("%d users", len(users)))
		list.Refresh()
	}
	dobFrom.OnChanged = func(*time.Time) { applyDateFilter() }
	dobTo.OnChanged = func(*time.Time) { applyDateFilter() }
	clearDateFilter := widget.NewButton("Clear dates", func() {
		dobFrom.SetDate(nil)
		dobTo.SetDate(nil)
		applyDateFilter()
	})
	userFilters := container.NewVBox(
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("DOB from"), dobFrom),
			container.NewVBox(widget.NewLabel("DOB to"), dobTo),
		),
		container.NewHBox(clearDateFilter, filterStatus),
	)
	applyDateFilter()

	form := widget.NewForm(
		widget.NewFormItem("Name", name),
		widget.NewFormItem("DOB", dob),
		widget.NewFormItem("Email", email),
		widget.NewFormItem("Role", roleSelect),
		widget.NewFormItem("Parent", parentSelect),
		widget.NewFormItem("Password", password),
	)

	reload := func() {
		s.ShowUsers()
	}

	newButton := widget.NewButton(
		"New",
		func() {
			clearForm()
			list.UnselectAll()
			status.SetText("New user")
		},
	)

	saveButton := widget.NewButton(
		"Save",
		func() {
			dobValue := ""
			if dob.Date != nil {
				dobValue = dob.Date.Format("2006-01-02")
			} else if strings.TrimSpace(dob.Text) != "" {
				status.SetText("Select a valid date of birth")
				return
			}

			var parentID sql.NullInt64

			if id := parentMap[parentSelect.Selected]; id != 0 {
				parentID = sql.NullInt64{
					Int64: id,
					Valid: true,
				}
			}

			if selected == nil {
				if password.Text == "" {
					status.SetText(
						"Password required for new user",
					)
					return
				}

				_, err := model.CreateUser(
					name.Text,
					dobValue,
					email.Text,
					model.Role(roleSelect.Selected),
					password.Text,
					parentID,
				)

				if err != nil {
					status.SetText(err.Error())
					return
				}

				reload()
				return
			}

			selected.Name = name.Text
			selected.DOB = dobValue
			selected.Email = email.Text
			selected.Role = model.Role(
				roleSelect.Selected,
			)
			selected.ParentID = parentID

			if selected.ParentID.Valid &&
				selected.ParentID.Int64 == selected.ID {

				status.SetText(
					"a user cannot be their own parent",
				)
				return
			}

			err := model.UpdateUser(
				selected,
				password.Text,
			)

			if err != nil {
				status.SetText(err.Error())
				return
			}

			// If currently logged in user was edited,
			// refresh the session copy.
			if selected.ID == s.Session.User.ID {
				s.Session.User = selected
			}

			reload()
		},
	)

	deleteButton := widget.NewButton(
		"Delete",
		func() {
			if selected == nil {
				status.SetText("Select a user")
				return
			}

			if selected.ID == s.Session.User.ID {
				status.SetText(
					"cannot delete logged-in user",
				)
				return
			}

			msg := fmt.Sprintf(
				"Delete user %s (#%s)?",
				selected.Name,
				strconv.FormatInt(selected.ID, 10),
			)

			dialog.ShowConfirm(
				"Delete User",
				msg,
				func(ok bool) {
					if !ok {
						return
					}

					if err := model.DeleteUser(
						selected.ID,
					); err != nil {
						dialog.ShowError(
							err,
							s.Window,
						)
						return
					}

					reload()
				},
				s.Window,
			)
		},
	)

	deleteButton.Importance = widget.DangerImportance

	buttons := container.NewHBox(
		newButton,
		saveButton,
		deleteButton,
	)

	editor := container.NewBorder(
		widget.NewLabelWithStyle(
			"User CRUD",
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		),
		container.NewVBox(
			buttons,
			status,
		),
		nil,
		nil,
		form,
	)

	split := container.NewHSplit(
		container.NewBorder(
			userFilters,
			nil,
			nil,
			nil,
			list,
		),
		editor,
	)

	split.Offset = 0.38

	s.setMain(split)
}

func (s *AppState) ShowCurrentUser() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}

	u := s.Session.User

	parent := "None"

	if u.ParentID.Valid {
		if p, err := model.GetUser(
			u.ParentID.Int64,
		); err == nil {

			parent = p.Name
		}
	}

	content := container.NewVBox(
		widget.NewLabelWithStyle(
			"Current User",
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		),

		widget.NewSeparator(),

		widget.NewLabel(
			"Name: "+u.Name,
		),

		widget.NewLabel(
			"Email: "+u.Email,
		),

		widget.NewLabel(
			"DOB: "+u.DOB,
		),

		widget.NewLabel(
			"Role: "+string(u.Role),
		),

		widget.NewLabel(
			"Parent: "+parent,
		),
	)

	s.setMain(content)
}
