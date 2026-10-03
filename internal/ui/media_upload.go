//go:build !js

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/server"
	"github.com/fsnotify/fsnotify"
)

func (s *AppState) uploadClient() (*api.Client, error) {
	if api.Enabled() {
		return api.Default()
	}
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.localUploadClient != nil {
		return s.localUploadClient, nil
	}
	client, closeServer, err := server.NewLocalAdminClient(s.Session.User.ID)
	if err != nil {
		return nil, err
	}
	s.localUploadClient, s.localUploadClose = client, closeServer
	return client, nil
}

func (s *AppState) closeLocalUploadClient() {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.localUploadClose != nil {
		s.localUploadClose()
	}
	s.localUploadClient, s.localUploadClose = nil, nil
}

func (s *AppState) showMediaUpload() {
	if !s.Session.IsAdmin() {
		dialog.ShowError(fmt.Errorf("administrator access required"), s.Window)
		return
	}
	client, err := s.uploadClient()
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}
	eras, err := client.Eras()
	if err != nil || len(eras) == 0 {
		if err == nil {
			err = fmt.Errorf("create an era before uploading media")
		}
		dialog.ShowError(err, s.Window)
		return
	}
	eraLabels := make([]string, len(eras))
	eraByLabel := make(map[string]api.Era, len(eras))
	for index, era := range eras {
		eraLabels[index] = era.Title
		eraByLabel[era.Title] = era
	}
	eraSelect := widget.NewSelect(eraLabels, nil)
	albumSelect := widget.NewSelect(nil, nil)
	kindSelect := widget.NewSelect([]string{"photo", "short"}, nil)
	kindSelect.SetSelected("photo")
	caption := widget.NewEntry()
	caption.SetPlaceHolder("A short caption")
	fileLabel := widget.NewLabel("No file selected")
	status := widget.NewLabel("")
	chooseFile := widget.NewButton("Choose media...", nil)

	loadAlbums := func(era api.Era) {
		albums, err := client.Albums(era.Slug)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		labels := make([]string, len(albums))
		for index, album := range albums {
			labels[index] = fmt.Sprintf("%d - %s", album.ID, album.Title)
		}
		albumSelect.Options = labels
		if len(labels) > 0 {
			albumSelect.SetSelected(labels[0])
		} else {
			albumSelect.ClearSelected()
		}
		albumSelect.Refresh()
	}
	eraSelect.OnChanged = func(label string) {
		if era, ok := eraByLabel[label]; ok {
			loadAlbums(era)
		}
	}
	eraSelect.SetSelected(eraLabels[0])
	loadAlbums(eras[0])

	chooseFile.OnTapped = func() {
		picker := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, s.Window)
				return
			}
			if reader == nil {
				return
			}
			if albumSelect.Selected == "" {
				reader.Close()
				status.SetText("Choose an album first.")
				return
			}
			albumID, parseErr := uploadAlbumID(albumSelect.Selected)
			if parseErr != nil {
				reader.Close()
				status.SetText(parseErr.Error())
				return
			}
			fileLabel.SetText(reader.URI().Name())
			chooseFile.Disable()
			status.SetText("Uploading...")
			go func() {
				defer reader.Close()
				_, uploadErr := client.UploadMedia(albumID, kindSelect.Selected, reader.URI().Name(), caption.Text, reader)
				fyne.Do(func() {
					chooseFile.Enable()
					if uploadErr != nil {
						status.SetText(uploadErr.Error())
						return
					}
					status.SetText("Upload complete. Thumbnail generation is running.")
				})
			}()
		}, s.Window)
		picker.SetTitleText("Choose a photo or short video")
		picker.Show()
	}

	content := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Era", eraSelect),
			widget.NewFormItem("Album", albumSelect),
			widget.NewFormItem("Kind", kindSelect),
			widget.NewFormItem("Caption", caption),
			widget.NewFormItem("File", container.NewVBox(chooseFile, fileLabel)),
		),
		status,
	)
	dialog.NewCustom("Upload memory", "Close", container.NewVScroll(content), s.Window).Show()
}

