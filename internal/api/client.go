package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parent_id,omitempty"`
	Name     string `json:"name"`
	DOB      string `json:"dob"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type PasswordPolicy struct {
	MinimumLength    int  `json:"minimum_length"`
	RequireSpecial   bool `json:"require_special"`
	RequireMixedCase bool `json:"require_mixed_case"`
}

type PasswordResetRequest struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	UserName       string `json:"user_name"`
	UserEmail      string `json:"user_email"`
	TimeframeStart string `json:"timeframe_start"`
	TimeframeEnd   string `json:"timeframe_end"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

type Project struct {
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"owner_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Task struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	DueDate  string `json:"due_date"`
	Done     bool   `json:"done"`
}

type Event struct {
	ID          int64   `json:"id"`
	OwnerID     int64   `json:"owner_id"`
	InviteeIDs  []int64 `json:"invitee_ids,omitempty"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	EventType   string  `json:"event_type"`
	EventClass  string  `json:"event_class"`
	EventDate   string  `json:"event_date"`
	OpensAt     string  `json:"opens_at"`
	ClosesAt    string  `json:"closes_at"`
	IsActive    bool    `json:"is_active"`
	VoteCount   int     `json:"vote_count"`
}

type Artifact struct {
	ID          int64  `json:"id"`
	EventID     int64  `json:"event_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FileName    string `json:"file_name"`
	FileURL     string `json:"file_url"`
	CreatedAt   string `json:"created_at"`
}

type Vote struct {
	Choice   string `json:"choice"`
	Comments string `json:"comments"`
}

type VoteSummary struct {
	Yes     int `json:"yes"`
	No      int `json:"no"`
	Abstain int `json:"abstain"`
}

type Era struct {
	ID        int64           `json:"id"`
	Slug      string          `json:"slug"`
	Title     string          `json:"title"`
	SortOrder int64           `json:"sort_order"`
	Theme     json.RawMessage `json:"theme"`
}

type Album struct {
	ID        int64  `json:"id"`
	EraID     int64  `json:"era_id"`
	Title     string `json:"title"`
	SortOrder int64  `json:"sort_order"`
}

