package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
	"github.com/skip2/go-qrcode"
)

var mediaHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type eraResponse struct {
	ID        int64           `json:"id"`
	Slug      string          `json:"slug"`
	Title     string          `json:"title"`
	SortOrder int64           `json:"sort_order"`
	Theme     json.RawMessage `json:"theme"`
}

type albumResponse struct {
	ID        int64  `json:"id"`
	EraID     int64  `json:"era_id"`
	Title     string `json:"title"`
	SortOrder int64  `json:"sort_order"`
}

type mediaResponse struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Duration int64  `json:"duration_s,omitempty"`
	Caption  string `json:"caption"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
}

type layoutResponse struct {
	Region     string          `json:"region"`
	Feature    string          `json:"feature"`
	Visible    bool            `json:"visible"`
	ConfigJSON json.RawMessage `json:"config"`
}

func (s *Server) listEras(w http.ResponseWriter, _ *http.Request) {
	rows, err := db.DB().Query(`SELECT id, slug, title, sort_order, theme_json FROM era ORDER BY sort_order, id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	erases := make([]eraResponse, 0)
	for rows.Next() {
		var era eraResponse
		var theme string
		if err := rows.Scan(&era.ID, &era.Slug, &era.Title, &era.SortOrder, &theme); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		era.Theme = validJSONOrObject(theme)
		erases = append(erases, era)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, erases)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	if err := db.DB().Ping(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listEraAlbums(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB().Query(`SELECT album.id, album.era_id, album.title, album.sort_order
		FROM album JOIN era ON era.id = album.era_id WHERE era.slug = ? ORDER BY album.sort_order, album.id`, r.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	albums := make([]albumResponse, 0)
	for rows.Next() {
		var album albumResponse
		if err := rows.Scan(&album.ID, &album.EraID, &album.Title, &album.SortOrder); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Server) listAlbumMedia(w http.ResponseWriter, r *http.Request) {
	albumID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := db.DB().Query(`SELECT id, kind, COALESCE(duration_s, 0), COALESCE(caption, '')
		FROM media WHERE album_id = ? ORDER BY sort_order, id`, albumID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := make([]mediaResponse, 0)
	for rows.Next() {
		var item mediaResponse
		if err := rows.Scan(&item.ID, &item.Kind, &item.Duration, &item.Caption); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		item.URL = fmt.Sprintf("/media/%d", item.ID)
		item.ThumbURL = fmt.Sprintf("/thumbs/%d", item.ID)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) listLayoutFrames(w http.ResponseWriter, _ *http.Request) {
	rows, err := db.DB().Query(`SELECT region, feature, visible, config_json FROM layout_frame ORDER BY region`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	frames := make([]layoutResponse, 0, 5)
	for rows.Next() {
		var frame layoutResponse
		var config string
		if err := rows.Scan(&frame.Region, &frame.Feature, &frame.Visible, &config); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		frame.ConfigJSON = validJSONOrObject(config)
		frames = append(frames, frame)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, frames)
}

func (s *Server) donateLink(w http.ResponseWriter, r *http.Request) {
	link, err := stripePaymentLink()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "donations are not configured")
		return
	}
	http.Redirect(w, r, link, http.StatusTemporaryRedirect)
}

func (s *Server) donateStatus(w http.ResponseWriter, _ *http.Request) {
	_, err := stripePaymentLink()
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": err == nil})
}

func (s *Server) donateQR(w http.ResponseWriter, _ *http.Request) {
	link, err := stripePaymentLink()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "donations are not configured")
		return
	}
	image, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not render donation QR")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}

func stripePaymentLink() (string, error) {
	link := strings.TrimSpace(os.Getenv("STRIPE_PAYMENT_LINK"))
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "buy.stripe.com" && parsed.Hostname() != "checkout.stripe.com") {
		return "", fmt.Errorf("invalid Stripe payment link")
	}
	return parsed.String(), nil
}

func (s *Server) saveLayoutFrame(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	region := r.PathValue("region")
	if !validRegion(region) {
		writeError(w, http.StatusBadRequest, "invalid layout region")
		return
	}
	var input struct {
		Feature string          `json:"feature"`
		Visible bool            `json:"visible"`
		Config  json.RawMessage `json:"config"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validFeature(input.Feature) || !json.Valid(input.Config) {
		writeError(w, http.StatusBadRequest, "invalid feature or config")
		return
	}
	_, err := db.DB().Exec(`INSERT INTO layout_frame (region, feature, visible, config_json) VALUES (?, ?, ?, ?)
		ON CONFLICT(region) DO UPDATE SET feature = excluded.feature, visible = excluded.visible, config_json = excluded.config_json`, region, input.Feature, input.Visible, string(input.Config))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, layoutResponse{Region: region, Feature: input.Feature, Visible: input.Visible, ConfigJSON: input.Config})
}

func (s *Server) listMediaComments(w http.ResponseWriter, r *http.Request) {
	if !isRegistered(currentUser(r)) {
		writeError(w, http.StatusForbidden, "registered account required")
		return
	}
	mediaID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := db.DB().Query(`SELECT comment.id, comment.body, users.name, comment.created_at
		FROM comment JOIN users ON users.id = comment.user_id WHERE comment.media_id = ? ORDER BY comment.id`, mediaID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	comments := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var body, name, created string
		if err := rows.Scan(&id, &body, &name, &created); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		comments = append(comments, map[string]any{"id": id, "body": body, "name": name, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *Server) createMediaComment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !isRegistered(user) {
		writeError(w, http.StatusForbidden, "registered account required")
		return
	}
	mediaID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || len(input.Body) > 4000 {
		writeError(w, http.StatusBadRequest, "comment must be between 1 and 4000 characters")
		return
	}
	result, err := db.DB().Exec(`INSERT INTO comment (media_id, user_id, body, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, mediaID, user.ID, input.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "media not found")
		return
	}
	commentID, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": commentID, "body": input.Body, "name": user.Name, "created_at": time.Now().UTC().Format(time.RFC3339)})
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	mediaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || mediaID < 1 {
		http.NotFound(w, r)
		return
	}
	file, err := mediaFile(mediaID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, file)
}

func (s *Server) serveThumbnail(w http.ResponseWriter, r *http.Request) {
	mediaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || mediaID < 1 {
		http.NotFound(w, r)
		return
	}
	var hash string
	if err := db.DB().QueryRow(`SELECT content_hash FROM media WHERE id = ?`, mediaID).Scan(&hash); err != nil || !mediaHashPattern.MatchString(hash) {
		http.NotFound(w, r)
		return
	}
	file := filepath.Join(mediaRoot(), "thumbs", hash+".jpg")
	if _, err := os.Stat(file); err != nil {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, file)
}

func mediaFile(mediaID int64) (string, error) {
	var hash, eraSlug, albumTitle string
	var albumID int64
	err := db.DB().QueryRow(`SELECT media.content_hash, era.slug, album.title, album.id
		FROM media JOIN album ON album.id = media.album_id JOIN era ON era.id = album.era_id WHERE media.id = ?`, mediaID).
		Scan(&hash, &eraSlug, &albumTitle, &albumID)
	if err != nil || !mediaHashPattern.MatchString(hash) {
		return "", sql.ErrNoRows
	}
	pattern := filepath.Join(mediaRoot(), safePathSegment(eraSlug), albumPathSegment(albumTitle, albumID), hash+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", os.ErrNotExist
	}
	return matches[0], nil
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "application/octet-stream"
	if detected := http.DetectContentType(make([]byte, 0)); detected != "" {
		contentType = detected
	}
	if extensionType := mimeTypeByExtension(filepath.Ext(path)); extensionType != "" {
		contentType = extensionType
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func mediaRoot() string {
	dir, err := db.DataDir()
	if err != nil {
		return filepath.Join("data", "media")
	}
	return filepath.Join(dir, "media")
}

func safePathSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	separator := false
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	if result.Len() == 0 {
		return "item"
	}
	return result.String()
}

func albumPathSegment(title string, id int64) string {
	return fmt.Sprintf("%s-%d", safePathSegment(title), id)
}

func validJSONOrObject(value string) json.RawMessage {
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	return json.RawMessage(`{}`)
}

func validRegion(region string) bool {
	switch region {
	case "top", "bottom", "left", "right", "center":
		return true
	default:
		return false
	}
}

func validFeature(feature string) bool {
	switch feature {
	case "timeline", "era_title", "album_grid", "media_gallery", "comments", "donate_qr", "empty":
		return true
	default:
		return false
	}
}

func isRegistered(user *model.User) bool {
	return user != nil && user.Role != model.RoleVisitor
}

func mimeTypeByExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		return ""
	}
}
