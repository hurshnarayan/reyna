package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hurshnarayan/reyna/internal/auth"
	"github.com/hurshnarayan/reyna/internal/config"
	"github.com/hurshnarayan/reyna/internal/integrations/gdrive"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/repository"
)

func TestDashboardGroupSettingsLinkUserToStagedFiles(t *testing.T) {
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
	sharer, err := store.UpsertUser("+15550000002", "Sharer")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertGroupRaw("group@g.us", "Testing234", 2); err != nil {
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

	cfg := &config.Config{JWTSecret: "test-secret"}
	server := NewServer(cfg, store, gdrive.New("", "", "", filepath.Join(dir, "drive")), nil)
	token, err := auth.GenerateToken(dashboardUser.ID, cfg.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"group_id":%d,"enabled":true,"tracking_mode":"auto"}`, group.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/groups/settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("settings update status = %d: %s", response.Code, response.Body.String())
	}

	files, err := store.GetGroupsFiles(store.GetUserGroupIDs(dashboardUser.ID), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].FileName != "shared-notes.pdf" {
		t.Fatalf("dashboard-visible files = %v, want the staged file from the managed group", files)
	}
}
