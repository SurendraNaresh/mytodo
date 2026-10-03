package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
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
	if _, token, err := client.Login(admin.Email, "secure-test-password"); err != nil {
		t.Fatalf("admin login: %v", err)
	} else if strings.Count(token, ".") != 2 {
		t.Fatalf("login token is not a signed JWT: %q", token)
	} else {
		signatureStart := strings.LastIndex(token, ".") + 1
		tamperedFirst := "A"
		if token[signatureStart] == 'A' {
			tamperedFirst = "B"
		}
		tampered := token[:signatureStart] + tamperedFirst + token[signatureStart+1:]
		client.SetToken(tampered)
		if _, err := client.ListUsers(); err == nil {
			t.Fatal("tampered JWT signature was accepted")
		}
		client.SetToken(token)
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

func testSessionToken(t *testing.T, s *Server, userID int64, role model.Role) string {
	t.Helper()
	token, err := s.issueSession(&model.User{ID: userID, Role: role}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestProtectedAPIRequiresBearerToken(t *testing.T) {
	for _, path := range []string{"/api/v1/events", "/api/v1/admin/backup", "/api/v1/artifacts", "/api/v1/artifacts/1/file"} {
		request := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		New().ServeHTTP(response, request)
		if response.Code != 401 {
			t.Fatalf("GET %s returned %d, want 401", path, response.Code)
		}
	}
}

func TestArtifactCRUDIsAdminOnlyAndFilesAreEventLinked(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	model.UseRemoteAPI(false)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		model.UseRemoteAPI(false)
		api.Disable()
		_ = db.Close()
	})
	adminResult, err := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES ('Artifact admin', '', 'artifact-admin@example.test', 'Admin', 'unused')`)
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := adminResult.LastInsertId()
	memberResult, err := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES ('Artifact member', '', 'artifact-member@example.test', 'Member', 'unused')`)
	if err != nil {
		t.Fatal(err)
	}
	memberID, _ := memberResult.LastInsertId()
	eventResult, err := db.DB().Exec(`INSERT INTO voting_event (title, event_type, event_class, event_date, opens_at, closes_at) VALUES ('Artifact event', 'Internal', 'Public', '2099-01-01', '2099-01-01 09:00', '2099-01-01 10:00')`)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := eventResult.LastInsertId()

	s := New()
	adminToken := testSessionToken(t, s, adminID, model.RoleAdmin)
	memberToken := testSessionToken(t, s, memberID, model.RoleMember)
	makeUpload := func(token, method, path, fileName, fileContent string) *httptest.ResponseRecorder {
		t.Helper()
		body := new(bytes.Buffer)
		form := multipart.NewWriter(body)
		_ = form.WriteField("event_id", fmt.Sprint(eventID))
		_ = form.WriteField("title", "Event notes")
		_ = form.WriteField("description", "Approved minutes")
		if fileName != "" {
			part, err := form.CreateFormFile("file", fileName)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write([]byte(fileContent))
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, body)
		request.Header.Set("Content-Type", form.FormDataContentType())
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, request)
		return response
	}

	if response := makeUpload(memberToken, http.MethodPost, "/api/v1/artifacts", "minutes.pdf", "%PDF-1.7\nminutes"); response.Code != http.StatusForbidden {
		t.Fatalf("member artifact create = %d %s; want 403", response.Code, response.Body.String())
	}
	created := makeUpload(adminToken, http.MethodPost, "/api/v1/artifacts", "minutes.pdf", "%PDF-1.7\nminutes")
	if created.Code != http.StatusCreated {
		t.Fatalf("admin artifact create = %d %s", created.Code, created.Body.String())
	}
	var artifact api.Artifact
	if err := json.Unmarshal(created.Body.Bytes(), &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.ID == 0 || artifact.EventID != eventID || artifact.FileName != "minutes.pdf" {
		t.Fatalf("created artifact metadata = %#v", artifact)
	}
	filePath, err := db.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	var storedPath string
	if err := db.DB().QueryRow(`SELECT file_path FROM artifacts WHERE id = ?`, artifact.ID).Scan(&storedPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filePath, filepath.FromSlash(storedPath))); err != nil {
		t.Fatalf("stored artifact file missing: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/artifacts/%d", artifact.ID), nil)
	request.Header.Set("Authorization", "Bearer "+memberToken)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"event_id"`) {
		t.Fatalf("member artifact read = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/artifacts/%d/file", artifact.ID), nil)
	request.Header.Set("Authorization", "Bearer "+memberToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/pdf" || response.Body.String() != "%PDF-1.7\nminutes" {
		t.Fatalf("artifact download = %d %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}

	updated := makeUpload(memberToken, http.MethodPut, fmt.Sprintf("/api/v1/artifacts/%d", artifact.ID), "", "")
	if updated.Code != http.StatusForbidden {
		t.Fatalf("member artifact update = %d; want 403", updated.Code)
	}
	updated = makeUpload(adminToken, http.MethodPut, fmt.Sprintf("/api/v1/artifacts/%d", artifact.ID), "", "")
	if updated.Code != http.StatusOK {
		t.Fatalf("admin artifact update = %d %s", updated.Code, updated.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/artifacts/%d", artifact.ID), nil)
	request.Header.Set("Authorization", "Bearer "+memberToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member artifact delete = %d; want 403", response.Code)
	}
	invalid := makeUpload(adminToken, http.MethodPost, "/api/v1/artifacts", "minutes.pdf", "not a PDF")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("mismatched artifact file = %d; want 400", invalid.Code)
	}

	request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/artifacts/%d", artifact.ID), nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("admin artifact delete = %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(filePath, filepath.FromSlash(storedPath))); !os.IsNotExist(err) {
		t.Fatalf("artifact file remains after deletion: %v", err)
	}
}

func TestEraTimelineIsPublicAndCommentsRequireRegisteredRole(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	model.UseRemoteAPI(false)
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
	if _, err := db.DB().Exec(`UPDATE era SET theme_json = '{"accent":"#e4573d"}' WHERE slug = 'first-year'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO album (era_id, title, sort_order) VALUES (1, 'First Steps', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO media (album_id, kind, content_hash, sort_order) VALUES (2, 'photo', ?, 1)`, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	visitorResult, err := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES ('Visitor', '', 'visitor@example.test', 'Visitor', 'unused')`)
	if err != nil {
		t.Fatal(err)
	}
	visitorID, _ := visitorResult.LastInsertId()
	memberResult, err := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES ('Member', '', 'member@example.test', 'Member', 'unused')`)
	if err != nil {
		t.Fatal(err)
	}
	memberID, _ := memberResult.LastInsertId()

	s := New()
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/v1/eras", want: "first-year"},
		{path: "/api/v1/eras/first-year/albums", want: "First Steps"},
		{path: "/api/v1/albums/2/media", want: "/media/1"},
	} {
		response := httptest.NewRecorder()
		s.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("GET %s = %d %s, want 200 containing %q", test.path, response.Code, response.Body.String(), test.want)
		}
	}

	visitorToken := testSessionToken(t, s, visitorID, model.RoleVisitor)
	request := httptest.NewRequest("POST", "/api/v1/media/1/comments", strings.NewReader(`{"body":"A little comment"}`))
	request.Header.Set("Authorization", "Bearer "+visitorToken)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("visitor comment status = %d, want 403: %s", response.Code, response.Body.String())
	}

	memberToken := testSessionToken(t, s, memberID, model.RoleMember)
	request = httptest.NewRequest("POST", "/api/v1/media/1/comments", strings.NewReader(`{"body":"A little comment"}`))
	request.Header.Set("Authorization", "Bearer "+memberToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatalf("registered comment status = %d, want 201: %s", response.Code, response.Body.String())
	}
	var comment struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &comment); err != nil || comment.Body != "A little comment" {
		t.Fatalf("comment response = %s, %v", response.Body.String(), err)
	}
	request = httptest.NewRequest("GET", "/api/v1/media/1/comments", nil)
	request.Header.Set("Authorization", "Bearer "+memberToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "A little comment") {
		t.Fatalf("registered comment list = %d %s", response.Code, response.Body.String())
	}
}

func TestMagicLinkCreatesPasswordlessAccountAndIsSingleUse(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	t.Setenv("MYTODO_PUBLIC_URL", "https://archive.example.test")
	model.UseRemoteAPI(false)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		model.UseRemoteAPI(false)
		api.Disable()
		_ = db.Close()
	})
	s := New()
	var sentLink string
	s.sendMagicEmail = func(email, name, link string) error {
		if email != "member@example.test" || name != "Member" {
			t.Errorf("email delivery recipient/name = %q/%q", email, name)
		}
		sentLink = link
		return nil
	}
	request := httptest.NewRequest("POST", "/api/v1/auth/magic-link", strings.NewReader(`{"name":"Member","email":"Member@Example.Test"}`))
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 202 || !strings.Contains(response.Body.String(), "If this address") {
		t.Fatalf("magic-link request = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(sentLink, "https://archive.example.test/?token=") {
		t.Fatalf("magic link has unexpected origin/path: %q", sentLink)
	}
	tokenPosition := strings.Index(sentLink, "token=")
	magicToken := sentLink[tokenPosition+len("token="):]
	if len(magicToken) != 64 {
		t.Fatalf("magic token length = %d", len(magicToken))
	}
	var role, passwordHash string
	if err := db.DB().QueryRow(`SELECT role, password_hash FROM users WHERE email = 'member@example.test'`).Scan(&role, &passwordHash); err != nil {
		t.Fatal(err)
	}
	if role != "Member" || passwordHash != "" {
		t.Fatalf("passwordless account role/hash = %q/%q", role, passwordHash)
	}

	verify := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "/api/v1/auth/verify", strings.NewReader(`{"token":"`+magicToken+`"}`))
		response := httptest.NewRecorder()
		s.ServeHTTP(response, request)
		return response
	}
	verified := verify()
	if verified.Code != 200 || !strings.Contains(verified.Body.String(), `"role":"Member"`) {
		t.Fatalf("magic-link verification = %d %s", verified.Code, verified.Body.String())
	}
	if reused := verify(); reused.Code != 401 {
		t.Fatalf("reused magic link status = %d, want 401", reused.Code)
	}
}

func TestAdminLayoutAndMediaUpload(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", t.TempDir())
	model.UseRemoteAPI(false)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		model.UseRemoteAPI(false)
		api.Disable()
		_ = db.Close()
	})
	adminResult, err := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES ('Admin', '', 'upload-admin@example.test', 'Admin', 'unused')`)
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := adminResult.LastInsertId()
	s := New()
	adminToken := testSessionToken(t, s, adminID, model.RoleAdmin)

	request := httptest.NewRequest("PUT", "/api/v1/admin/layout/top", strings.NewReader(`{"feature":"era_title","visible":true,"config":{"size":"small"}}`))
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatalf("anonymous layout update status = %d, want 401", response.Code)
	}
	request = httptest.NewRequest("PUT", "/api/v1/admin/layout/top", strings.NewReader(`{"feature":"era_title","visible":true,"config":{"size":"small"}}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("admin layout update = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest("POST", "/api/v1/admin/eras", strings.NewReader(`{"slug":"upload-test","title":"Upload Test","sort_order":2,"theme":{"accent":"#df6049"}}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatalf("create era = %d %s", response.Code, response.Body.String())
	}
	var createdEra eraResponse
	if err := json.Unmarshal(response.Body.Bytes(), &createdEra); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/eras/%d/albums", createdEra.ID), strings.NewReader(`{"title":"First Steps","sort_order":1}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatalf("create album = %d %s", response.Code, response.Body.String())
	}
	var createdAlbum albumResponse
	if err := json.Unmarshal(response.Body.Bytes(), &createdAlbum); err != nil {
		t.Fatal(err)
	}

	imageData := new(bytes.Buffer)
	imageFixture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imageFixture.Set(0, 0, color.RGBA{R: 220, G: 80, B: 60, A: 255})
	if err := png.Encode(imageData, imageFixture); err != nil {
		t.Fatal(err)
	}
	upload := func(kind, filename string, content []byte) *httptest.ResponseRecorder {
		body := new(bytes.Buffer)
		form := multipart.NewWriter(body)
		_ = form.WriteField("kind", kind)
		part, err := form.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/albums/%d/media", createdAlbum.ID), body)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		request.Header.Set("Content-Type", form.FormDataContentType())
		response := httptest.NewRecorder()
		s.ServeHTTP(response, request)
		return response
	}
	photo := upload("photo", "first.png", imageData.Bytes())
	if photo.Code != 201 {
		t.Fatalf("image upload = %d %s", photo.Code, photo.Body.String())
	}
	var uploaded mediaResponse
	if err := json.Unmarshal(photo.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	duplicate := upload("photo", "copy.png", imageData.Bytes())
	if duplicate.Code != 200 || !strings.Contains(duplicate.Body.String(), fmt.Sprintf(`"id":%d`, uploaded.ID)) {
		t.Fatalf("duplicate upload = %d %s", duplicate.Code, duplicate.Body.String())
	}
	s.probeDuration = func(string) (float64, error) { return 61, nil }
	tooLong := upload("short", "long.mp4", []byte("video bytes"))
	if tooLong.Code != 422 || !strings.Contains(tooLong.Body.String(), "without trimming") {
		t.Fatalf("long short upload = %d %s; want explicit rejection without truncation", tooLong.Code, tooLong.Body.String())
	}
}

func TestDonationLinkAndQRRequireStripePaymentLink(t *testing.T) {
	s := New()
	t.Setenv("STRIPE_PAYMENT_LINK", "https://buy.stripe.com/test_payment_link")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/donate/link", nil))
	if response.Code != 307 || response.Header().Get("Location") != "https://buy.stripe.com/test_payment_link" {
		t.Fatalf("donation redirect = %d %q", response.Code, response.Header().Get("Location"))
	}
	response = httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/donate/qr", nil))
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/png" || !bytes.HasPrefix(response.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("donation QR = %d %q %d bytes", response.Code, response.Header().Get("Content-Type"), response.Body.Len())
	}
	t.Setenv("STRIPE_PAYMENT_LINK", "https://payments.example.test/not-stripe")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/donate/qr", nil))
	if response.Code != 503 {
		t.Fatalf("untrusted payment link status = %d, want 503", response.Code)
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
