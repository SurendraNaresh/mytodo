package ui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/db"
)

type votingEventRow struct {
	id          int64
	title       string
	description string
	opensAt     string
	closesAt    string
}

func (s *AppState) ShowVoting() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}

	title := widget.NewEntry()
	description := widget.NewMultiLineEntry()
	opensDate := widget.NewDateEntry()
	closesDate := widget.NewDateEntry()
	opensDate.SetPlaceHolder("YYYY-MM-DD")
	closesDate.SetPlaceHolder("YYYY-MM-DD")
	opensTime := widget.NewEntry()
	closesTime := widget.NewEntry()
	opensTime.SetPlaceHolder("HH:MM")
	closesTime.SetPlaceHolder("HH:MM")
	createStatus := widget.NewLabel("")
	voteStatus := widget.NewLabel("")
	choice := widget.NewEntry()
	choice.SetPlaceHolder("Enter your vote")
	selectedEventID := int64(0)
	const noEventSelected = "Select an event"

	prompt := widget.NewLabel("Select a voting event to add or review your vote.")
	votePanel := container.NewVBox(
		widget.NewLabelWithStyle("Your vote", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(widget.NewFormItem("Choice", choice)),
	)
	votePanel.Hide()
	eventIDs := make(map[string]int64)
	var eventEditor *fyne.Container
	var rightPanel *fyne.Container
	eventFrom := widget.NewDateEntry()
	eventTo := widget.NewDateEntry()
	eventFrom.SetPlaceHolder("YYYY-MM-DD")
	eventTo.SetPlaceHolder("YYYY-MM-DD")
	eventFilterStatus := widget.NewLabel("")
	allEvents := make([]votingEventRow, 0)

	loadUserVote := func() {
		var savedChoice string
		err := db.DB().QueryRow(`
			SELECT choice FROM vote
			WHERE voting_event_id = ? AND voter_user_id = ?`,
			selectedEventID, s.Session.User.ID,
		).Scan(&savedChoice)
		switch err {
		case nil:
			choice.SetText(savedChoice)
			voteStatus.SetText("Your vote is saved. You can update it here.")
		case sql.ErrNoRows:
			choice.SetText("")
			voteStatus.SetText("No vote submitted for this event yet.")
		default:
			voteStatus.SetText(err.Error())
		}
	}

	saveVote := widget.NewButton("Save vote", func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		if selectedEventID == 0 {
			voteStatus.SetText("Select an event first")
			return
		}
		vote := strings.TrimSpace(choice.Text)
		if vote == "" {
			voteStatus.SetText("A vote choice is required")
			return
		}
		_, err := db.DB().Exec(`
			INSERT INTO vote (voting_event_id, voter_user_id, choice)
			VALUES (?, ?, ?)
			ON CONFLICT (voting_event_id, voter_user_id)
			DO UPDATE SET choice = excluded.choice`,
			selectedEventID, s.Session.User.ID, vote,
		)
		if err != nil {
			voteStatus.SetText(err.Error())
			return
		}
		voteStatus.SetText("Vote saved")
	})
	votePanel.Add(saveVote)
	votePanel.Add(voteStatus)

	eventSelect := widget.NewSelect(nil, func(option string) {
		eventID, ok := eventIDs[option]
		if !ok {
			selectedEventID = 0
			votePanel.Hide()
			if eventEditor != nil {
				eventEditor.Hide()
			}
			prompt.Show()
			if rightPanel != nil {
				rightPanel.Refresh()
			}
			return
		}
		selectedEventID = eventID
		loadUserVote()
		prompt.Hide()
		if eventEditor != nil {
			eventEditor.Hide()
		}
		votePanel.Show()
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	})
	eventSelect.PlaceHolder = noEventSelected

	loadEvents := func() {
		rows, err := db.DB().Query(`
			SELECT id, title, COALESCE(description, ''), opens_at, closes_at
			FROM voting_event ORDER BY opens_at, id`)
		if err != nil {
			createStatus.SetText(err.Error())
			return
		}

		loadedEvents := make([]votingEventRow, 0)
		for rows.Next() {
			var event votingEventRow
			if err := rows.Scan(&event.id, &event.title, &event.description, &event.opensAt, &event.closesAt); err != nil {
				rows.Close()
				createStatus.SetText(err.Error())
				return
			}
			loadedEvents = append(loadedEvents, event)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			createStatus.SetText(err.Error())
			return
		}
		if err := rows.Close(); err != nil {
			createStatus.SetText(err.Error())
			return
		}

		allEvents = loadedEvents
		options := []string{noEventSelected}
		nextIDs := make(map[string]int64)
		for _, event := range allEvents {
			if !eventOverlapsDateRange(event.opensAt, event.closesAt, eventFrom.Date, eventTo.Date) {
				continue
			}
			option := fmt.Sprintf("%s | %s to %s (#%d)", event.title, event.opensAt, event.closesAt, event.id)
			options = append(options, option)
			nextIDs[option] = event.id
		}
		eventFilterStatus.SetText(fmt.Sprintf("%d events", len(options)-1))
		eventIDs = nextIDs
		eventSelect.SetOptions(options)
		currentOption := noEventSelected
		for option, eventID := range eventIDs {
			if eventID == selectedEventID {
				currentOption = option
				break
			}
		}
		if currentOption == noEventSelected && selectedEventID != 0 {
			selectedEventID = 0
			votePanel.Hide()
			prompt.Show()
		}
		eventSelect.SetSelected(currentOption)
	}
	eventFrom.OnChanged = func(*time.Time) {
		if !dateRangeValid(eventFrom.Date, eventTo.Date) {
			eventFilterStatus.SetText("Start date must be on or before end date")
			return
		}
		loadEvents()
	}
	eventTo.OnChanged = func(*time.Time) {
		if !dateRangeValid(eventFrom.Date, eventTo.Date) {
			eventFilterStatus.SetText("Start date must be on or before end date")
			return
		}
		loadEvents()
	}
	clearEventFilter := widget.NewButton("Clear dates", func() {
		eventFrom.SetDate(nil)
		eventTo.SetDate(nil)
		loadEvents()
	})

	clear := func() {
		title.SetText("")
		description.SetText("")
		opensDate.SetDate(nil)
		closesDate.SetDate(nil)
		opensTime.SetText("")
		closesTime.SetText("")
	}

	add := widget.NewButton("Add event", func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		if !s.Session.IsAdmin() {
			createStatus.SetText("Only administrators can create voting events")
			return
		}
		if strings.TrimSpace(title.Text) == "" || opensDate.Date == nil || closesDate.Date == nil {
			createStatus.SetText("Title, opening time, and closing time are required")
			return
		}
		if _, err := time.Parse("15:04", strings.TrimSpace(opensTime.Text)); err != nil {
			createStatus.SetText("Opening time must use 24-hour HH:MM format")
			return
		}
		if _, err := time.Parse("15:04", strings.TrimSpace(closesTime.Text)); err != nil {
			createStatus.SetText("Closing time must use 24-hour HH:MM format")
			return
		}
		opensAt := opensDate.Date.Format("2006-01-02") + " " + strings.TrimSpace(opensTime.Text)
		closesAt := closesDate.Date.Format("2006-01-02") + " " + strings.TrimSpace(closesTime.Text)
		if closesAt <= opensAt {
			createStatus.SetText("Closing time must be after opening time")
			return
		}
		_, err := db.DB().Exec(`INSERT INTO voting_event (title, description, opens_at, closes_at) VALUES (?, ?, ?, ?)`, strings.TrimSpace(title.Text), strings.TrimSpace(description.Text), opensAt, closesAt)
		if err != nil {
			createStatus.SetText(err.Error())
			return
		}
		clear()
		createStatus.SetText("Voting event added")
		selectedEventID = 0
		loadEvents()
		eventEditor.Hide()
		prompt.Show()
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	})

	newEvent := widget.NewButton("New event", func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		if !s.Session.IsAdmin() {
			return
		}
		clear()
		createStatus.SetText("")
		prompt.Hide()
		votePanel.Hide()
		eventEditor.Show()
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	})
	if !s.Session.IsAdmin() {
		newEvent.Disable()
	}

	cancelEvent := widget.NewButton("Cancel", func() {
		clear()
		createStatus.SetText("")
		eventEditor.Hide()
		if selectedEventID == 0 {
			prompt.Show()
		} else {
			votePanel.Show()
		}
		rightPanel.Refresh()
	})

	form := widget.NewForm(
		widget.NewFormItem("Title", title),
		widget.NewFormItem("Description", description),
		widget.NewFormItem("Opens", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), opensDate),
			container.NewVBox(widget.NewLabel("Time"), opensTime),
		)),
		widget.NewFormItem("Closes", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), closesDate),
			container.NewVBox(widget.NewLabel("Time"), closesTime),
		)),
	)
	eventEditor = container.NewBorder(
		widget.NewLabelWithStyle("Add voting event", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(container.NewHBox(cancelEvent, add), createStatus),
		nil,
		nil,
		form,
	)
	eventEditor.Hide()
	rightPanel = container.NewStack(prompt, votePanel, eventEditor)

	eventPane := container.NewBorder(
		widget.NewLabel("Voting event"),
		container.NewVBox(
			newEvent,
			widget.NewLabel("Event date range"),
			container.NewGridWithColumns(2, eventFrom, eventTo),
			container.NewHBox(clearEventFilter, eventFilterStatus),
		),
		nil,
		nil,
		eventSelect,
	)
	split := container.NewHSplit(eventPane, rightPanel)
	split.Offset = 0.38
	s.setMain(split)
	loadEvents()
}
