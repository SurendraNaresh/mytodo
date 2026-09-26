package user_management

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewUserView() fyne.CanvasObject {

	username := widget.NewEntry()

	email := widget.NewEntry()

	form := widget.NewForm(

		widget.NewFormItem("Username", username),

		widget.NewFormItem("Email", email),
	)
	return container.NewVScroll(form)
}
