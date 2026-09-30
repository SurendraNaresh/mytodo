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
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"owner_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	EventType   string `json:"event_type"`
	EventClass  string `json:"event_class"`
	EventDate   string `json:"event_date"`
	OpensAt     string `json:"opens_at"`
	ClosesAt    string `json:"closes_at"`
	IsActive    bool   `json:"is_active"`
	VoteCount   int    `json:"vote_count"`
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
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid API URL %q", baseURL)
	}
	defaultMu.Lock()
	defaultClient = &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	defaultMu.Unlock()
	return nil
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
