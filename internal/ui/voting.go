package ui

import (
	"database/sql"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/clientdata"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

type votingEventRow struct {
	id          int64
	ownerID     int64
	inviteeIDs  []int64
	voteCount   int
	title       string
	description string
	eventType   string
	eventClass  string
	eventDate   string
	opensAt     string
	closesAt    string
	isActive    bool
}

func (s *AppState) ShowVoting() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}
	s.setMain(s.votingView())
}

func (s *AppState) votingView() fyne.CanvasObject {
	admin := s.Session.IsAdmin()
	title := widget.NewEntry()
	description := widget.NewMultiLineEntry()
	eventTypes := []string{"Personal"}
	if admin {
		eventTypes = []string{"Vote", "Internal", "External", "Personal"}
	}
	eventType := widget.NewSelect(eventTypes, nil)
	if admin {
		eventType.SetSelected("Vote")
	} else {
		eventType.SetSelected("Personal")
	}
	eventClass := widget.NewSelect([]string{"Private", "Public"}, nil)
	if admin {
		eventClass.SetSelected("Public")
	} else {
		eventClass.SetSelected("Private")
	}
	opensDate := widget.NewDateEntry()
	closesDate := widget.NewDateEntry()
	eventDate := widget.NewDateEntry()
	eventDate.SetPlaceHolder("YYYY-MM-DD")
	opensDate.SetPlaceHolder("YYYY-MM-DD")
	closesDate.SetPlaceHolder("YYYY-MM-DD")
	opensTime, getOpensTime, setOpensTime := newTimeSpinner("06:00")
	closesTime, getClosesTime, setClosesTime := newTimeSpinner("22:00")
	eventStatus := widget.NewLabel("")
	voteStatus := widget.NewLabel("")
	selectedChoice := ""
	comments := widget.NewMultiLineEntry()
	comments.SetPlaceHolder("Required when abstaining")
	currentChoice := widget.NewLabel("Current choice: N/A")
	yesBar, noBar, abstainBar := widget.NewProgressBar(), widget.NewProgressBar(), widget.NewProgressBar()
	yesResult, noResult, abstainResult := widget.NewLabel("0 (0%)"), widget.NewLabel("0 (0%)"), widget.NewLabel("0 (0%)")
	resultTotal := widget.NewLabel("0 votes")
	selectedEventID := int64(0)
	var selectedEvent *votingEventRow
	displayedEvents := make([]votingEventRow, 0)
	var eventList *widget.List
	var editButton *widget.Button
	var deleteButton *widget.Button
	var deactivateButton *widget.Button
	var saveVoteButton *widget.Button
	var saveButton *widget.Button
	var eventEditorTitle *widget.Label
	var rightPanel *fyne.Container
	var eventEditor *fyne.Container
	var inviteeFields *fyne.Container
	var editingEventID int64
	var yesButton, noButton, abstainButton *widget.Button
	var availableInvitees []api.User
	selectedInvitees := make([]int64, 0)
	inviteeNames := make(map[int64]string)
	inviteeLabels := make(map[string]int64)
	inviteeCount := widget.NewSelect([]string{"0"}, nil)
	inviteeCount.SetSelected("0")
	inviteeSelect := widget.NewSelect(nil, nil)
	inviteeStatus := widget.NewLabel("")
	selectedInviteeList := container.NewVBox()

	var inviteeLoadErr error
	if api.Enabled() {
		client, err := api.Default()
		if err == nil {
			availableInvitees, inviteeLoadErr = client.EventInvitees()
		} else {
			inviteeLoadErr = err
		}
	} else {
		rows, err := db.DB().Query(`SELECT id, name, email, role FROM users WHERE role = ? AND id != ? ORDER BY name, id`, string(model.RoleMember), s.Session.User.ID)
		if err != nil {
			inviteeLoadErr = err
		} else {
			for rows.Next() {
				var user api.User
				if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Role); err != nil {
					inviteeLoadErr = err
					break
				}
				availableInvitees = append(availableInvitees, user)
			}
			if err := rows.Err(); err != nil {
				inviteeLoadErr = err
			}
			if err := rows.Close(); err != nil && inviteeLoadErr == nil {
				inviteeLoadErr = err
			}
		}
	}
	countOptions := []string{"0"}
	for index, user := range availableInvitees {
		label := fmt.Sprintf("%s [%d]", user.Name, user.ID)
		inviteeNames[user.ID] = label
		inviteeLabels[label] = user.ID
		countOptions = append(countOptions, fmt.Sprint(index+1))
	}
	inviteeCount.Options = countOptions
	if inviteeLoadErr != nil {
		inviteeStatus.SetText("Could not load invitees: " + inviteeLoadErr.Error())
	}
	var refreshInviteeOptions func()
	refreshInviteeOptions = func() {
		options := make([]string, 0, len(availableInvitees))
		for _, user := range availableInvitees {
			alreadySelected := false
			for _, id := range selectedInvitees {
				if id == user.ID {
					alreadySelected = true
					break
				}
			}
			if !alreadySelected {
				options = append(options, inviteeNames[user.ID])
			}
		}
		inviteeSelect.Options = options
		inviteeSelect.SetSelected("")
		selectedInviteeList.RemoveAll()
		for _, id := range selectedInvitees {
			userID := id
			removeButton := widget.NewButton("Remove "+inviteeNames[id], func() {
				for index, selectedID := range selectedInvitees {
					if selectedID == userID {
						selectedInvitees = append(selectedInvitees[:index], selectedInvitees[index+1:]...)
						break
					}
				}
				refreshInviteeOptions()
			})
			selectedInviteeList.Add(removeButton)
		}
		count, _ := strconv.Atoi(inviteeCount.Selected)
		if len(selectedInvitees) >= count || len(options) == 0 {
			inviteeSelect.Disable()
		} else {
			inviteeSelect.Enable()
		}
		selectedInviteeList.Refresh()
	}
	refreshInviteeOptions()
	inviteeCount.OnChanged = func(string) { refreshInviteeOptions() }
	inviteeSelect.OnChanged = func(label string) {
		if id, ok := inviteeLabels[label]; ok && len(selectedInvitees) < len(availableInvitees) {
			selectedInvitees = append(selectedInvitees, id)
			refreshInviteeOptions()
		}
	}
	updateSelectedInvitees := func(ids []int64) {
		selectedInvitees = append(selectedInvitees[:0], ids...)
		inviteeCount.SetSelected(fmt.Sprint(len(selectedInvitees)))
		refreshInviteeOptions()
	}

	var updateChoice func(string)
	yesButton = widget.NewButton("Yes", func() { updateChoice("Yes") })
	yesButton.Importance = widget.SuccessImportance
	noButton = widget.NewButton("No", func() { updateChoice("No") })
	noButton.Importance = widget.DangerImportance
	abstainButton = widget.NewButton("Abstain", func() { updateChoice("Abstain") })
	abstainButton.Importance = widget.HighImportance
	updateChoice = func(value string) { selectedChoice = value }
	choiceButtons := container.NewGridWithColumns(3, yesButton, noButton, abstainButton)

	if admin {
		comments.Disable()
		currentChoice.SetText("Current choice: N/A (admin)")
	}
	prompt := widget.NewLabel("Select an event.")
	votePanel := container.NewVBox(
		widget.NewLabelWithStyle("Your vote", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		currentChoice,
		widget.NewForm(
			widget.NewFormItem("Choice", choiceButtons),
			widget.NewFormItem("Comments", comments),
		),
	)
	resultsPanel := container.NewVBox(
		widget.NewLabelWithStyle("Current results", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(widget.NewLabel("Yes"), yesBar, yesResult),
		container.NewHBox(widget.NewLabel("No"), noBar, noResult),
		container.NewHBox(widget.NewLabel("Abstain"), abstainBar, abstainResult),
		resultTotal,
	)
	votePanel.Hide()
	eventInfo := widget.NewLabel("")
	eventInfo.Wrapping = fyne.TextWrapWord
	eventInfo.Hide()
	eventFilterStatus := widget.NewLabel("")

	showPrompt := func() {
		selectedEventID = 0
		selectedEvent = nil
		votePanel.Hide()
		eventInfo.Hide()
		if eventEditor != nil {
			eventEditor.Hide()
		}
		prompt.Show()
		if editButton != nil {
			editButton.Disable()
			deleteButton.Disable()
			deactivateButton.Disable()
		}
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	}

	loadUserVote := func() {
		if api.Enabled() {
			client, err := api.Default()
			if err != nil {
				voteStatus.SetText(err.Error())
				return
			}
			saved, err := client.GetVote(selectedEventID)
			if err != nil {
				voteStatus.SetText(err.Error())
				return
			}
			updateChoice(saved.Choice)
			comments.SetText(saved.Comments)
			if admin {
				currentChoice.SetText("Current choice: N/A (admin)")
			} else if saved.Choice == "" {
				currentChoice.SetText("Current choice: N/A")
			} else {
				currentChoice.SetText("Current choice: " + saved.Choice)
			}
			if saved.Choice == "" {
				voteStatus.SetText("No vote submitted for this event yet.")
			} else {
				voteStatus.SetText("Your vote is saved. You can update it here.")
			}
			return
		}
		var savedChoice, savedComments string
		err := db.DB().QueryRow(`
			SELECT choice, COALESCE(comments, '') FROM vote
			WHERE voting_event_id = ? AND voter_user_id = ?`,
			selectedEventID, s.Session.User.ID,
		).Scan(&savedChoice, &savedComments)
		switch err {
		case nil:
			updateChoice(savedChoice)
			comments.SetText(savedComments)
			currentChoice.SetText("Current choice: " + savedChoice)
			voteStatus.SetText("Your vote is saved. You can update it here.")
		case sql.ErrNoRows:
			updateChoice("")
			comments.SetText("")
			currentChoice.SetText("Current choice: N/A")
			voteStatus.SetText("No vote submitted for this event yet.")
		default:
			voteStatus.SetText(err.Error())
		}
	}

	loadVoteSummary := func() {
		var summary api.VoteSummary
		var err error
		if api.Enabled() {
			client, clientErr := api.Default()
			if clientErr != nil {
				voteStatus.SetText(clientErr.Error())
				return
			}
			summary, err = client.EventVoteSummary(selectedEventID)
		} else {
			err = db.DB().QueryRow(`SELECT
				COALESCE(SUM(choice = 'Yes'), 0),
				COALESCE(SUM(choice = 'No'), 0),
				COALESCE(SUM(choice = 'Abstain'), 0)
				FROM vote WHERE voting_event_id = ?`, selectedEventID).Scan(&summary.Yes, &summary.No, &summary.Abstain)
		}
		if err != nil {
			voteStatus.SetText(err.Error())
			return
		}
		total := summary.Yes + summary.No + summary.Abstain
		setResult := func(bar *widget.ProgressBar, label *widget.Label, count int) {
			percent := 0.0
			if total > 0 {
				percent = float64(count) / float64(total)
			}
			bar.SetValue(percent)
			label.SetText(fmt.Sprintf("%d (%d%%)", count, int(percent*100+0.5)))
		}
		setResult(yesBar, yesResult, summary.Yes)
		setResult(noBar, noResult, summary.No)
		setResult(abstainBar, abstainResult, summary.Abstain)
		resultTotal.SetText(fmt.Sprintf("%d votes", total))
	}

	showSelectedEvent := func(event votingEventRow) {
		selectedEvent = &event
		selectedEventID = event.id
		prompt.Hide()
		if eventEditor != nil {
			eventEditor.Hide()
		}
		if editButton != nil {
			ownsPersonal := event.eventType == "Personal" && event.ownerID == s.Session.User.ID
			locked := event.voteCount > 0 || eventIsActive(event, time.Now())
			if admin || ownsPersonal && !locked {
				editButton.Enable()
			} else {
				editButton.Disable()
			}
			if admin || ownsPersonal && event.isActive && !locked {
				deleteButton.Enable()
			} else {
				deleteButton.Disable()
			}
			if admin || ownsPersonal && event.isActive {
				deactivateButton.Enable()
			} else {
				deactivateButton.Disable()
			}
		}
		if event.eventType == "Vote" || event.eventType == "Personal" {
			eventInfo.Hide()
			votePanel.Show()
			canVote := !admin && (event.eventType == "Vote" || event.eventType == "Personal" && containsInvitee(event.inviteeIDs, s.Session.User.ID))
			if canVote {
				yesButton.Enable()
				noButton.Enable()
				abstainButton.Enable()
				comments.Enable()
			} else {
				yesButton.Disable()
				noButton.Disable()
				abstainButton.Disable()
				comments.Disable()
			}
			if event.isActive && eventIsActive(event, time.Now()) && canVote {
				saveVoteButton.Enable()
			} else {
				saveVoteButton.Disable()
			}
			if admin {
				currentChoice.SetText("Current choice: N/A (admin)")
			} else if canVote {
				loadUserVote()
			} else {
				currentChoice.SetText("Current choice: N/A")
			}
			if event.ownerID == s.Session.User.ID {
				resultsPanel.Show()
				loadVoteSummary()
			} else {
				resultsPanel.Hide()
			}
		} else {
			votePanel.Hide()
			eventInfo.SetText(fmt.Sprintf("%s\n%s\nType: %s\nClass: %s\nEvent date: %s\nOpens: %s\nCloses: %s\nStatus: %s", event.title, event.description, eventTypeText(event.eventType), event.eventClass, event.eventDate, event.opensAt, event.closesAt, eventStatusText(event)))
			eventInfo.Show()
		}
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	}

	saveVoteButton = widget.NewButton("Save vote", func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		if selectedEventID == 0 {
			voteStatus.SetText("Select an event first")
			return
		}
		if admin || selectedEvent == nil || selectedEvent.eventType != "Vote" && selectedEvent.eventType != "Personal" {
			voteStatus.SetText("Voting is available only for Vote events")
			return
		}
		if selectedEvent.eventType == "Personal" && !containsInvitee(selectedEvent.inviteeIDs, s.Session.User.ID) {
			voteStatus.SetText("Only invited Members can vote in this Personal event")
			return
		}
		if !selectedEvent.isActive || !eventIsActive(*selectedEvent, time.Now()) {
			voteStatus.SetText("Voting is available only while the event is open")
			return
		}
		if selectedChoice != "Yes" && selectedChoice != "No" && selectedChoice != "Abstain" {
			voteStatus.SetText("Choose Yes, No, or Abstain")
			return
		}
		comment := strings.TrimSpace(comments.Text)
		if selectedChoice == "Abstain" && comment == "" {
			voteStatus.SetText("Comments are required when abstaining")
			return
		}
		var err error
		if api.Enabled() {
			client, clientErr := api.Default()
			if clientErr != nil {
				voteStatus.SetText(clientErr.Error())
				return
			}
			err = client.SaveVote(selectedEventID, api.Vote{Choice: selectedChoice, Comments: comment})
		} else {
			_, err = db.DB().Exec(`
			INSERT INTO vote (voting_event_id, voter_user_id, choice, comments)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (voting_event_id, voter_user_id)
			DO UPDATE SET choice = excluded.choice, comments = excluded.comments`,
				selectedEventID, s.Session.User.ID, selectedChoice, comment,
			)
		}
		if err != nil {
			voteStatus.SetText(err.Error())
			return
		}
		voteStatus.SetText("Vote saved")
		currentChoice.SetText("Current choice: " + selectedChoice)
		if selectedEvent.ownerID == s.Session.User.ID {
			loadVoteSummary()
		}
	})
	votePanel.Add(saveVoteButton)
	votePanel.Add(voteStatus)
	votePanel.Add(resultsPanel)

	eventList = widget.NewList(
		func() int { return len(displayedEvents) },
		func() fyne.CanvasObject {
			return container.NewVBox(widget.NewLabel(""), widget.NewLabel(""), canvas.NewText("", color.NRGBA{R: 196, G: 34, B: 34, A: 255}))
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			event := displayedEvents[int(id)]
			row := object.(*fyne.Container)
			row.Objects[0].(*widget.Label).SetText(event.title)
			row.Objects[1].(*widget.Label).SetText(fmt.Sprintf("%s | %s | %s", eventTypeText(event.eventType), event.eventClass, eventStatusText(event)))
			reminder := row.Objects[2].(*canvas.Text)
			if event.isActive && eventIsActive(event, time.Now()) {
				reminder.Text = "ACTIVE - closes " + event.closesAt
			} else {
				reminder.Text = ""
			}
			reminder.Refresh()
		},
	)
	eventList.OnSelected = func(id widget.ListItemID) {
		if int(id) >= len(displayedEvents) {
			return
		}
		showSelectedEvent(displayedEvents[int(id)])
	}

	loadEvents := func() {
		loadedEvents := make([]votingEventRow, 0)
		if api.Enabled() {
			client, err := api.Default()
			if err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
			remoteEvents, err := client.Events()
			if err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
			_ = clientdata.SaveEvents(remoteEvents)
			for _, event := range remoteEvents {
				loadedEvents = append(loadedEvents, votingEventRow{
					id: event.ID, title: event.Title, description: event.Description,
					inviteeIDs: event.InviteeIDs,
					ownerID:    event.OwnerID, voteCount: event.VoteCount, isActive: event.IsActive,
					eventType: event.EventType, eventClass: event.EventClass, eventDate: event.EventDate, opensAt: event.OpensAt, closesAt: event.ClosesAt,
				})
			}
		} else {
			rows, err := db.DB().Query(`
			SELECT id, title, COALESCE(description, ''), event_type, event_class, event_date, opens_at, closes_at,
				COALESCE(owner_user_id, 0), is_active,
				(SELECT COUNT(*) FROM vote WHERE vote.voting_event_id = voting_event.id)
			FROM voting_event ORDER BY opens_at, id`)
			if err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
			for rows.Next() {
				var event votingEventRow
				if err := rows.Scan(&event.id, &event.title, &event.description, &event.eventType, &event.eventClass, &event.eventDate, &event.opensAt, &event.closesAt, &event.ownerID, &event.isActive, &event.voteCount); err != nil {
					rows.Close()
					eventFilterStatus.SetText(err.Error())
					return
				}
				if event.ownerID == s.Session.User.ID {
					event.inviteeIDs, err = eventInviteeIDs(event.id)
					if err != nil {
						rows.Close()
						eventFilterStatus.SetText(err.Error())
						return
					}
				} else if event.eventType == "Personal" {
					event.inviteeIDs = []int64{s.Session.User.ID}
				}
				loadedEvents = append(loadedEvents, event)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				eventFilterStatus.SetText(err.Error())
				return
			}
			if err := rows.Close(); err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
		}

		displayedEvents = visibleVotingEvents(loadedEvents, s.Session.User.ID, admin)
		eventFilterStatus.SetText(fmt.Sprintf("%d events", len(displayedEvents)))
		eventList.Refresh()
		if selectedEventID != 0 {
			for index, event := range displayedEvents {
				if event.id == selectedEventID {
					showSelectedEvent(event)
					eventList.Select(widget.ListItemID(index))
					return
				}
			}
			showPrompt()
		}
	}
	clear := func() {
		title.SetText("")
		description.SetText("")
		if admin {
			eventType.SetSelected("Vote")
			eventClass.SetSelected("Public")
		} else {
			eventType.SetSelected("Personal")
			eventClass.SetSelected("Private")
		}
		opensDate.SetDate(nil)
		closesDate.SetDate(nil)
		eventDate.SetDate(nil)
		setOpensTime("06:00")
		setClosesTime("22:00")
		updateSelectedInvitees(nil)
	}

	fillEditor := func(event votingEventRow) {
		title.SetText(event.title)
		description.SetText(event.description)
		updateSelectedInvitees(event.inviteeIDs)
		eventType.SetSelected(event.eventType)
		eventClass.SetSelected(event.eventClass)
		if selectedDate, err := time.Parse("2006-01-02", event.eventDate); err == nil {
			eventDate.SetDate(&selectedDate)
		}
		if opened, err := time.Parse("2006-01-02 15:04", event.opensAt); err == nil {
			opensDate.SetDate(&opened)
			setOpensTime(opened.Format("15:04"))
		}
		if closed, err := time.Parse("2006-01-02 15:04", event.closesAt); err == nil {
			closesDate.SetDate(&closed)
			setClosesTime(closed.Format("15:04"))
		}
	}

	saveEvent := func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		if !s.Session.IsAdmin() {
			if eventType.Selected != "Personal" {
				eventStatus.SetText("Only Personal events can be managed by non-administrators")
				return
			}
			if editingEventID != 0 {
				if selectedEvent == nil || selectedEvent.ownerID != s.Session.User.ID || selectedEvent.eventType != "Personal" {
					eventStatus.SetText("This event does not belong to your account")
					return
				}
				if selectedEvent.voteCount > 0 || eventIsActive(*selectedEvent, time.Now()) {
					eventStatus.SetText("Only administrators can edit voted or currently open events")
					return
				}
			}
		}
		if strings.TrimSpace(title.Text) == "" || opensDate.Date == nil || closesDate.Date == nil || eventDate.Date == nil {
			eventStatus.SetText("Title, event date, opening date, and closing date are required")
			return
		}
		if eventClass.Selected != "Private" && eventClass.Selected != "Public" {
			eventStatus.SetText("Choose Private or Public event class")
			return
		}
		if eventType.Selected != "Vote" && eventType.Selected != "Internal" && eventType.Selected != "External" && eventType.Selected != "Personal" {
			eventStatus.SetText("Choose a valid event type")
			return
		}
		if eventType.Selected == "Personal" {
			count, _ := strconv.Atoi(inviteeCount.Selected)
			if count < 1 || len(selectedInvitees) != count {
				eventStatus.SetText("Choose an invitee count and select exactly that many Members")
				return
			}
		}
		if eventType.Selected == "Personal" {
			count, _ := strconv.Atoi(inviteeCount.Selected)
			if count <= 0 || len(selectedInvitees) != count {
				eventStatus.SetText("Choose an invitee count and select exactly that many Members")
				return
			}
		}
		if !clockInRange(getOpensTime(), "06:00", "22:00") {
			eventStatus.SetText("Opening time must be between 06:00 and 22:00")
			return
		}
		if !clockInRange(getClosesTime(), "06:00", "22:00") {
			eventStatus.SetText("Closing time must be between 06:00 and 22:00")
			return
		}
		opensAt := opensDate.Date.Format("2006-01-02") + " " + getOpensTime()
		closesAt := closesDate.Date.Format("2006-01-02") + " " + getClosesTime()
		opens, _ := time.ParseInLocation("2006-01-02 15:04", opensAt, time.Local)
		closes, _ := time.ParseInLocation("2006-01-02 15:04", closesAt, time.Local)
		deadline := time.Date(eventDate.Date.Year(), eventDate.Date.Month(), eventDate.Date.Day(), 23, 59, 0, 0, time.Local)
		if !closes.After(opens) {
			eventStatus.SetText("Closing time must be after opening time")
			return
		}
		if !opens.Before(deadline) || !closes.Before(deadline) {
			eventStatus.SetText("Opening and closing times must be before the event date at 23:59")
			return
		}
		var err error
		savedEventID := editingEventID
		if api.Enabled() {
			client, clientErr := api.Default()
			if clientErr != nil {
				eventStatus.SetText(clientErr.Error())
				return
			}
			saved, saveErr := client.SaveEvent(api.Event{
				ID: editingEventID, Title: strings.TrimSpace(title.Text), Description: strings.TrimSpace(description.Text),
				OwnerID: s.Session.User.ID, InviteeIDs: append([]int64(nil), selectedInvitees...), EventType: eventType.Selected, EventClass: eventClass.Selected, EventDate: eventDate.Date.Format("2006-01-02"), OpensAt: opensAt, ClosesAt: closesAt, IsActive: true,
			})
			err = saveErr
			savedEventID = saved.ID
		} else if editingEventID == 0 {
			var result sql.Result
			result, err = db.DB().Exec(`INSERT INTO voting_event (title, description, event_type, event_class, event_date, opens_at, closes_at, owner_user_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`, strings.TrimSpace(title.Text), strings.TrimSpace(description.Text), eventType.Selected, eventClass.Selected, eventDate.Date.Format("2006-01-02"), opensAt, closesAt, s.Session.User.ID)
			if err == nil {
				savedEventID, err = result.LastInsertId()
			}
		} else {
			_, err = db.DB().Exec(`UPDATE voting_event SET title = ?, description = ?, event_type = ?, event_class = ?, event_date = ?, opens_at = ?, closes_at = ? WHERE id = ?`, strings.TrimSpace(title.Text), strings.TrimSpace(description.Text), eventType.Selected, eventClass.Selected, eventDate.Date.Format("2006-01-02"), opensAt, closesAt, editingEventID)
		}
		if err != nil {
			eventStatus.SetText(err.Error())
			return
		}
		if !api.Enabled() {
			if _, err := db.DB().Exec(`DELETE FROM voting_event_invitee WHERE event_id = ?`, savedEventID); err != nil {
				eventStatus.SetText(err.Error())
				return
			}
			for _, inviteeID := range selectedInvitees {
				if _, err := db.DB().Exec(`INSERT INTO voting_event_invitee (event_id, user_id) VALUES (?, ?)`, savedEventID, inviteeID); err != nil {
					eventStatus.SetText(err.Error())
					return
				}
			}
		}
		selectedEventID = savedEventID
		editingEventID = 0
		clear()
		eventStatus.SetText("Event saved")
		eventEditor.Hide()
		loadEvents()
	}

	addButton := widget.NewButton("Add", func() {
		if err := s.Session.Validate(); err != nil {
			s.ShowLogin()
			return
		}
		editingEventID = 0
		clear()
		eventStatus.SetText("")
		eventEditorTitle.SetText("Add event")
		saveButton.SetText("Save event")
		prompt.Hide()
		votePanel.Hide()
		eventInfo.Hide()
		saveButton.Enable()
		eventEditor.Show()
		rightPanel.Refresh()
	})
	editButton = widget.NewButton("Edit", func() {
		if selectedEvent == nil {
			return
		}
		editingEventID = selectedEvent.id
		fillEditor(*selectedEvent)
		eventStatus.SetText("")
		eventEditorTitle.SetText("Edit event")
		saveButton.SetText("Save changes")
		prompt.Hide()
		votePanel.Hide()
		eventInfo.Hide()
		saveButton.Enable()
		saveButton.Show()
		eventEditor.Show()
		rightPanel.Refresh()
	})
	editButton.Disable()
	deleteButton = widget.NewButton("Delete", func() {
		if selectedEvent == nil {
			return
		}
		if !admin && (selectedEvent.eventType != "Personal" || selectedEvent.ownerID != s.Session.User.ID || !selectedEvent.isActive || selectedEvent.voteCount > 0 || eventIsActive(*selectedEvent, time.Now())) {
			eventFilterStatus.SetText("Only your active, unopened Personal events without votes can be deleted")
			return
		}
		deletingID := selectedEvent.id
		dialog.ShowConfirm("Delete event", "Delete this event and its votes?", func(confirmed bool) {
			if !confirmed {
				return
			}
			var err error
			if api.Enabled() {
				client, clientErr := api.Default()
				if clientErr != nil {
					eventFilterStatus.SetText(clientErr.Error())
					return
				}
				err = client.DeleteEvent(deletingID)
			} else {
				_, err = db.DB().Exec(`DELETE FROM voting_event WHERE id = ?`, deletingID)
			}
			if err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
			showPrompt()
			eventFilterStatus.SetText("Event deleted")
			loadEvents()
		}, s.Window)
	})
	deleteButton.Disable()
	deactivateButton = widget.NewButton("Deactivate", func() {
		if selectedEvent == nil {
			return
		}
		if !admin && (selectedEvent.eventType != "Personal" || selectedEvent.ownerID != s.Session.User.ID || !selectedEvent.isActive) {
			eventFilterStatus.SetText("Only your active Personal events can be deactivated")
			return
		}
		deactivatingID := selectedEvent.id
		dialog.ShowConfirm("Deactivate event", "Deactivate this event? Voting will stop immediately.", func(confirmed bool) {
			if !confirmed {
				return
			}
			var err error
			if api.Enabled() {
				client, clientErr := api.Default()
				if clientErr != nil {
					eventFilterStatus.SetText(clientErr.Error())
					return
				}
				err = client.DeactivateEvent(deactivatingID)
			} else {
				_, err = db.DB().Exec(`UPDATE voting_event SET is_active = 0 WHERE id = ?`, deactivatingID)
			}
			if err != nil {
				eventFilterStatus.SetText(err.Error())
				return
			}
			eventFilterStatus.SetText("Event deactivated")
			loadEvents()
		}, s.Window)
	})
	deactivateButton.Disable()

	cancelButton := widget.NewButton("Cancel", func() {
		editingEventID = 0
		clear()
		saveButton.Hide()
		eventStatus.SetText("")
		if selectedEvent != nil {
			showSelectedEvent(*selectedEvent)
		} else {
			showPrompt()
		}
	})
	saveButton = widget.NewButton("Save event", saveEvent)
	saveButton.Hide()
	eventEditorTitle = widget.NewLabelWithStyle("Add event", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	eventTypeCaption := widget.NewLabelWithStyle("Event type", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	eventType.OnChanged = func(value string) {
		if value == "Personal" {
			eventTypeCaption.SetText("Personal-Event")
		} else {
			eventTypeCaption.SetText("Event type")
		}
		if inviteeFields != nil {
			if value == "Personal" {
				inviteeFields.Show()
			} else {
				inviteeFields.Hide()
			}
			inviteeFields.Refresh()
		}
	}
	title.OnChanged = func(value string) {
		if strings.TrimSpace(value) != "" || editingEventID != 0 {
			saveButton.Show()
		} else {
			saveButton.Hide()
		}
		eventEditor.Refresh()
	}

	form := widget.NewForm(
		widget.NewFormItem("Title", title),
		widget.NewFormItem("Description", description),
		widget.NewFormItem("Event date", eventDate),
		widget.NewFormItem("Opens", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), opensDate),
			container.NewVBox(widget.NewLabel("Time"), opensTime),
		)),
		widget.NewFormItem("Closes", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), closesDate),
			container.NewVBox(widget.NewLabel("Time"), closesTime),
		)),
	)
	inviteeFields = container.NewVBox(
		widget.NewLabelWithStyle("Invitees who can vote", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Number of invitees", inviteeCount),
			widget.NewFormItem("Add Member", inviteeSelect),
		),
		selectedInviteeList,
		inviteeStatus,
	)
	if eventType.Selected != "Personal" {
		inviteeFields.Hide()
	}
	eventFields := container.NewVBox(
		container.NewVBox(eventTypeCaption, eventType),
		container.NewVBox(widget.NewLabelWithStyle("Event class", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), eventClass),
		inviteeFields,
		form,
	)
	eventEditor = container.NewBorder(
		eventEditorTitle,
		container.NewVBox(container.NewHBox(cancelButton, saveButton), eventStatus),
		nil,
		nil,
		eventFields,
	)
	eventEditor.Hide()
	rightPanel = container.NewStack(prompt, votePanel, eventInfo, eventEditor)

	if admin {
		addButton.Importance = widget.HighImportance
	}
	eventActions := container.NewHBox(editButton, addButton, deleteButton, deactivateButton)
	eventPane := container.NewBorder(
		widget.NewLabelWithStyle("Events", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(eventActions, eventFilterStatus),
		nil,
		nil,
		eventList,
	)
	split := container.NewHSplit(eventPane, rightPanel)
	split.Offset = 0.38
	loadEvents()
	return split
}