func (s *AppState) startDropWatcher() {
	s.mediaMu.Lock()
	if s.dropWatcherCancel != nil {
		s.mediaMu.Unlock()
		return
	}
	cancel := make(chan struct{})
	s.dropWatcherCancel = cancel
	s.mediaMu.Unlock()
	go s.watchDropFolder(cancel)
}

func (s *AppState) stopDropWatcher() {
	s.mediaMu.Lock()
	if s.dropWatcherCancel != nil {
		close(s.dropWatcherCancel)
		s.dropWatcherCancel = nil
	}
	s.mediaMu.Unlock()
}

func (s *AppState) watchDropFolder(cancel <-chan struct{}) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	root := filepath.Join(home, "mytodo-drop")
	if err := os.MkdirAll(root, 0700); err != nil {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			_ = watcher.Add(path)
		} else {
			go s.ingestDropFile(root, path)
		}
		return nil
	})
	go func() {
		for {
			select {
			case <-cancel:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Rename == 0 {
					continue
				}
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = watcher.Add(event.Name)
					continue
				}
				go func(path string) {
					select {
					case <-cancel:
						return
					case <-time.After(600 * time.Millisecond):
						s.ingestDropFile(root, path)
					}
				}(event.Name)
			case <-watcher.Errors:
			}
		}
	}()
	<-cancel
}

func (s *AppState) ingestDropFile(root, path string) {
	relative, err := filepath.Rel(root, path)
	if err != nil || len(strings.Split(relative, string(filepath.Separator))) != 3 {
		return
	}
	parts := strings.Split(relative, string(filepath.Separator))
	client, err := s.uploadClient()
	if err != nil {
		s.showDropError(err)
		return
	}
	eras, err := client.Eras()
	if err != nil {
		s.showDropError(err)
		return
	}
	var selectedEra *api.Era
	for index := range eras {
		if eras[index].Slug == safeFolder(parts[0]) {
			selectedEra = &eras[index]
			break
		}
	}
	if selectedEra == nil {
		return
	}
	albums, err := client.Albums(selectedEra.Slug)
	if err != nil {
		s.showDropError(err)
		return
	}
	var selectedAlbum *api.Album
	for index := range albums {
		if safeFolder(albums[index].Title) == safeFolder(parts[1]) {
			if selectedAlbum != nil {
				s.showDropError(fmt.Errorf("drop folder album name is ambiguous: %s", parts[1]))
				return
			}
			selectedAlbum = &albums[index]
		}
	}
	if selectedAlbum == nil {
		return
	}
	extension := strings.ToLower(filepath.Ext(path))
	kind := ""
	switch extension {
	case ".jpg", ".jpeg", ".png", ".webp":
		kind = "photo"
	case ".mp4", ".mov", ".webm":
		kind = "short"
	default:
		return
	}
	file, err := os.Open(path)
	if err != nil {
		s.showDropError(err)
		return
	}
	_, err = client.UploadMedia(selectedAlbum.ID, kind, filepath.Base(path), "", file)
	_ = file.Close()
	if err != nil {
		s.showDropError(fmt.Errorf("%s: %w", filepath.Base(path), err))
		return
	}
	if err := os.Remove(path); err != nil {
		s.showDropError(fmt.Errorf("uploaded but could not remove drop file: %w", err))
	}
}

func (s *AppState) showDropError(err error) {
	fyne.Do(func() { dialog.ShowError(err, s.Window) })
}

func uploadAlbumID(value string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(value, "%d", &id); err != nil || id < 1 {
		return 0, fmt.Errorf("choose a valid album")
	}
	return id, nil
}

func safeFolder(value string) string {
	value = strings.ToLower(value)
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
	return result.String()
}
