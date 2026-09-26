package voting

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewVotingEventView is a deliberately small generated starting point.
// Add domain-specific layout and validation in a hand-written companion file.
func NewVotingEventView() fyne.CanvasObject {

	title := widget.NewEntry()

	description := widget.NewEntry()

	opensAtDate := widget.NewDateEntry()
	opensAtDate.SetPlaceHolder("YYYY-MM-DD")
	opensAtTime := widget.NewEntry()
	opensAtTime.SetPlaceHolder("HH:MM")

	closesAtDate := widget.NewDateEntry()
	closesAtDate.SetPlaceHolder("YYYY-MM-DD")
	closesAtTime := widget.NewEntry()
	closesAtTime.SetPlaceHolder("HH:MM")

	form := widget.NewForm(

		widget.NewFormItem("Title", title),

		widget.NewFormItem("Description", description),

		widget.NewFormItem("Opens", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), opensAtDate),
			container.NewVBox(widget.NewLabel("Time"), opensAtTime),
		)),

		widget.NewFormItem("Closes", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), closesAtDate),
			container.NewVBox(widget.NewLabel("Time"), closesAtTime),
		)),
	)
	return container.NewVScroll(form)
}
