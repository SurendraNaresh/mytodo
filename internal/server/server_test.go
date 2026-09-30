package server

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
)

func TestRemoteAdminBootstrapAndAuthorization(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		api.Disable()
		_ = db.Close()
	})

	server := httptest.NewServer(New())
	defer server.Close()
	if err := api.Configure(server.URL + "/api/v1"); err != nil {
		t.Fatal(err)
	}
	client, err := api.Default()
	if err != nil {
		t.Fatal(err)
	}
	needsSetup, err := client.NeedsSetup()
	if err != nil || !needsSetup {
		t.Fatalf("NeedsSetup() = %t, %v; want true, nil", needsSetup, err)
	}
	admin, err := client.SaveUser(api.User{
		Name:  "Administrator",
		Email: "admin@example.test",
		Role:  "Admin",
	}, "secure-test-password")
	if err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	if admin.ID == 0 {
		t.Fatal("bootstrap returned an empty user ID")
	}
	if _, err := client.SaveUser(api.User{Name: "Second admin", Email: "second@example.test", Role: "Admin"}, "another-test-password"); err == nil {
		t.Fatal("unauthenticated second account creation succeeded")
	}
	if _, _, err := client.Login(admin.Email, "secure-test-password"); err != nil {
		t.Fatalf("admin login: %v", err)
	}
	var backup bytes.Buffer
	if err := client.BackupDatabase(&backup); err != nil {
		t.Fatalf("download database backup: %v", err)
	}
	if !bytes.HasPrefix(backup.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatal("downloaded backup is not a SQLite database")
	}
	users, err := client.ListUsers()
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers() returned %d users, %v; want one user", len(users), err)
	}
	if err := client.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListUsers(); err == nil {
		t.Fatal("unauthenticated user listing succeeded")
	}
}

func TestProtectedAPIRequiresBearerToken(t *testing.T) {
	for _, path := range []string{"/api/v1/events", "/api/v1/admin/backup"} {
		request := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		New().ServeHTTP(response, request)
		if response.Code != 401 {
			t.Fatalf("GET %s returned %d, want 401", path, response.Code)
		}
	}
}

func TestValidateEventDateAndTimes(t *testing.T) {
	valid := api.Event{
		Title: "Event", EventType: "Vote", EventClass: "Public", EventDate: "2099-01-02",
		OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00",
	}
	if err := validateEvent(valid); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		update func(*api.Event)
	}{
		{name: "close before open", update: func(event *api.Event) { event.ClosesAt = "2099-01-01 08:59" }},
		{name: "close at event cutoff", update: func(event *api.Event) { event.ClosesAt = "2099-01-02 23:59" }},
		{name: "open at event cutoff", update: func(event *api.Event) { event.OpensAt = "2099-01-02 23:59" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.update(&event)
			if err := validateEvent(event); err == nil {
				t.Fatal("invalid event was accepted")
			}
		})
	}
}

