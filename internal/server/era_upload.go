package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "image/png"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxMediaUploadBytes = 128 << 20

var eraSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (s *Server) createEra(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var input struct {
		Slug      string          `json:"slug"`
		Title     string          `json:"title"`
		SortOrder int64           `json:"sort_order"`
		Theme     json.RawMessage `json:"theme"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.Slug = strings.TrimSpace(strings.ToLower(input.Slug))
	input.Title = strings.TrimSpace(input.Title)
	if !eraSlugPattern.MatchString(input.Slug) || input.Title == "" || len(input.Title) > 120 || input.SortOrder < 0 || len(input.Theme) == 0 || len(input.Theme) > 4096 || !json.Valid(input.Theme) {
		writeError(w, http.StatusBadRequest, "invalid era fields or theme JSON")
		return
	}
	result, err := db.DB().Exec(`INSERT INTO era (slug, title, sort_order, theme_json) VALUES (?, ?, ?, ?)`, input.Slug, input.Title, input.SortOrder, string(input.Theme))
	if err != nil {
		writeError(w, http.StatusConflict, "era slug already exists or could not be saved")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, eraResponse{ID: id, Slug: input.Slug, Title: input.Title, SortOrder: input.SortOrder, Theme: input.Theme})
}

func (s *Server) createAlbum(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	eraID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		Title     string `json:"title"`
		SortOrder int64  `json:"sort_order"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len(input.Title) > 120 || input.SortOrder < 0 {
		writeError(w, http.StatusBadRequest, "album title and a non-negative sort order are required")
		return
	}
	result, err := db.DB().Exec(`INSERT INTO album (era_id, title, sort_order) VALUES (?, ?, ?)`, eraID, input.Title, input.SortOrder)
	if err != nil {
		writeError(w, http.StatusBadRequest, "era not found or album could not be saved")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, albumResponse{ID: id, EraID: eraID, Title: input.Title, SortOrder: input.SortOrder})
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	albumID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "a media file is required")
		return
	}
	defer file.Close()
	kind := r.FormValue("kind")
	extension := strings.ToLower(filepath.Ext(filepath.Base(header.Filename)))
	if !supportedMedia(kind, extension) {
		writeError(w, http.StatusBadRequest, "choose a supported image or short video file")
		return
	}
	var eraSlug, albumTitle string
	var eraID int64
	if err := db.DB().QueryRow(`SELECT era.slug, album.title, era.id FROM album JOIN era ON era.id = album.era_id WHERE album.id = ?`, albumID).Scan(&eraSlug, &albumTitle, &eraID); err != nil {
		writeError(w, http.StatusNotFound, "album not found")
		return
	}
	dataDir, err := db.DataDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "media storage is unavailable")
		return
	}
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		writeError(w, http.StatusInternalServerError, "media storage is unavailable")
		return
	}
	temporary, err := os.CreateTemp(dataDir, "media-upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage media upload")
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	bytesWritten, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(file, maxMediaUploadBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil || bytesWritten == 0 || bytesWritten > maxMediaUploadBytes {
		writeError(w, http.StatusBadRequest, "media upload is empty or exceeds 128 MiB")
		return
	}
	contentHash := hex.EncodeToString(hash.Sum(nil))
	var existingID, existingAlbumID int64
	var existingKind string
	if err := db.DB().QueryRow(`SELECT id, album_id, kind FROM media WHERE content_hash = ?`, contentHash).Scan(&existingID, &existingAlbumID, &existingKind); err == nil {
		if existingAlbumID != albumID || existingKind != kind {
			writeError(w, http.StatusConflict, "this file is already stored in another album or as a different media kind")
			return
		}
		writeJSON(w, http.StatusOK, mediaResponse{ID: existingID, Kind: existingKind, URL: fmt.Sprintf("/media/%d", existingID), ThumbURL: fmt.Sprintf("/thumbs/%d", existingID)})
		return
	} else if err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "could not check media hash")
		return
	}

	var duration any
	if kind == "short" {
		probe := s.probeDuration
		if probe == nil {
			probe = probeMediaDuration
		}
		seconds, probeErr := probe(temporaryPath)
		if probeErr != nil {
			writeError(w, http.StatusServiceUnavailable, "could not verify video duration; ffprobe must be installed")
			return
		}
		if seconds <= 0 {
			writeError(w, http.StatusBadRequest, "video duration could not be determined")
			return
		}
		if seconds > 60 {
			writeError(w, http.StatusUnprocessableEntity, "short videos must be 60 seconds or less; this file was rejected without trimming")
			return
		}
		duration = int64(math.Round(seconds))
	} else if err := validateImage(temporaryPath); err != nil {
		writeError(w, http.StatusBadRequest, "file is not a supported image")
		return
	}

	mediaDir := filepath.Join(dataDir, "media", safePathSegment(eraSlug), albumPathSegment(albumTitle, albumID))
	if err := os.MkdirAll(mediaDir, 0750); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create media directory")
		return
	}
	storedPath := filepath.Join(mediaDir, contentHash+extension)
	if err := os.Rename(temporaryPath, storedPath); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store media file")
		return
	}
	var nextOrder int64
	if err := db.DB().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM media WHERE album_id = ?`, albumID).Scan(&nextOrder); err != nil {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusInternalServerError, "could not order album media")
		return
	}
	caption := strings.TrimSpace(r.FormValue("caption"))
	if len(caption) > 1000 {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusBadRequest, "caption must be at most 1000 characters")
		return
	}
	result, err := db.DB().Exec(`INSERT INTO media (album_id, kind, content_hash, duration_s, caption, sort_order) VALUES (?, ?, ?, ?, ?, ?)`, albumID, kind, contentHash, duration, caption, nextOrder)
	if err != nil {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusConflict, "media could not be saved")
		return
	}
	mediaID, err := result.LastInsertId()
	if err != nil {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	thumbnailPath := filepath.Join(dataDir, "media", "thumbs", contentHash+".jpg")
	go createThumbnail(storedPath, thumbnailPath, kind)
	writeJSON(w, http.StatusCreated, mediaResponse{ID: mediaID, Kind: kind, Duration: int64Value(duration), Caption: caption, URL: fmt.Sprintf("/media/%d", mediaID), ThumbURL: fmt.Sprintf("/thumbs/%d", mediaID)})
}

func supportedMedia(kind, extension string) bool {
	switch kind {
	case "photo":
		return extension == ".jpg" || extension == ".jpeg" || extension == ".png" || extension == ".webp"
	case "short":
		return extension == ".mp4" || extension == ".mov" || extension == ".webm"
	default:
		return false
	}
}

func validateImage(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	configuration, _, err := image.DecodeConfig(file)
	if err != nil {
		return err
	}
	if configuration.Width < 1 || configuration.Height < 1 || int64(configuration.Width)*int64(configuration.Height) > 20_000_000 {
		return fmt.Errorf("image dimensions exceed the supported limit")
	}
	return nil
}

func probeMediaDuration(path string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
}

func createThumbnail(sourcePath, thumbnailPath, kind string) {
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0750); err != nil {
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(thumbnailPath), ".thumb-*.jpg")
	if err != nil {
		return
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return
	}
	_ = os.Remove(temporaryPath)
	defer os.Remove(temporaryPath)
	if kind == "short" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := exec.CommandContext(ctx, "ffmpeg", "-y", "-ss", "0.5", "-i", sourcePath, "-frames:v", "1", "-vf", "scale=480:-1", "-q:v", "5", temporaryPath).Run(); err != nil {
			return
		}
		_ = os.Rename(temporaryPath, thumbnailPath)
		return
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return
	}
	decoded, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return
	}
	bounds := decoded.Bounds()
	scale := math.Min(1, math.Min(480/float64(bounds.Dx()), 480/float64(bounds.Dy())))
	width, height := int(math.Round(float64(bounds.Dx())*scale)), int(math.Round(float64(bounds.Dy())*scale))
	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(thumbnail, thumbnail.Bounds(), decoded, bounds, draw.Over, nil)
	output, err := os.Create(temporaryPath)
	if err != nil {
		return
	}
	if err := jpeg.Encode(output, thumbnail, &jpeg.Options{Quality: 82}); err != nil {
		_ = output.Close()
		_ = os.Remove(thumbnailPath)
		return
	}
	_ = output.Close()
	_ = os.Rename(temporaryPath, thumbnailPath)
}

func int64Value(value any) int64 {
	if number, ok := value.(int64); ok {
		return number
	}
	return 0
}
