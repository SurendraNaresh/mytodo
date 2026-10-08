package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/api"
)

func (s *AppState) ShowPasswordAdmin() {
	if !s.Session.IsAdmin() || !api.Enabled() {
		s.setMain(container.NewCenter(widget.NewLabel("Administrator access required")))
		return
	}
	client, err := api.Default()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	policy, err := client.PasswordPolicy()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	minimumLength := widget.NewEntry()
	minimumLength.SetText(strconv.Itoa(policy.MinimumLength))
	requireSpecial := widget.NewCheck("Require one of %$#@!^&~`", nil)
	requireSpecial.SetChecked(policy.RequireSpecial)
	requireMixedCase := widget.NewCheck("Require upper and lowercase", nil)
	requireMixedCase.SetChecked(policy.RequireMixedCase)
	status := widget.NewLabel("")
	savePolicy := widget.NewButton("Save password rules", func() {
		length, err := strconv.Atoi(minimumLength.Text)
		if err != nil {
			status.SetText("Minimum length must be a number")
			return
		}
		err = client.SavePasswordPolicy(api.PasswordPolicy{
			MinimumLength: length, RequireSpecial: requireSpecial.Checked, RequireMixedCase: requireMixedCase.Checked,
		})
		if err != nil {
			status.SetText(err.Error())
			return
		}
		status.SetText("Password rules saved")
	})
	policyForm := container.NewVBox(minimumLength, requireSpecial, requireMixedCase, savePolicy, status)

	requests, err := client.PasswordResetRequests()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	selected := -1
	requestList := widget.NewList(
		func() int { return len(requests) },
		func() fyne.CanvasObject { return widget.NewLabel("request") },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			request := requests[id]
			object.(*widget.Label).SetText(fmt.Sprintf("%s | %s | %s", request.Status, request.UserName, request.TimeframeStart))
		},
	)
	requestDetails := widget.NewLabel("Select a request")
	requestList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(requests) {
			return
		}
		selected = int(id)
		request := requests[selected]
		requestDetails.SetText(fmt.Sprintf("%s <%s>\n%s to %s\n%s", request.UserName, request.UserEmail, request.TimeframeStart, request.TimeframeEnd, request.Reason))
	}
	defaultPassword := widget.NewPasswordEntry()
	defaultPassword.SetPlaceHolder("Temporary default password")
	resetStatus := widget.NewLabel("")
	resetPassword := widget.NewButton("Set default password", func() {
		if selected < 0 || requests[selected].Status != "Pending" {
			resetStatus.SetText("Select a pending request")
			return
		}
		if err := client.ResetUserPassword(requests[selected].ID, defaultPassword.Text); err != nil {
			resetStatus.SetText(err.Error())
			return
		}
		resetStatus.SetText("Password reset complete; provide the temporary password to the user")
		requests[selected].Status = "Reset"
		requestList.Refresh()
		defaultPassword.SetText("")
	})
	resets := container.NewBorder(nil, nil, nil, nil, container.NewVBox(requestList, requestDetails, defaultPassword, resetPassword, resetStatus))
	s.setMain(container.NewVSplit(policyForm, resets))
}
