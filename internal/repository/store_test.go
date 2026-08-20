package repository

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedGroup creates a user and a group they own. Foreign keys are enforced, so
// files cannot reference IDs that do not exist.
func seedGroup(t *testing.T, s *Store, phone, name, waID string) (userID, groupID int64) {
	t.Helper()
	u, err := s.UpsertUser(phone, name)
	if err != nil {
		t.Fatalf("UpsertUser(%s): %v", phone, err)
	}
	g, err := s.UpsertGroup(waID, "Test Group", u.ID)
	if err != nil {
		t.Fatalf("UpsertGroup(%s): %v", waID, err)
	}
	return u.ID, g.ID
}

// TestPostedAtSurvivesRoundTrip guards the distinction between when a message
// was sent and when Reyna inserted the row. Under the Baileys bot those were
// seconds apart and could be used interchangeably; on-device they can be days
// apart, because a file reaches the disk when the user taps download. If this
// regresses, Reyna states the wrong date confidently and the user has no way
// to tell.
//
// It also pins a subtlety that already broke once: posted_at must be selected
// as a bare column, not wrapped in COALESCE. go-sqlite3 only converts a value
// to time.Time when the declared column type is a date type, and the result of
// an expression has no declared type, so it arrives as a string and fails to
// scan. scanFiles skips rows it cannot scan, so the symptom was every file
// silently disappearing.
func TestPostedAtSurvivesRoundTrip(t *testing.T) {
	s := newTestStore(t)
	userID, groupID := seedGroup(t, s, "+911111111111", "Mohit", "group-a@g.us")

	posted := time.Date(2026, 7, 21, 9, 30, 0, 0, time.UTC)
	saved, err := s.AddFile(&model.File{
		GroupID:               groupID,
		UserID:                userID,
		SharedByName:          "Mohit",
		SharedByPhone:         "+911111111111",
		FileName:              "Compiler_Notes.pdf",
		PostedAt:              posted,
		AttributionMethod:     model.AttrBaileys,
		AttributionConfidence: 1.0,
	})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	got, err := s.GetFileByID(saved.ID)
	if err != nil {
		t.Fatalf("GetFileByID: %v", err)
	}
	if !got.PostedAt.Equal(posted) {
		t.Errorf("PostedAt = %v, want %v", got.PostedAt, posted)
	}
	if got.CreatedAt.Equal(posted) {
		t.Error("CreatedAt equals PostedAt; insert time should be now, not the message time")
	}
	if got.AttributionMethod != model.AttrBaileys || got.AttributionConfidence != 1.0 {
		t.Errorf("attribution = %q/%v, want baileys/1", got.AttributionMethod, got.AttributionConfidence)
	}

	// The same values must survive the list path, which scans through scanFiles
	// rather than a single-row Scan.
	files, err := s.GetGroupFiles(groupID, 10)
	if err != nil {
		t.Fatalf("GetGroupFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("GetGroupFiles returned %d files, want 1 (a scan error silently drops rows)", len(files))
	}
	if !files[0].PostedAt.Equal(posted) {
		t.Errorf("listed PostedAt = %v, want %v", files[0].PostedAt, posted)
	}
}

// TestPostedAtFallsBackToCreatedAt covers rows written without a known message
// time: pre-migration rows, and any capture path that could not determine when
// something was posted. SharedAt must never be the zero time, or the UI renders
// year 1.
func TestPostedAtFallsBackToCreatedAt(t *testing.T) {
	s := newTestStore(t)
	userID, groupID := seedGroup(t, s, "+911111111111", "Mohit", "group-a@g.us")

	saved, err := s.AddFile(&model.File{
		GroupID: groupID, UserID: userID, FileName: "unknown_time.pdf",
	})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	got, err := s.GetFileByID(saved.ID)
	if err != nil {
		t.Fatalf("GetFileByID: %v", err)
	}
	if got.PostedAt.IsZero() {
		t.Fatal("PostedAt is zero; should have fallen back to CreatedAt")
	}
	if !got.PostedAt.Equal(got.CreatedAt) {
		t.Errorf("PostedAt = %v, want fallback to CreatedAt %v", got.PostedAt, got.CreatedAt)
	}
	if got.SharedAt().IsZero() {
		t.Error("SharedAt() is zero")
	}
}

// TestUnattributedFileIsNotNamed pins the rule that governs what Reyna is
// allowed to say. A file whose sender was guessed from a time window must not
// be presented as "shared by X", however plausible the guess.
func TestUnattributedFileIsNotNamed(t *testing.T) {
	cases := []struct {
		name       string
		file       model.File
		wantNamed  bool
	}{
		{
			name:      "baileys is authoritative",
			file:      model.File{SharedByName: "Mohit", AttributionMethod: model.AttrBaileys, AttributionConfidence: 1.0},
			wantNamed: true,
		},
		{
			name:      "export match is authoritative",
			file:      model.File{SharedByName: "Mohit", AttributionMethod: model.AttrExport, AttributionConfidence: 1.0},
			wantNamed: true,
		},
		{
			name:      "notification match clears the bar",
			file:      model.File{SharedByName: "Mohit", AttributionMethod: model.AttrNotification, AttributionConfidence: 0.85},
			wantNamed: true,
		},
		{
			name:      "time window guess does not",
			file:      model.File{SharedByName: "Mohit", AttributionMethod: model.AttrTimeWindow, AttributionConfidence: 0.45},
			wantNamed: false,
		},
		{
			name:      "no attribution at all",
			file:      model.File{AttributionMethod: model.AttrNone, AttributionConfidence: 0},
			wantNamed: false,
		},
		{
			name:      "high confidence but no name to give",
			file:      model.File{AttributionMethod: model.AttrExport, AttributionConfidence: 1.0},
			wantNamed: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.file.SenderKnown(); got != tc.wantNamed {
				t.Errorf("SenderKnown() = %v, want %v", got, tc.wantNamed)
			}
		})
	}
}

