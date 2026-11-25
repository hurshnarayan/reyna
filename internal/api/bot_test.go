package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hurshnarayan/reyna/internal/config"
	"github.com/hurshnarayan/reyna/internal/integrations/gdrive"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/repository"
)

func TestBotSyncGroupLinksOwnerToStagedFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := repository.New(filepath.Join(dir, "reyna.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer store.Close()

	server := NewServer(
		&config.Config{DeviceToken: "secret"},
		store,
		gdrive.New("", "", "", filepath.Join(dir, "drive")),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/bot/sync-group", strings.NewReader(
		`{"wa_id":"group@g.us","name":"Study Group","owner_phone":"+911234567890"}`,
	))
	req.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("sync status = %d: %s", response.Code, response.Body.String())
	}

	owner, err := store.GetUserByPhone("+911234567890")
	if err != nil {
		t.Fatalf("owner was not created: %v", err)
	}
	group, err := store.GetGroupByWAID("group@g.us")
	if err != nil {
		t.Fatalf("group was not created: %v", err)
	}

	sharer, err := store.UpsertUser("+919876543210", "Sharer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddFile(&model.File{
		GroupID: group.ID, UserID: sharer.ID, FileName: "notes.pdf", Status: "staged",
	}); err != nil {
		t.Fatal(err)
	}

	ownerGroups := store.GetUserGroupIDs(owner.ID)
	files, err := store.GetGroupsFiles(ownerGroups, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].FileName != "notes.pdf" {
		t.Fatalf("owner-visible files = %v, want the staged file", files)
	}
}

func TestBotSyncGroupLinksExistingWhatsAppMembersToStagedFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := repository.New(filepath.Join(dir, "reyna.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dashboardUser, err := store.UpsertUser("+15550000001", "Dashboard")
	if err != nil {
		t.Fatal(err)
	}
	sharer, err := store.UpsertUser("+919876543210", "Sharer")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertGroupRaw("group@g.us", "Study Group", 2); err != nil {
		t.Fatal(err)
	}
	group, err := store.GetGroupByWAID("group@g.us")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddFile(&model.File{
		GroupID: group.ID, UserID: sharer.ID, FileName: "shared-notes.pdf", Status: "staged",
	}); err != nil {
		t.Fatal(err)
	}

	server := NewServer(
		&config.Config{DeviceToken: "secret"},
		store,
		gdrive.New("", "", "", filepath.Join(dir, "drive")),
		nil,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/bot/sync-group", strings.NewReader(
		`{"wa_id":"group@g.us","name":"Study Group","member_count":2,"member_phones":["+15550000001"]}`,
	))
	req.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("sync status = %d: %s", response.Code, response.Body.String())
	}

	files, err := store.GetGroupsFiles(store.GetUserGroupIDs(dashboardUser.ID), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].FileName != "shared-notes.pdf" {
		t.Fatalf("dashboard-visible files = %v, want the staged file from the WhatsApp group", files)
	}
}