func TestPersonalEventPermissionsAndVoteSummary(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		api.Disable()
		_ = db.Close()
	})

	server := httptest.NewServer(New())
	defer server.Close()
	if err := api.Configure(server.URL + "/api/v1"); err != nil {
		t.Fatal(err)
	}
	client, err := api.Default()
	if err != nil {
		t.Fatal(err)
	}
	admin, err := client.SaveUser(api.User{Name: "Administrator", Email: "admin-events@example.test", Role: "Admin"}, "secure-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Login(admin.Email, "secure-test-password"); err != nil {
		t.Fatal(err)
	}
	member, err := client.SaveUser(api.User{Name: "Member", Email: "member-events@example.test", Role: "Member"}, "member-test-password")
	if err != nil {
		t.Fatal(err)
	}
	otherMember, err := client.SaveUser(api.User{Name: "Other Member", Email: "other-events@example.test", Role: "Member"}, "other-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Login(member.Email, "member-test-password"); err != nil {
		t.Fatal(err)
	}
	newEvent := func(title string) api.Event {
		return api.Event{
			Title: title, EventType: "Personal", EventClass: "Private",
			EventDate: "2099-01-02",
			OpensAt:   "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00",
		}
	}
	if _, err := client.SaveEvent(api.Event{Title: "Internal", EventType: "Internal", EventDate: "2099-01-02", OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00"}); err == nil {
		t.Fatal("non-admin created a non-Personal event")
	}
	personal, err := client.SaveEvent(newEvent("My event"))
	if err != nil {
		t.Fatalf("create Personal event: %v", err)
	}
	if personal.OwnerID != member.ID || !personal.IsActive {
		t.Fatalf("created event owner/status = %d/%t; want %d/true", personal.OwnerID, personal.IsActive, member.ID)
	}
	events, err := client.Events()
	if err != nil || len(events) != 1 || events[0].OwnerID != member.ID {
		t.Fatalf("member event list = %#v, %v; want only own Personal event", events, err)
	}
	if events[0].OwnerID == otherMember.ID {
		t.Fatal("member received another user's Personal event")
	}
	if _, _, err := client.Login(admin.Email, "secure-test-password"); err != nil {
		t.Fatal(err)
	}
	publicEvent, err := client.SaveEvent(api.Event{
		Title: "Public event", EventType: "Vote", EventClass: "Public",
		EventDate: "2099-01-02",
		OpensAt:   "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00",
	})
	if err != nil {
		t.Fatalf("create public event: %v", err)
	}
	deactivatedPublicEvent, err := client.SaveEvent(api.Event{
		Title: "Deactivated public event", EventType: "Vote", EventClass: "Public", EventDate: "2099-01-02",
		OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeactivateEvent(deactivatedPublicEvent.ID); err != nil {
		t.Fatal(err)
	}
	adminEvents, err := client.Events()
	if err != nil || len(adminEvents) != 2 {
		t.Fatalf("admin event list = %#v, %v; want its public events including its deactivated event", adminEvents, err)
	}
	if _, _, err := client.Login(member.Email, "member-test-password"); err != nil {
		t.Fatal(err)
	}
	memberEvents, err := client.Events()
	if err != nil || len(memberEvents) != 2 {
		t.Fatalf("member event list = %#v, %v; want own private and public events", memberEvents, err)
	}
	for _, event := range memberEvents {
		if event.ID == deactivatedPublicEvent.ID {
			t.Fatal("member received another user's deactivated public event")
		}
	}
	if _, _, err := client.Login(otherMember.Email, "other-test-password"); err != nil {
		t.Fatal(err)
	}
	otherEvents, err := client.Events()
	if err != nil || len(otherEvents) != 1 || otherEvents[0].ID != publicEvent.ID {
		t.Fatalf("other member event list = %#v, %v; want public event only", otherEvents, err)
	}
	if _, _, err := client.Login(member.Email, "member-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SaveEvent(api.Event{ID: personal.ID, Title: "Updated", EventType: "Personal", EventDate: "2099-01-02", OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00"}); err != nil {
		t.Fatalf("update own future Personal event: %v", err)
	}
	if _, err := db.DB().Exec(`INSERT INTO vote (voting_event_id, voter_user_id, choice) VALUES (?, ?, 'Yes')`, personal.ID, otherMember.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SaveEvent(api.Event{ID: personal.ID, Title: "Locked", EventType: "Personal", EventDate: "2099-01-02", OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00"}); err == nil {
		t.Fatal("non-admin edited a voted event")
	}
	if err := client.DeleteEvent(personal.ID); err == nil {
		t.Fatal("non-admin deleted a voted event")
	}
	if err := client.DeactivateEvent(personal.ID); err != nil {
		t.Fatalf("deactivate own event: %v", err)
	}
	if err := client.DeleteEvent(personal.ID); err == nil {
		t.Fatal("non-admin deleted a deactivated event")
	}
	deletable, err := client.SaveEvent(newEvent("Deletable"))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteEvent(deletable.ID); err != nil {
		t.Fatalf("delete own active Personal event without votes: %v", err)
	}

	if _, _, err := client.Login(admin.Email, "secure-test-password"); err != nil {
		t.Fatal(err)
	}
	votingEvent, err := client.SaveEvent(api.Event{
		Title: "Ballot", EventType: "Vote", EventDate: "2099-01-02", OpensAt: "2099-01-01 09:00", ClosesAt: "2099-01-01 10:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	for voterID, choice := range map[int64]string{member.ID: "Yes", otherMember.ID: "No"} {
		if _, err := db.DB().Exec(`INSERT INTO vote (voting_event_id, voter_user_id, choice) VALUES (?, ?, ?)`, votingEvent.ID, voterID, choice); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := client.EventVoteSummary(votingEvent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Yes != 1 || summary.No != 1 || summary.Abstain != 0 {
		t.Fatalf("vote summary = %+v; want Yes=1, No=1, Abstain=0", summary)
	}
}
