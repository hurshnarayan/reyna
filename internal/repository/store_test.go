package repository

import (
	"math"
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

// TestSearchFilesNLPWhoDoesNotHideUnattributedFiles pins the WHO semantics.
//
// The filter used to be a hard AND on the sender fields, which was right only
// while Baileys supplied the sender as a fact. Once attribution can fail, a
// file that really was Mohit's but could not be attributed would never match
// "what did Mohit share", and the user saw an empty result — indistinguishable
// from the file never having been captured.
//
// The rule is: exclude a file only when we know enough to rule it out. A
// confidently-attributed file from somebody else is excluded. A file we cannot
// attribute is still a candidate, ranked below the confident matches.
func TestSearchFilesNLPWhoDoesNotHideUnattributedFiles(t *testing.T) {
	s := newTestStore(t)
	userID, groupID := seedGroup(t, s, "+911111111111", "Mohit", "group-a@g.us")

	add := func(name, sender, method string, conf float64) {
		t.Helper()
		if _, err := s.AddFile(&model.File{
			GroupID: groupID, UserID: userID,
			FileName: name, SharedByName: sender,
			AttributionMethod: method, AttributionConfidence: conf,
		}); err != nil {
			t.Fatalf("AddFile(%s): %v", name, err)
		}
	}

	add("mohit_confident.pdf", "Mohit", model.AttrBaileys, 1.0)
	add("priya_confident.pdf", "Priya", model.AttrBaileys, 1.0)
	add("unattributed.pdf", "", model.AttrNone, 0)
	add("weak_guess.pdf", "Priya", model.AttrTimeWindow, 0.45)

	files, err := s.SearchFilesNLP([]int64{groupID}, "mohit", "", nil, 20)
	if err != nil {
		t.Fatalf("SearchFilesNLP: %v", err)
	}

	got := map[string]int{}
	for i, f := range files {
		got[f.FileName] = i
	}

	if _, ok := got["mohit_confident.pdf"]; !ok {
		t.Error("confident match missing")
	}
	if _, ok := got["unattributed.pdf"]; !ok {
		t.Error("unattributed file was hidden; it could still be Mohit's")
	}
	if _, ok := got["weak_guess.pdf"]; !ok {
		t.Error("weakly-attributed file was hidden; the guess is not strong enough to rule it out")
	}
	if _, ok := got["priya_confident.pdf"]; ok {
		t.Error("confidently attributed to Priya but returned for a Mohit query")
	}

	if len(files) == 0 || files[0].FileName != "mohit_confident.pdf" {
		t.Errorf("first result = %v, want mohit_confident.pdf ranked above the uncertain ones", files)
	}
}

// TestSearchFilesNLPWhenUsesPostedAt guards the time window against the
// created_at/posted_at split. A file posted last year but downloaded today must
// not surface for "what came in today", and one posted today but inserted late
// must not be missed.
func TestSearchFilesNLPWhenUsesPostedAt(t *testing.T) {
	s := newTestStore(t)
	userID, groupID := seedGroup(t, s, "+911111111111", "Mohit", "group-a@g.us")

	// Inserted now (as all test rows are), but posted a year ago.
	if _, err := s.AddFile(&model.File{
		GroupID: groupID, UserID: userID, FileName: "old_but_downloaded_today.pdf",
		PostedAt: time.Now().AddDate(-1, 0, 0),
	}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if _, err := s.AddFile(&model.File{
		GroupID: groupID, UserID: userID, FileName: "posted_today.pdf",
		PostedAt: time.Now().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	since := time.Now().AddDate(0, 0, -1)
	files, err := s.SearchFilesNLP([]int64{groupID}, "", "", &since, 20)
	if err != nil {
		t.Fatalf("SearchFilesNLP: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].FileName != "posted_today.pdf" {
		t.Errorf("got %q, want posted_today.pdf — the window must apply to posted_at, not created_at", files[0].FileName)
	}
}

func TestSearchFilesNLPReynaScript(t *testing.T) {
	s := newTestStore(t)
	userID, groupID := seedGroup(t, s, "+911111111111", "Device User", "device-group@g.us")

	// Reyna script file captured without sender (unattributed)
	if _, err := s.AddFile(&model.File{
		GroupID:               groupID,
		UserID:                userID,
		FileName:              "Reyna_SIH260150_script.pdf",
		MimeType:              "application/pdf",
		PostedAt:              time.Now().Add(-20 * time.Hour),
		AttributionMethod:     "",
		AttributionConfidence: 0.0,
		SharedByName:          "",
		SharedByPhone:         "",
	}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// 1. Search with what="reyna script"
	files, err := s.SearchFilesNLP([]int64{groupID}, "", "reyna script", nil, 10)
	if err != nil {
		t.Fatalf("SearchFilesNLP: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected to find Reyna_SIH260150_script.pdf for what='reyna script', got 0 files")
	}
	if files[0].FileName != "Reyna_SIH260150_script.pdf" {
		t.Errorf("got %q, want Reyna_SIH260150_script.pdf", files[0].FileName)
	}

	// 2. Search with what="Reyna_SIH260150_script.pdf"
	files, err = s.SearchFilesNLP([]int64{groupID}, "", "Reyna_SIH260150_script.pdf", nil, 10)
	if err != nil {
		t.Fatalf("SearchFilesNLP: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected to find Reyna_SIH260150_script.pdf, got 0 files")
	}

	// 3. Search with what="script"
	files, err = s.SearchFilesNLP([]int64{groupID}, "", "script", nil, 10)
	if err != nil {
		t.Fatalf("SearchFilesNLP: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected to find Reyna_SIH260150_script.pdf for what='script', got 0 files")
	}
}

func TestTokenizeWhatEdgeCases(t *testing.T) {
	if tok := TokenizeWhat(""); tok != nil {
		t.Errorf("TokenizeWhat(\"\") = %v, want nil", tok)
	}
	if tok := TokenizeWhat("   "); tok != nil {
		t.Errorf("TokenizeWhat(\"   \") = %v, want nil", tok)
	}
	tok := TokenizeWhat("can you find me the latest Reyna script received")
	for _, expected := range []string{"reyna", "script"} {
		found := false
		for _, tokItem := range tok {
			if tokItem == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TokenizeWhat missing %q in %v", expected, tok)
		}
	}
}

func TestFileEmbeddings(t *testing.T) {
	s := newTestStore(t)
	user, err := s.UpsertUser("+919999999999", "Test User")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	group, err := s.UpsertGroup("test-group", "Test Group", user.ID)
	if err != nil {
		t.Fatalf("UpsertGroup: %v", err)
	}
	file, err := s.AddFile(&model.File{
		GroupID:  group.ID,
		UserID:   user.ID,
		FileName: "test_circuit.pdf",
		MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = float32(i) * 0.001
	}

	if err := s.SaveFileEmbedding(file.ID, vec); err != nil {
		t.Fatalf("SaveFileEmbedding: %v", err)
	}

	got, err := s.GetFileEmbedding(file.ID)
	if err != nil {
		t.Fatalf("GetFileEmbedding: %v", err)
	}
	if len(got) != 768 {
		t.Fatalf("got len %d, want 768", len(got))
	}
	for i := range vec {
		if math.Abs(float64(got[i]-vec[i])) > 1e-6 {
			t.Fatalf("mismatch at %d: got %v, want %v", i, got[i], vec[i])
		}
	}

	all, err := s.GetAllEmbeddings([]int64{group.ID})
	if err != nil {
		t.Fatalf("GetAllEmbeddings: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetAllEmbeddings returned %d, want 1", len(all))
	}
	if len(all[file.ID]) != 768 {
		t.Fatalf("GetAllEmbeddings file vec len = %d, want 768", len(all[file.ID]))
	}
}