type Media struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Duration int64  `json:"duration_s,omitempty"`
	Caption  string `json:"caption"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
}

type LayoutFrame struct {
	Region  string          `json:"region"`
	Feature string          `json:"feature"`
	Visible bool            `json:"visible"`
	Config  json.RawMessage `json:"config"`
}

type Client struct {
	baseURL string
	client  *http.Client
	mu      sync.RWMutex
	token   string
}

var (
	defaultMu     sync.RWMutex
	defaultClient *Client
)

func Configure(baseURL string) error {
	client, err := NewClient(baseURL)
	if err != nil {
		return err
	}
	defaultMu.Lock()
	defaultClient = client
	defaultMu.Unlock()
	return nil
}

func NewClient(baseURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid API URL %q", baseURL)
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func ConfigureFromEnvironment() error {
	baseURL := strings.TrimSpace(os.Getenv("MYTODO_API_URL"))
	if baseURL == "" {
		return nil
	}
	return Configure(baseURL)
}

func Enabled() bool {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultClient != nil
}

func CurrentURL() string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultClient == nil {
		return ""
	}
	return defaultClient.baseURL
}

func Disable() {
	defaultMu.Lock()
	defaultClient = nil
	defaultMu.Unlock()
}

func Default() (*Client, error) {
	defaultMu.RLock()
	client := defaultClient
	defaultMu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("server API is not configured")
	}
	return client, nil
}

func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

func (c *Client) request(method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		if result.Error == "" {
			result.Error = response.Status
		}
		return fmt.Errorf("%s", result.Error)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func (c *Client) NeedsSetup() (bool, error) {
	var result struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	err := c.request(http.MethodGet, "/status", nil, &result)
	return result.NeedsSetup, err
}

func (c *Client) Login(email, password string) (User, string, error) {
	var result struct {
		User  User   `json:"user"`
		Token string `json:"token"`
	}
	err := c.request(http.MethodPost, "/auth/login", map[string]string{"email": email, "password": password}, &result)
	if err == nil {
		c.SetToken(result.Token)
	}
	return result.User, result.Token, err
}

func (c *Client) Logout() error {
	err := c.request(http.MethodPost, "/auth/logout", struct{}{}, nil)
	c.SetToken("")
	return err
}

func (c *Client) Me() (User, error) {
	var user User
	err := c.request(http.MethodGet, "/auth/me", nil, &user)
	return user, err
}

func (c *Client) ListUsers() ([]User, error) {
	var users []User
	err := c.request(http.MethodGet, "/users", nil, &users)
	return users, err
}

func (c *Client) User(id int64) (User, error) {
	var user User
	err := c.request(http.MethodGet, fmt.Sprintf("/users/%d", id), nil, &user)
	return user, err
}

func (c *Client) SaveUser(user User, password string) (User, error) {
	var saved User
	path, method := "/users", http.MethodPost
	if user.ID != 0 {
		path = fmt.Sprintf("/users/%d", user.ID)
		method = http.MethodPut
	} else {
		needsSetup, err := c.NeedsSetup()
		if err != nil {
			return User{}, err
		}
		if needsSetup {
			path = "/auth/bootstrap"
		}
	}
	payload := struct {
		User     User   `json:"user"`
		Password string `json:"password"`
	}{user, password}
	err := c.request(method, path, payload, &saved)
	return saved, err
}

func (c *Client) DeleteUser(id int64) error {
	return c.request(http.MethodDelete, fmt.Sprintf("/users/%d", id), nil, nil)
}

func (c *Client) ChangePassword(currentPassword, newPassword string) error {
	return c.request(http.MethodPost, "/auth/password/change", map[string]string{
		"current_password": currentPassword,
		"new_password":     newPassword,
	}, nil)
}

func (c *Client) RequestPasswordReset(start, end, reason string) error {
	return c.request(http.MethodPost, "/auth/password/reset-requests", map[string]string{
		"timeframe_start": start,
		"timeframe_end":   end,
		"reason":          reason,
	}, nil)
}

func (c *Client) PasswordPolicy() (PasswordPolicy, error) {
	var policy PasswordPolicy
	err := c.request(http.MethodGet, "/admin/password-policy", nil, &policy)
	return policy, err
}

func (c *Client) SavePasswordPolicy(policy PasswordPolicy) error {
	return c.request(http.MethodPut, "/admin/password-policy", policy, nil)
}

func (c *Client) PasswordResetRequests() ([]PasswordResetRequest, error) {
	var requests []PasswordResetRequest
	err := c.request(http.MethodGet, "/admin/password-reset-requests", nil, &requests)
	return requests, err
}

func (c *Client) ResetUserPassword(requestID int64, password string) error {
	return c.request(http.MethodPost, fmt.Sprintf("/admin/password-reset-requests/%d/reset", requestID), map[string]string{
		"default_password": password,
	}, nil)
}

func (c *Client) LayoutFrames() ([]LayoutFrame, error) {
	var frames []LayoutFrame
	err := c.request(http.MethodGet, "/layout", nil, &frames)
	return frames, err
}

func (c *Client) SaveLayoutFrame(frame LayoutFrame) error {
	var input struct {
		Feature string          `json:"feature"`
		Visible bool            `json:"visible"`
		Config  json.RawMessage `json:"config"`
	}
	input.Feature, input.Visible = frame.Feature, frame.Visible
	input.Config = frame.Config
	return c.request(http.MethodPut, "/admin/layout/"+url.PathEscape(frame.Region), input, nil)
}

func (c *Client) Eras() ([]Era, error) {
	var eras []Era
	err := c.request(http.MethodGet, "/eras", nil, &eras)
	return eras, err
}

func (c *Client) Albums(eraSlug string) ([]Album, error) {
	var albums []Album
	err := c.request(http.MethodGet, "/eras/"+url.PathEscape(eraSlug)+"/albums", nil, &albums)
	return albums, err
}

func (c *Client) CreateEra(era Era) (Era, error) {
	var created Era
	err := c.request(http.MethodPost, "/admin/eras", era, &created)
	return created, err
}

func (c *Client) CreateAlbum(eraID int64, album Album) (Album, error) {
	var created Album
	err := c.request(http.MethodPost, fmt.Sprintf("/admin/eras/%d/albums", eraID), album, &created)
	return created, err
}

func (c *Client) UploadMedia(albumID int64, kind, filename, caption string, content io.Reader) (Media, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kind", kind); err != nil {
		return Media{}, err
	}
	if err := writer.WriteField("caption", caption); err != nil {
		return Media{}, err
	}
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return Media{}, err
	}
	if _, err := io.Copy(part, content); err != nil {
		return Media{}, err
	}
	if err := writer.Close(); err != nil {
		return Media{}, err
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/admin/albums/%d/media", c.baseURL, albumID), &body)
	if err != nil {
		return Media{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return Media{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		if result.Error == "" {
			result.Error = response.Status
		}
		return Media{}, fmt.Errorf("%s", result.Error)
	}
	var media Media
	err = json.NewDecoder(response.Body).Decode(&media)
	return media, err
}

func (c *Client) Projects() ([]Project, error) {
	var values []Project
	err := c.request(http.MethodGet, "/projects", nil, &values)
	return values, err
}

func (c *Client) Project(id int64) (Project, error) {
	var project Project
	err := c.request(http.MethodGet, fmt.Sprintf("/projects/%d", id), nil, &project)
	return project, err
}

func (c *Client) SaveProject(project Project) (Project, error) {
	var saved Project
	path, method := "/projects", http.MethodPost
	if project.ID != 0 {
		path = fmt.Sprintf("/projects/%d", project.ID)
		method = http.MethodPut
	}
	err := c.request(method, path, project, &saved)
	return saved, err
}

func (c *Client) DeleteProject(id int64) error {
	return c.request(http.MethodDelete, fmt.Sprintf("/projects/%d", id), nil, nil)
}

func (c *Client) Tasks(projectID int64) ([]Task, error) {
	var values []Task
	err := c.request(http.MethodGet, fmt.Sprintf("/projects/%d/tasks", projectID), nil, &values)
	return values, err
}

func (c *Client) Task(id int64) (Task, error) {
	var task Task
	err := c.request(http.MethodGet, fmt.Sprintf("/tasks/%d", id), nil, &task)
	return task, err
}

func (c *Client) SaveTask(task Task) (Task, error) {
	var saved Task
	path, method := fmt.Sprintf("/projects/%d/tasks", task.ParentID), http.MethodPost
	if task.ID != 0 {
		path = fmt.Sprintf("/tasks/%d", task.ID)
		method = http.MethodPut
	}
	err := c.request(method, path, task, &saved)
	return saved, err
}

func (c *Client) DeleteTask(id int64) error {
	return c.request(http.MethodDelete, fmt.Sprintf("/tasks/%d", id), nil, nil)
}

func (c *Client) Events() ([]Event, error) {
	var values []Event
	err := c.request(http.MethodGet, "/events", nil, &values)
	return values, err
}

func (c *Client) EventInvitees() ([]User, error) {
	var values []User
	err := c.request(http.MethodGet, "/event-invitees", nil, &values)
	return values, err
}

func (c *Client) SaveEvent(event Event) (Event, error) {
	var saved Event
	path, method := "/events", http.MethodPost
	if event.ID != 0 {
		path = fmt.Sprintf("/events/%d", event.ID)
		method = http.MethodPut
	}
	err := c.request(method, path, event, &saved)
	return saved, err
}

func (c *Client) Artifacts(eventID int64) ([]Artifact, error) {
	path := "/artifacts"
	if eventID > 0 {
		path += fmt.Sprintf("?event_id=%d", eventID)
	}
	var values []Artifact
	err := c.request(http.MethodGet, path, nil, &values)
	return values, err
}

func (c *Client) Artifact(id int64) (Artifact, error) {
	var artifact Artifact
	err := c.request(http.MethodGet, fmt.Sprintf("/artifacts/%d", id), nil, &artifact)
	return artifact, err
}

func (c *Client) UploadArtifact(artifact Artifact, filename string, content io.Reader) (Artifact, error) {
	return c.saveArtifact(artifact, filename, content)
}

func (c *Client) UpdateArtifact(artifact Artifact, filename string, content io.Reader) (Artifact, error) {
	return c.saveArtifact(artifact, filename, content)
}

func (c *Client) saveArtifact(artifact Artifact, filename string, content io.Reader) (Artifact, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"event_id":    strconv.FormatInt(artifact.EventID, 10),
		"title":       artifact.Title,
		"description": artifact.Description,
	} {
		if err := writer.WriteField(key, value); err != nil {
			return Artifact{}, err
		}
	}
	if content != nil {
		part, err := writer.CreateFormFile("file", filepath.Base(filename))
		if err != nil {
			return Artifact{}, err
		}
		if _, err := io.Copy(part, content); err != nil {
			return Artifact{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return Artifact{}, err
	}
	path, method := "/artifacts", http.MethodPost
	if artifact.ID != 0 {
		path, method = fmt.Sprintf("/artifacts/%d", artifact.ID), http.MethodPut
	}
	request, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, &body)
	if err != nil {
		return Artifact{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return Artifact{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Artifact{}, decodeAPIError(response)
	}
	var saved Artifact
	err = json.NewDecoder(response.Body).Decode(&saved)
	return saved, err
}

func (c *Client) DeleteArtifact(id int64) error {
	return c.request(http.MethodDelete, fmt.Sprintf("/artifacts/%d", id), nil, nil)
}

func (c *Client) DownloadArtifact(id int64, destination io.Writer) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("%s/artifacts/%d/file", c.baseURL, id), nil)
	if err != nil {
		return err
	}
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeAPIError(response)
	}
	_, err = io.Copy(destination, response.Body)
	return err
}

func decodeAPIError(response *http.Response) error {
	var result struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	if result.Error == "" {
		result.Error = response.Status
	}
	return fmt.Errorf("%s", result.Error)
}

func (c *Client) DeleteEvent(id int64) error {
	return c.request(http.MethodDelete, fmt.Sprintf("/events/%d", id), nil, nil)
}

func (c *Client) DeactivateEvent(id int64) error {
	return c.request(http.MethodPost, fmt.Sprintf("/events/%d/deactivate", id), nil, nil)
}

func (c *Client) EventVoteSummary(id int64) (VoteSummary, error) {
	var summary VoteSummary
	err := c.request(http.MethodGet, fmt.Sprintf("/events/%d/summary", id), nil, &summary)
	return summary, err
}

func (c *Client) GetVote(eventID int64) (Vote, error) {
	var vote Vote
	err := c.request(http.MethodGet, fmt.Sprintf("/events/%d/vote", eventID), nil, &vote)
	return vote, err
}

func (c *Client) SaveVote(eventID int64, vote Vote) error {
	return c.request(http.MethodPut, fmt.Sprintf("/events/%d/vote", eventID), vote, nil)
}

func (c *Client) ImportDatabase(filename string, content []byte) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("database", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/admin/import", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		if result.Error == "" {
			result.Error = response.Status
		}
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (c *Client) BackupDatabase(destination io.Writer) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/admin/backup", nil)
	if err != nil {
		return err
	}
	if token := c.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		if result.Error == "" {
			result.Error = response.Status
		}
		return fmt.Errorf("%s", result.Error)
	}
	_, err = io.Copy(destination, response.Body)
	return err
}

type environment interface{ Get(string) string }