// TestAutoLinkDoesNotJoinUnrelatedGroups guards the tenancy fix. Membership must
// follow evidence that the phone participated. An earlier revision joined every
// registering user to every group in the database, which exposed every user's
// files to every other user.
func TestAutoLinkDoesNotJoinUnrelatedGroups(t *testing.T) {
	s := newTestStore(t)

	owner, err := s.UpsertUser("+911111111111", "Owner")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	group, err := s.UpsertGroup("group-a@g.us", "Sem 5 CS", owner.ID)
	if err != nil {
		t.Fatalf("UpsertGroup: %v", err)
	}
	if _, err := s.AddFile(&model.File{
		GroupID: group.ID, UserID: owner.ID,
		SharedByPhone: "+911111111111", SharedByName: "Owner",
		FileName: "notes.pdf",
	}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	s.AutoLinkUserToGroups(owner.ID, "+911111111111")

	stranger, err := s.UpsertUser("+919999999999", "Stranger")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	s.AutoLinkUserToGroups(stranger.ID, "+919999999999")

	if ids := s.GetUserGroupIDs(owner.ID); len(ids) != 1 {
		t.Errorf("owner sees %d groups, want 1", len(ids))
	}
	if ids := s.GetUserGroupIDs(stranger.ID); len(ids) != 0 {
		t.Errorf("stranger sees %d groups, want 0 — they were never in any", len(ids))
	}
	files, err := s.GetGroupsFiles(s.GetUserGroupIDs(stranger.ID), 50)
	if err != nil {
		t.Fatalf("GetGroupsFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("stranger sees %d files, want 0", len(files))
	}
}

// TestFindDriveConnectedUserDoesNotFallBackToStrangers guards the other half of
// the tenancy fix. Returning any Drive-connected user in the database meant one
// group's files could be uploaded into the Drive of someone never in it.
func TestFindDriveConnectedUserDoesNotFallBackToStrangers(t *testing.T) {
	s := newTestStore(t)

	outsider, err := s.UpsertUser("+915555555555", "Outsider")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := s.UpdateUserGoogle(outsider.ID, "outsider@example.com", "tok", "refresh", "drive-root-id"); err != nil {
		t.Fatalf("UpdateUserGoogle: %v", err)
	}

	// A group the outsider has nothing to do with.
	_, groupID := seedGroup(t, s, "+911111111111", "Owner", "group-b@g.us")

	if u := s.FindDriveConnectedUser(groupID); u != nil {
		t.Errorf("returned %q for a group they are not in; must return nil", u.Email)
	}

	// Once they are actually a member, they are a legitimate answer.
	if err := s.AddGroupMember(groupID, outsider.ID, "+915555555555", "member"); err != nil {
		t.Fatalf("AddGroupMember: %v", err)
	}
	if u := s.FindDriveConnectedUser(groupID); u == nil {
		t.Error("returned nil for a Drive-connected member of the group")
	}
}