func clockInRange(value, minimum, maximum string) bool {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return false
	}
	min, minErr := time.Parse("15:04", minimum)
	max, maxErr := time.Parse("15:04", maximum)
	return minErr == nil && maxErr == nil && !parsed.Before(min) && !parsed.After(max)
}

func newTimeSpinner(initial string) (fyne.CanvasObject, func() string, func(string)) {
	const minimum = 6 * 60
	const maximum = 22 * 60
	const step = 15
	minutes := minimum
	if parsed, err := time.Parse("15:04", initial); err == nil {
		minutes = parsed.Hour()*60 + parsed.Minute()
	}
	if minutes < minimum {
		minutes = minimum
	}
	if minutes > maximum {
		minutes = maximum
	}
	value := widget.NewLabel("")
	var decrement, increment *widget.Button
	refresh := func() {
		value.SetText(fmt.Sprintf("%02d:%02d", minutes/60, minutes%60))
		if minutes <= minimum {
			decrement.Disable()
		} else {
			decrement.Enable()
		}
		if minutes >= maximum {
			increment.Disable()
		} else {
			increment.Enable()
		}
	}
	decrement = widget.NewButton("-", func() {
		minutes -= step
		if minutes < minimum {
			minutes = minimum
		}
		refresh()
	})
	increment = widget.NewButton("+", func() {
		minutes += step
		if minutes > maximum {
			minutes = maximum
		}
		refresh()
	})
	refresh()
	set := func(text string) {
		parsed, err := time.Parse("15:04", strings.TrimSpace(text))
		if err != nil {
			return
		}
		minutes = parsed.Hour()*60 + parsed.Minute()
		if minutes < minimum {
			minutes = minimum
		}
		if minutes > maximum {
			minutes = maximum
		}
		refresh()
	}
	get := func() string { return value.Text }
	return container.NewHBox(decrement, value, increment), get, set
}

