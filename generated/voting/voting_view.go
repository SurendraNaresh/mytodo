package voting

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewVotingEventView is a deliberately small generated starting point.
// Add domain-specific layout and validation in a hand-written companion file.
func NewVotingEventView() fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, nil, widget.NewLabel("Voting event"))
}
