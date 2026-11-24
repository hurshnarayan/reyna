package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hurshnarayan/reyna/internal/config"
	"github.com/hurshnarayan/reyna/internal/integrations/gdrive"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/repository"
)

func TestDeviceFileContentRequiresTokenAndReturnsArchivedBytes(t *testing.T) {
	dir := t.TempDir()
	store, err := repository.New(filepath.Join(dir, "reyna.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	drive := gdrive.New("", "", "", filepath.Join(dir, "drive"))
	user, err := store.UpsertUser("device", "")
	if err != nil {
		t.Fatal(err)
	}
	group, err := store.UpsertGroup("device", "Device", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("preview bytes")
	file, err := store.AddFile(&model.File{
		GroupID: group.ID, UserID: user.ID, FileName: "notes.pdf",
		FileSize: int64(len(want)), MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := drive.SaveLocalFileData(file.ID, want); err != nil {
		t.Fatal(err)
	}

	server := NewServer(&config.Config{DeviceToken: "secret"}, store, drive, nil)
	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/device/files/content?file_id=1", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("without token: got %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/device/files/content?file_id=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("with token: got %d: %s", response.Code, response.Body.String())
	}
	got, _ := io.ReadAll(response.Body)
	if string(got) != string(want) {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if gotType := response.Header().Get("Content-Type"); gotType != "application/pdf" {
		t.Fatalf("Content-Type = %q", gotType)
	}
}