func eventIsActive(event votingEventRow, now time.Time) bool {
	opens, opensErr := time.ParseInLocation("2006-01-02 15:04", event.opensAt, time.Local)
	closes, closesErr := time.ParseInLocation("2006-01-02 15:04", event.closesAt, time.Local)
	return opensErr == nil && closesErr == nil && !now.Before(opens) && now.Before(closes)
}

func eventStatusText(event votingEventRow) string {
	if !event.isActive {
		return "Deactivated"
	}
	if eventIsActive(event, time.Now()) {
		return "Active"
	}
	return "Scheduled"
}

func visibleVotingEvents(events []votingEventRow, userID int64, admin bool) []votingEventRow {
	visible := make([]votingEventRow, 0, len(events))
	for _, event := range events {
		if admin && event.eventType == "Personal" && event.ownerID != userID {
			continue
		}
		if event.eventType == "Personal" && event.ownerID != userID && !containsInvitee(event.inviteeIDs, userID) {
			continue
		}
		if !event.isActive && event.ownerID != userID {
			continue
		}
		if event.eventType != "Personal" && event.eventClass == "Private" && event.ownerID != userID {
			continue
		}
		visible = append(visible, event)
	}
	return visible
}

func containsInvitee(inviteeIDs []int64, userID int64) bool {
	for _, inviteeID := range inviteeIDs {
		if inviteeID == userID {
			return true
		}
	}
	return false
}

func eventInviteeIDs(eventID int64) ([]int64, error) {
	rows, err := db.DB().Query(`SELECT user_id FROM voting_event_invitee WHERE event_id = ? ORDER BY user_id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func eventTypeText(eventType string) string {
	if eventType == "Personal" {
		return "Personal-Event"
	}
	return eventType
}
