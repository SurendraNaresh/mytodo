package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

const maxArtifactUploadBytes = 32 << 20

type artifactInput struct {
	EventID     int64
	Title       string
	Description string
}

type artifactDTO struct {
	ID          int64  `json:"id"`
	EventID     int64  `json:"event_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FileName    string `json:"file_name"`
	FileURL     string `json:"file_url"`
	CreatedAt   string `json:"created_at"`
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	eventID := int64(0)
	if rawID := r.URL.Query().Get("event_id"); rawID != "" {
		parsed, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "invalid event_id")
			return
		}
		eventID = parsed
		if !artifactEventVisible(eventID, r) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
	}
	artifacts, err := model.ListArtifacts(eventID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list artifacts")
		return
	}
	result := make([]artifactDTO, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !isAdmin(r) && !artifactEventVisible(artifact.EventID, r) {
			continue
		}
		result = append(result, toArtifactDTO(artifact))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, ok := loadVisibleArtifact(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toArtifactDTO(*artifact))
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, ok := loadVisibleArtifact(w, r)
	if !ok {
		return
	}
	path, err := artifactFilePath(artifact.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifact file path is invalid")
		return
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "artifact file not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not open artifact file")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusInternalServerError, "artifact file is unavailable")
		return
	}
	contentType := artifactMIME(filepath.Ext(artifact.FileName))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	disposition := "attachment"
	if strings.HasPrefix(contentType, "image/") || contentType == "application/pdf" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": artifact.FileName}))
	http.ServeContent(w, r, artifact.FileName, info.ModTime(), file)
}

func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	defer cleanupArtifactForm(r)
	input, err := parseArtifactInput(w, r)
	if err != nil {
		artifactUploadError(w, err)
		return
	}
	if err := validateArtifactInput(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !artifactEventExists(input.EventID) {
		writeError(w, http.StatusBadRequest, "event not found")
		return
	}
	filePath, fileName, err := storeArtifactUpload(w, r)
	if err != nil {
		artifactUploadError(w, err)
		return
	}
	artifact, err := model.CreateArtifact(model.Artifact{
		EventID: input.EventID, Title: input.Title, Description: input.Description,
		FilePath: filePath, FileName: fileName,
	})
	if err != nil {
		removeArtifactFile(filePath)
		writeError(w, http.StatusInternalServerError, "could not save artifact")
		return
	}
	writeJSON(w, http.StatusCreated, toArtifactDTO(*artifact))
}

func (s *Server) updateArtifact(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	defer cleanupArtifactForm(r)
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	artifact, err := model.GetArtifact(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load artifact")
		return
	}
	input, err := parseArtifactInput(w, r)
	if err != nil {
		artifactUploadError(w, err)
		return
	}
	if err := validateArtifactInput(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !artifactEventExists(input.EventID) {
		writeError(w, http.StatusBadRequest, "event not found")
		return
	}
	previousPath := artifact.FilePath
	if r.MultipartForm != nil && len(r.MultipartForm.File["file"]) > 0 {
		filePath, fileName, err := storeArtifactUpload(w, r)
		if err != nil {
			artifactUploadError(w, err)
			return
		}
		artifact.FilePath, artifact.FileName = filePath, fileName
	}
	artifact.EventID, artifact.Title, artifact.Description = input.EventID, input.Title, input.Description
	if err := model.UpdateArtifact(*artifact); err != nil {
		if artifact.FilePath != previousPath {
			removeArtifactFile(artifact.FilePath)
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update artifact")
		return
	}
	if artifact.FilePath != previousPath {
		removeArtifactFile(previousPath)
	}
	writeJSON(w, http.StatusOK, toArtifactDTO(*artifact))
}

func (s *Server) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	artifact, err := model.GetArtifact(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load artifact")
		return
	}
	if err := model.DeleteArtifact(id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete artifact")
		return
	}
	if err := removeArtifactFile(artifact.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "artifact record deleted but file cleanup failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseArtifactInput(w http.ResponseWriter, r *http.Request) (artifactInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxArtifactUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return artifactInput{}, fmt.Errorf("invalid multipart artifact request: %w", err)
	}
	eventID, err := strconv.ParseInt(r.FormValue("event_id"), 10, 64)
	if err != nil || eventID <= 0 {
		return artifactInput{}, fmt.Errorf("event_id must be a positive integer")
	}
	return artifactInput{
		EventID: eventID, Title: strings.TrimSpace(r.FormValue("title")),
		Description: strings.TrimSpace(r.FormValue("description")),
	}, nil
}

func validateArtifactInput(input artifactInput) error {
	if input.EventID <= 0 {
		return fmt.Errorf("event_id must be a positive integer")
	}
	if input.Title == "" || len([]rune(input.Title)) > 160 {
		return fmt.Errorf("title is required and must be at most 160 characters")
	}
	if len([]rune(input.Description)) > 4000 {
		return fmt.Errorf("description must be at most 4000 characters")
	}
	return nil
}

func storeArtifactUpload(w http.ResponseWriter, r *http.Request) (string, string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", "", fmt.Errorf("a file is required")
	}
	defer file.Close()
	filename := filepath.Base(strings.TrimSpace(header.Filename))
	if filename == "." || filename == "" || len(filename) > 255 {
		return "", "", fmt.Errorf("invalid artifact filename")
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if !supportedArtifactExtension(extension) {
		return "", "", fmt.Errorf("unsupported artifact file type")
	}
	dataDir, err := db.DataDir()
	if err != nil {
		return "", "", fmt.Errorf("artifact storage is unavailable")
	}
	artifactDir := filepath.Join(dataDir, "artifacts")
	if err := os.MkdirAll(artifactDir, 0750); err != nil {
		return "", "", fmt.Errorf("artifact storage is unavailable")
	}
	temporary, err := os.CreateTemp(artifactDir, ".artifact-upload-*")
	if err != nil {
		return "", "", fmt.Errorf("could not stage artifact upload")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	seeker, ok := file.(io.Seeker)
	if !ok {
		temporary.Close()
		return "", "", fmt.Errorf("uploaded file cannot be read")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		temporary.Close()
		return "", "", fmt.Errorf("could not inspect uploaded file")
	}
	buffer := make([]byte, 512)
	read, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		temporary.Close()
		return "", "", fmt.Errorf("could not inspect uploaded file")
	}
	if !validArtifactContent(extension, buffer[:read]) {
		temporary.Close()
		return "", "", fmt.Errorf("file contents do not match the filename type")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		temporary.Close()
		return "", "", fmt.Errorf("could not read uploaded file")
	}
	written, err := io.Copy(temporary, io.LimitReader(file, maxArtifactUploadBytes+1))
	if err != nil || written == 0 || written > maxArtifactUploadBytes {
		temporary.Close()
		return "", "", fmt.Errorf("artifact must be non-empty and no larger than 32 MiB")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", "", fmt.Errorf("could not save artifact file")
	}
	if err := temporary.Close(); err != nil {
		return "", "", fmt.Errorf("could not save artifact file")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", fmt.Errorf("could not name artifact file")
	}
	storedName := hex.EncodeToString(random[:]) + extension
	storedPath := filepath.Join(artifactDir, storedName)
	if err := os.Rename(temporaryPath, storedPath); err != nil {
		return "", "", fmt.Errorf("could not store artifact file")
	}
	return filepath.ToSlash(filepath.Join("artifacts", storedName)), filename, nil
}

func supportedArtifactExtension(extension string) bool {
	switch extension {
	case ".pdf", ".png", ".jpg", ".jpeg", ".webp", ".txt":
		return true
	default:
		return false
	}
}

func validArtifactContent(extension string, content []byte) bool {
	switch extension {
	case ".pdf":
		return strings.HasPrefix(string(content), "%PDF-")
	case ".png":
		return len(content) >= 8 && string(content[:8]) == "\x89PNG\r\n\x1a\n"
	case ".jpg", ".jpeg":
		return len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff
	case ".webp":
		return len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP"
	case ".txt":
		return utf8.Valid(content) && !strings.ContainsRune(string(content), '\x00')
	default:
		return false
	}
}

func artifactMIME(extension string) string {
	switch strings.ToLower(extension) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func artifactFilePath(relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid stored path")
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) != 2 || parts[0] != "artifacts" || parts[1] == "" || filepath.Base(parts[1]) != parts[1] {
		return "", fmt.Errorf("invalid stored path")
	}
	dataDir, err := db.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, clean), nil
}

func artifactUploadError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if strings.Contains(err.Error(), "storage") || strings.Contains(err.Error(), "stage") || strings.Contains(err.Error(), "save") || strings.Contains(err.Error(), "store") || strings.Contains(err.Error(), "name artifact") {
		status = http.StatusInternalServerError
	}
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		status = http.StatusRequestEntityTooLarge
	}
	writeError(w, status, err.Error())
}

func cleanupArtifactForm(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

func artifactEventExists(id int64) bool {
	_, _, err := getEventState(id)
	return err == nil
}

func artifactEventVisible(id int64, r *http.Request) bool {
	event, _, err := getEventState(id)
	return err == nil && (isAdmin(r) || eventIsVisibleTo(event, currentUser(r).ID))
}

func loadVisibleArtifact(w http.ResponseWriter, r *http.Request) (*model.Artifact, bool) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	artifact, err := model.GetArtifact(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load artifact")
		return nil, false
	}
	if !artifactEventVisible(artifact.EventID, r) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return nil, false
	}
	return artifact, true
}

func toArtifactDTO(artifact model.Artifact) artifactDTO {
	return artifactDTO{
		ID: artifact.ID, EventID: artifact.EventID, Title: artifact.Title,
		Description: artifact.Description, FileName: artifact.FileName,
		FileURL: fmt.Sprintf("/api/v1/artifacts/%d/file", artifact.ID), CreatedAt: artifact.CreatedAt,
	}
}

func removeArtifactFile(relative string) error {
	path, err := artifactFilePath(relative)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
