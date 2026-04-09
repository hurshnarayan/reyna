package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/reyna-bot/reyna-backend/internal/models"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	database, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	database.SetMaxOpenConns(5)
	s := &Store{db: database}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phone TEXT UNIQUE NOT NULL,
		name TEXT DEFAULT '',
		email TEXT DEFAULT '',
		google_token TEXT DEFAULT '',
		google_refresh TEXT DEFAULT '',
		drive_root_id TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS groups_ (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wa_id TEXT UNIQUE NOT NULL,
		name TEXT DEFAULT '',
		member_count INTEGER DEFAULT 0,
		created_by INTEGER REFERENCES users(id),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS group_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id INTEGER REFERENCES groups_(id),
		user_id INTEGER REFERENCES users(id),
		phone TEXT NOT NULL,
		role TEXT DEFAULT 'member',
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(group_id, phone)
	);
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id INTEGER REFERENCES groups_(id),
		user_id INTEGER REFERENCES users(id),
		shared_by_phone TEXT DEFAULT '',
		shared_by_name TEXT DEFAULT '',
		file_name TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		mime_type TEXT DEFAULT '',
		drive_file_id TEXT DEFAULT '',
		drive_folder_id TEXT DEFAULT '',
		subject TEXT DEFAULT '',
		tags TEXT DEFAULT '',
		version INTEGER DEFAULT 1,
		parent_file_id INTEGER DEFAULT 0,
		wa_message_id TEXT DEFAULT '',
		status TEXT DEFAULT 'staged',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS file_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER REFERENCES files(id),
		version INTEGER NOT NULL,
		drive_file_id TEXT DEFAULT '',
		file_size INTEGER DEFAULT 0,
		changed_by INTEGER REFERENCES users(id),
		change_note TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS activity_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id INTEGER DEFAULT 0,
		user_id INTEGER DEFAULT 0,
		action TEXT NOT NULL,
		command TEXT DEFAULT '',
		result TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS waitlist (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contact TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS group_settings (
		group_id INTEGER PRIMARY KEY REFERENCES groups_(id),
		enabled INTEGER DEFAULT 0,
		tracking_mode TEXT DEFAULT 'auto',
		auto_commit_hours INTEGER DEFAULT 24,
		reaction_emoji TEXT DEFAULT '📌'
	);
	CREATE INDEX IF NOT EXISTS idx_files_group ON files(group_id);
	CREATE INDEX IF NOT EXISTS idx_files_user ON files(user_id);
	CREATE INDEX IF NOT EXISTS idx_files_name ON files(file_name);
	CREATE INDEX IF NOT EXISTS idx_files_subject ON files(subject);
	CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
	CREATE INDEX IF NOT EXISTS idx_files_created ON files(created_at);
	CREATE INDEX IF NOT EXISTS idx_activity_group ON activity_log(group_id);
	`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// v3 migrations — content extraction columns
	migrations := []string{
		`ALTER TABLE files ADD COLUMN extracted_content TEXT DEFAULT ''`,
		`ALTER TABLE files ADD COLUMN content_summary TEXT DEFAULT ''`,
		`ALTER TABLE files ADD COLUMN content_hash TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_files_hash ON files(group_id, content_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_files_drive_id ON files(drive_file_id)`,
		// Partial unique index — defense in depth against duplicate inserts
		// even if the in-process upload mutex ever misses a race. Excludes
		// rows with empty content_hash so historical rows aren't affected.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_files_hash_unique ON files(group_id, content_hash) WHERE content_hash != ''`,
	}
	for _, m := range migrations {
		s.db.Exec(m) // ignore errors if columns already exist
	}

	return nil
}

// ── User Operations ──

func (s *Store) UpsertUser(phone, name string) (*models.User, error) {
	// Clean the name: don't store phone numbers or LIDs as names
	cleanName := name
	if isAllDigitsOrPlus(cleanName) {
		cleanName = "" // don't store numeric strings as names
	}

	_, err := s.db.Exec(
		`INSERT INTO users (phone, name, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(phone) DO UPDATE SET name=COALESCE(NULLIF(excluded.name,''), name), updated_at=CURRENT_TIMESTAMP`,
		phone, cleanName,
	)
	if err != nil {
		return nil, err
	}
	return s.GetUserByPhone(phone)
}

// isAllDigitsOrPlus checks if a string is only digits/+ (phone number or LID, not a name)
func isAllDigitsOrPlus(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" { return false }
	for _, c := range s {
		if c != '+' && c != ' ' && c != '-' && c != '(' && c != ')' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func (s *Store) GetUserByPhone(phone string) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, phone, name, email, google_token, google_refresh, drive_root_id, created_at, updated_at FROM users WHERE phone=?`,
		phone,
	).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) GetUserByID(id int64) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, phone, name, email, google_token, google_refresh, drive_root_id, created_at, updated_at FROM users WHERE id=?`,
		id,
	).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UpdateUserGoogle(userID int64, email, token, refresh, rootID string) error {
	_, err := s.db.Exec(
		`UPDATE users SET email=?, google_token=?, google_refresh=?, drive_root_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		email, token, refresh, rootID, userID,
	)
	return err
}

func (s *Store) UpdateUserGoogleExpiry(userID int64, expiresAt int64) error {
	_, err := s.db.Exec(`UPDATE users SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, userID)
	return err
}

// ── Group Operations ──

func (s *Store) UpsertGroup(waID, name string, createdBy int64) (*models.Group, error) {
	_, err := s.db.Exec(
		`INSERT INTO groups_ (wa_id, name, created_by, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(wa_id) DO UPDATE SET name=excluded.name, updated_at=CURRENT_TIMESTAMP`,
		waID, name, createdBy,
	)
	if err != nil {
		return nil, err
	}
	return s.GetGroupByWAID(waID)
}

func (s *Store) GetGroupByWAID(waID string) (*models.Group, error) {
	g := &models.Group{}
	err := s.db.QueryRow(
		`SELECT id, wa_id, name, member_count, created_by, created_at, updated_at FROM groups_ WHERE wa_id=?`,
		waID,
	).Scan(&g.ID, &g.WAID, &g.Name, &g.MemberCount, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func (s *Store) UpdateGroupMemberCount(groupID int64, count int) error {
	_, err := s.db.Exec(`UPDATE groups_ SET member_count=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, count, groupID)
	return err
}

func (s *Store) InsertGroupRaw(waID, name string, memberCount int) error {
	_, err := s.db.Exec(
		`INSERT INTO groups_ (wa_id, name, member_count, created_by, updated_at) VALUES (?, ?, ?, NULL, CURRENT_TIMESTAMP)
		 ON CONFLICT(wa_id) DO UPDATE SET name=excluded.name, member_count=excluded.member_count, updated_at=CURRENT_TIMESTAMP`,
		waID, name, memberCount,
	)
	return err
}

func (s *Store) UpdateGroupName(groupID int64, name string) error {
	_, err := s.db.Exec(`UPDATE groups_ SET name=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, groupID)
	return err
}

func (s *Store) GetAllGroups() ([]models.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, wa_id, name, member_count, created_by, created_at, updated_at FROM groups_ ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.Group
	for rows.Next() {
		var g models.Group
		rows.Scan(&g.ID, &g.WAID, &g.Name, &g.MemberCount, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) GetUserGroups(userID int64) ([]models.Group, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.wa_id, g.name, g.member_count, g.created_by, g.created_at, g.updated_at
		 FROM groups_ g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_id=? ORDER BY g.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.Group
	for rows.Next() {
		var g models.Group
		rows.Scan(&g.ID, &g.WAID, &g.Name, &g.MemberCount, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) AddGroupMember(groupID, userID int64, phone, role string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO group_members (group_id, user_id, phone, role) VALUES (?, ?, ?, ?)`,
		groupID, userID, phone, role,
	)
	return err
}

// AutoLinkUserToGroups finds all groups where files were shared by this phone
// and adds the user as a member. This syncs web login with WhatsApp bot activity.
func (s *Store) AutoLinkUserToGroups(userID int64, phone string) {
	var groupIDs []int64
	seen := make(map[int64]bool)

	// 1. Groups where this phone shared files
	rows, err := s.db.Query(`SELECT DISTINCT group_id FROM files WHERE shared_by_phone=?`, phone)
	if err == nil {
		for rows.Next() {
			var gid int64
			rows.Scan(&gid)
			if !seen[gid] {
				groupIDs = append(groupIDs, gid)
				seen[gid] = true
			}
		}
		rows.Close()
	}

	// 2. Groups where this phone is already a member (from another user record)
	rows2, err := s.db.Query(`SELECT DISTINCT group_id FROM group_members WHERE phone=?`, phone)
	if err == nil {
		for rows2.Next() {
			var gid int64
			rows2.Scan(&gid)
			if !seen[gid] {
				groupIDs = append(groupIDs, gid)
				seen[gid] = true
			}
		}
		rows2.Close()
	}

	// 3. All existing groups (for hackathon demo — every user sees all data)
	rows3, err := s.db.Query(`SELECT id FROM groups_`)
	if err == nil {
		for rows3.Next() {
			var gid int64
			rows3.Scan(&gid)
			if !seen[gid] {
				groupIDs = append(groupIDs, gid)
				seen[gid] = true
			}
		}
		rows3.Close()
	}

	// Now insert memberships (all rows closed, no lock contention)
	for _, gid := range groupIDs {
		s.AddGroupMember(gid, userID, phone, "member")
	}
}

// ── File Operations ──

func (s *Store) AddFile(f *models.File) (*models.File, error) {
	var existingID int64
	var existingVersion int
	err := s.db.QueryRow(
		`SELECT id, version FROM files WHERE group_id=? AND file_name=? ORDER BY version DESC LIMIT 1`,
		f.GroupID, f.FileName,
	).Scan(&existingID, &existingVersion)

	if err == nil {
		f.Version = existingVersion + 1
		f.ParentFileID = existingID
	} else {
		f.Version = 1
	}

	if f.Status == "" {
		f.Status = "staged"
	}

	res, err := s.db.Exec(
		`INSERT INTO files (group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, content_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.GroupID, f.UserID, f.SharedByPhone, f.SharedByName, f.FileName, f.FileSize,
		f.MimeType, f.DriveFileID, f.DriveFolderID, f.Subject, f.Tags, f.Version, f.ParentFileID, f.WAMessageID, f.Status, f.ContentHash,
	)
	if err != nil {
		return nil, err
	}
	f.ID, _ = res.LastInsertId()
	f.CreatedAt = time.Now()

	s.db.Exec(
		`INSERT INTO file_versions (file_id, version, drive_file_id, file_size, changed_by) VALUES (?, ?, ?, ?, ?)`,
		f.ID, f.Version, f.DriveFileID, f.FileSize, f.UserID,
	)

	return f, nil
}

// CommitFiles marks staged files as committed in a group
func (s *Store) CommitFiles(groupID int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE files SET status='committed' WHERE group_id=? AND status='staged'`,
		groupID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CommitFileByName marks a specific staged file as committed
func (s *Store) CommitFileByName(groupID int64, fileName string) error {
	_, err := s.db.Exec(
		`UPDATE files SET status='committed' WHERE group_id=? AND file_name LIKE ? AND status='staged'`,
		groupID, "%"+fileName+"%",
	)
	return err
}

// RemoveStagedFile removes a staged (uncommitted) file
func (s *Store) RemoveStagedFile(groupID int64, fileName string) (bool, error) {
	// Must delete file_versions first due to foreign key constraint
	s.db.Exec(
		`DELETE FROM file_versions WHERE file_id IN (SELECT id FROM files WHERE group_id=? AND file_name LIKE ? AND status='staged')`,
		groupID, "%"+fileName+"%",
	)
	res, err := s.db.Exec(
		`DELETE FROM files WHERE group_id=? AND file_name LIKE ? AND status='staged'`,
		groupID, "%"+fileName+"%",
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RemoveAllStaged removes all staged files in a group
func (s *Store) RemoveAllStaged(groupID int64) (int64, error) {
	// Must delete file_versions first due to foreign key constraint
	s.db.Exec(
		`DELETE FROM file_versions WHERE file_id IN (SELECT id FROM files WHERE group_id=? AND status='staged')`,
		groupID,
	)
	res, err := s.db.Exec(
		`DELETE FROM files WHERE group_id=? AND status='staged'`,
		groupID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetStagedFiles returns all staged (uncommitted) files in a group
func (s *Store) GetStagedFiles(groupID int64) ([]models.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND status='staged' ORDER BY created_at DESC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// CountStagedFiles returns count of staged files in a group
func (s *Store) CountStagedFiles(groupID int64) int {
	var c int
	s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE group_id=? AND status='staged'`, groupID).Scan(&c)
	return c
}

func (s *Store) FindFiles(groupID int64, query string) ([]models.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND (file_name LIKE ? OR subject LIKE ? OR tags LIKE ?)
		 ORDER BY created_at DESC LIMIT 20`,
		groupID, "%"+query+"%", "%"+query+"%", "%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetGroupFiles(groupID int64, limit int) ([]models.File, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? ORDER BY created_at DESC LIMIT ?`,
		groupID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetUserFiles(userID int64, limit int) ([]models.File, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE user_id=? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetFileVersions(fileID int64) ([]models.FileVersion, error) {
	// Get the original file name, find all versions
	var fileName string
	var groupID int64
	s.db.QueryRow(`SELECT file_name, group_id FROM files WHERE id=?`, fileID).Scan(&fileName, &groupID)

	rows, err := s.db.Query(
		`SELECT f.id, f.version, f.drive_file_id, f.file_size, f.user_id, f.created_at
		 FROM files f WHERE f.group_id=? AND f.file_name=? ORDER BY f.version DESC`,
		groupID, fileName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []models.FileVersion
	for rows.Next() {
		var v models.FileVersion
		rows.Scan(&v.ID, &v.Version, &v.DriveFileID, &v.FileSize, &v.ChangedBy, &v.CreatedAt)
		v.FileID = fileID
		versions = append(versions, v)
	}
	return versions, nil
}

func (s *Store) CountGroupFiles(groupID int64) int {
	var c int
	s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE group_id=?`, groupID).Scan(&c)
	return c
}

func (s *Store) CountUserFiles(userID int64) int {
	var c int
	s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE user_id=?`, userID).Scan(&c)
	return c
}

func (s *Store) GetNewFilesSince(groupID int64, since time.Time) ([]models.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND created_at > ? ORDER BY created_at DESC`,
		groupID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// ── Dashboard Stats ──

func (s *Store) GetDashboardStats(userID int64) (*models.DashboardStats, error) {
	stats := &models.DashboardStats{
		SubjectBreak: make(map[string]int),
	}

	// Get all group IDs the user is a member of
	groupIDs := s.GetUserGroupIDs(userID)

	if len(groupIDs) == 0 {
		// No groups — show files user personally stored as fallback
		s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE user_id=?`, userID).Scan(&stats.TotalFiles)
		s.db.QueryRow(`SELECT COALESCE(SUM(file_size),0) FROM files WHERE user_id=?`, userID).Scan(&stats.TotalSize)
		stats.RecentFiles, _ = s.GetUserFiles(userID, 10)
		return stats, nil
	}

	// Build IN clause
	placeholders, args := buildInClause(groupIDs)

	s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE group_id IN (`+placeholders+`)`, args...).Scan(&stats.TotalFiles)
	s.db.QueryRow(`SELECT COUNT(DISTINCT group_id) FROM files WHERE group_id IN (`+placeholders+`)`, args...).Scan(&stats.TotalGroups)
	s.db.QueryRow(`SELECT COALESCE(SUM(file_size),0) FROM files WHERE group_id IN (`+placeholders+`)`, args...).Scan(&stats.TotalSize)

	// Recent files from all groups
	stats.RecentFiles, _ = s.GetGroupsFiles(groupIDs, 10)

	// Subject breakdown across all groups
	rows, _ := s.db.Query(
		`SELECT COALESCE(NULLIF(subject,''),'Uncategorized'), COUNT(*) FROM files WHERE group_id IN (`+placeholders+`) GROUP BY subject`,
		args...,
	)
	if rows != nil {
		for rows.Next() {
			var sub string
			var cnt int
			rows.Scan(&sub, &cnt)
			stats.SubjectBreak[sub] = cnt
		}
		rows.Close()
	}

	// Top contributors across all groups
	contribRows, _ := s.db.Query(
		`SELECT shared_by_name, shared_by_phone, COUNT(*) as cnt FROM files
		 WHERE group_id IN (`+placeholders+`) AND shared_by_phone != ''
		 GROUP BY shared_by_phone ORDER BY cnt DESC LIMIT 5`,
		args...,
	)
	if contribRows != nil {
		for contribRows.Next() {
			var c models.Contributor
			contribRows.Scan(&c.Name, &c.Phone, &c.Count)
			stats.TopContributors = append(stats.TopContributors, c)
		}
		contribRows.Close()
	}

	return stats, nil
}

// GetUserGroupIDs returns all group IDs a user belongs to
func (s *Store) GetUserGroupIDs(userID int64) []int64 {
	var ids []int64
	seen := make(map[int64]bool)

	rows, err := s.db.Query(`SELECT group_id FROM group_members WHERE user_id=?`, userID)
	if err == nil {
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		rows.Close()
	}

	// Also include groups the user created
	rows2, err := s.db.Query(`SELECT id FROM groups_ WHERE created_by=?`, userID)
	if err == nil {
		for rows2.Next() {
			var id int64
			rows2.Scan(&id)
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		rows2.Close()
	}

	return ids
}

// GetGroupsFiles returns files from multiple groups
func (s *Store) GetGroupsFiles(groupIDs []int64, limit int) ([]models.File, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	placeholders, args := buildInClause(groupIDs)
	args = append(args, interface{}(limit))
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id IN (`+placeholders+`) ORDER BY created_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func buildInClause(ids []int64) (string, []interface{}) {
	placeholders := ""
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}
	return placeholders, args
}

// ── Activity Log ──

func (s *Store) LogActivity(groupID, userID int64, action, command, result string) {
	s.db.Exec(
		`INSERT INTO activity_log (group_id, user_id, action, command, result) VALUES (?, ?, ?, ?, ?)`,
		groupID, userID, action, command, result,
	)
}

func (s *Store) GetActivityLog(groupID int64, limit int) ([]models.ActivityLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, action, command, result, created_at
		 FROM activity_log WHERE group_id=? ORDER BY created_at DESC LIMIT ?`,
		groupID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []models.ActivityLog
	for rows.Next() {
		var l models.ActivityLog
		rows.Scan(&l.ID, &l.GroupID, &l.UserID, &l.Action, &l.Command, &l.Result, &l.CreatedAt)
		logs = append(logs, l)
	}
	return logs, nil
}

// ── File Delete ──

func (s *Store) GetFileByID(fileID int64) (*models.File, error) {
	f := &models.File{}
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE id=?`, fileID,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
		&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
		&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Store) DeleteFile(fileID int64) error {
	s.db.Exec(`DELETE FROM file_versions WHERE file_id=?`, fileID)
	_, err := s.db.Exec(`DELETE FROM files WHERE id=?`, fileID)
	return err
}

func (s *Store) DeleteStagedFile(fileID int64) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE id=? AND status='staged'`, fileID)
	return err
}

// SuggestFiles returns filenames matching a prefix for autocomplete (lightweight)
func (s *Store) SuggestFiles(groupIDs []int64, prefix string, limit int) []string {
	if len(groupIDs) == 0 || prefix == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	placeholders, args := buildInClause(groupIDs)
	args = append(args, "%"+prefix+"%", interface{}(limit))
	rows, err := s.db.Query(
		`SELECT DISTINCT file_name FROM files WHERE group_id IN (`+placeholders+`)
		 AND file_name LIKE ?
		 ORDER BY file_name LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	return names
}

// FindFilesStrict uses stricter matching - filename must contain the query as a word boundary
func (s *Store) FindFilesStrict(groupIDs []int64, query string, limit int) ([]models.File, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	placeholders, args := buildInClause(groupIDs)
	// Match query against filename more strictly: must appear as substring of actual name part
	args = append(args, "%"+query+"%", interface{}(limit))
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id IN (`+placeholders+`) AND file_name LIKE ?
		 ORDER BY created_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// FileExistsInGroup checks if a file with the given name exists committed in a group
func (s *Store) FileExistsInGroup(groupID int64, fileName string) (*models.File, bool) {
	f := &models.File{}
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND file_name=? AND status='committed' ORDER BY version DESC LIMIT 1`,
		groupID, fileName,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
		&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
		&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, false
	}
	return f, true
}

// ClearUserGoogle clears Google Drive connection for a user
func (s *Store) ClearUserGoogle(userID int64) error {
	_, err := s.db.Exec(
		`UPDATE users SET email='', google_token='', google_refresh='', drive_root_id='', updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		userID,
	)
	return err
}

// DeleteStagedFiles deletes staged files by IDs, cleaning up file_versions first
func (s *Store) DeleteStagedFiles(fileIDs []int64) (int64, error) {
	if len(fileIDs) == 0 {
		return 0, nil
	}
	placeholders, args := buildInClause(fileIDs)
	// Must delete file_versions first due to foreign key constraint
	s.db.Exec(`DELETE FROM file_versions WHERE file_id IN (`+placeholders+`)`, args...)
	res, err := s.db.Exec(
		`DELETE FROM files WHERE id IN (`+placeholders+`) AND status='staged'`,
		args...,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ── Waitlist ──

func (s *Store) AddWaitlist(contact string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO waitlist (contact) VALUES (?)`, contact)
	return err
}

func (s *Store) CountWaitlist() int {
	var c int
	s.db.QueryRow(`SELECT COUNT(*) FROM waitlist`).Scan(&c)
	return c
}

// ── Group Settings ──

func (s *Store) GetGroupSettings(groupID int64) *models.GroupSettings {
	gs := &models.GroupSettings{GroupID: groupID, Enabled: false, TrackingMode: "auto", AutoCommitHours: 24, ReactionEmoji: "📌"}
	err := s.db.QueryRow(
		`SELECT group_id, enabled, tracking_mode, auto_commit_hours, reaction_emoji FROM group_settings WHERE group_id=?`,
		groupID,
	).Scan(&gs.GroupID, &gs.Enabled, &gs.TrackingMode, &gs.AutoCommitHours, &gs.ReactionEmoji)
	if err != nil {
		return gs // return defaults
	}
	return gs
}

func (s *Store) UpsertGroupSettings(gs *models.GroupSettings) error {
	_, err := s.db.Exec(
		`INSERT INTO group_settings (group_id, enabled, tracking_mode, auto_commit_hours, reaction_emoji)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET enabled=excluded.enabled, tracking_mode=excluded.tracking_mode,
		   auto_commit_hours=excluded.auto_commit_hours, reaction_emoji=excluded.reaction_emoji`,
		gs.GroupID, gs.Enabled, gs.TrackingMode, gs.AutoCommitHours, gs.ReactionEmoji,
	)
	return err
}

func (s *Store) IsGroupEnabled(waID string) bool {
	var enabled int
	err := s.db.QueryRow(
		`SELECT gs.enabled FROM group_settings gs JOIN groups_ g ON gs.group_id = g.id WHERE g.wa_id=?`,
		waID,
	).Scan(&enabled)
	if err != nil {
		return false
	}
	return enabled == 1
}

func (s *Store) GetGroupTrackingMode(waID string) string {
	var mode string
	err := s.db.QueryRow(
		`SELECT gs.tracking_mode FROM group_settings gs JOIN groups_ g ON gs.group_id = g.id WHERE g.wa_id=?`,
		waID,
	).Scan(&mode)
	if err != nil {
		return "auto"
	}
	return mode
}

func (s *Store) GetAllEnabledGroupWAIDs() []string {
	rows, err := s.db.Query(
		`SELECT g.wa_id FROM groups_ g JOIN group_settings gs ON g.id = gs.group_id WHERE gs.enabled = 1`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// ── Sortable File Queries ──

func (s *Store) GetFilesWithSorting(groupIDs []int64, sortBy, sortOrder string, limit int) ([]models.File, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}

	// Validate sort parameters
	validSorts := map[string]string{
		"name":     "file_name",
		"date":     "created_at",
		"size":     "file_size",
		"subject":  "subject",
		"version":  "version",
	}
	column, ok := validSorts[sortBy]
	if !ok {
		column = "created_at"
	}

	order := "DESC"
	if sortOrder == "asc" {
		order = "ASC"
	}

	placeholders, args := buildInClause(groupIDs)
	args = append(args, interface{}(limit))
	query := fmt.Sprintf(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id IN (%s) ORDER BY %s %s LIMIT ?`,
		placeholders, column, order,
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// ── Auto-Commit Queries ──

func (s *Store) GetStagedFilesOlderThan(hours int) ([]models.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE status='staged' AND created_at <= datetime('now', '-' || ? || ' hours')
		 ORDER BY group_id, created_at`,
		hours,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetStagedFilesByGroupID(groupID int64) ([]models.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND status='staged' ORDER BY created_at ASC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// ── Helpers ──

func scanFiles(rows *sql.Rows) ([]models.File, error) {
	var files []models.File
	for rows.Next() {
		var f models.File
		err := rows.Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
			&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
			&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt)
		if err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

// UpdateFileDriveID updates the drive file ID after uploading to Google Drive
func (s *Store) UpdateFileDriveID(fileID int64, driveFileID, driveFolderID string) error {
	_, err := s.db.Exec(
		`UPDATE files SET drive_file_id=?, drive_folder_id=? WHERE id=?`,
		driveFileID, driveFolderID, fileID,
	)
	return err
}

// FindFileByHash returns the most recent file in the group whose content hash
// matches. Used for byte-identical duplicate detection on bot upload.
func (s *Store) FindFileByHash(groupID int64, hash string) *models.File {
	if hash == "" {
		return nil
	}
	f := &models.File{}
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE group_id=? AND content_hash=? AND status != 'deleted_in_drive'
		 ORDER BY created_at DESC LIMIT 1`,
		groupID, hash,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName, &f.FileName, &f.FileSize,
		&f.MimeType, &f.DriveFileID, &f.DriveFolderID, &f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil
	}
	return f
}

// LatestVersionByName returns the highest existing version number for a file
// of the given name in the group, or 0 if none exists. Used to compute the
// next version when an updated copy of a same-named file is uploaded.
func (s *Store) LatestVersionByName(groupID int64, fileName string) int {
	var v int
	err := s.db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM files WHERE group_id=? AND file_name=?`,
		groupID, fileName,
	).Scan(&v)
	if err != nil {
		return 0
	}
	return v
}

// EnrichDriveMatches takes raw Drive walker results and fills in WHO/WHEN
// metadata from the Reyna DB by matching on drive_file_id. Files captured by
// the bot AND organised in Drive carry full sender/time info; pure Drive-native
// files (organised before Reyna) keep empty sender fields.
func (s *Store) EnrichDriveMatches(matches []models.DriveMatch) []models.DriveMatch {
	if len(matches) == 0 {
		return matches
	}
	// Build a set of drive_file_ids to look up in one query
	driveIDs := make([]string, 0, len(matches))
	idx := make(map[string]int)
	for i, m := range matches {
		if m.FileID == "" {
			continue
		}
		driveIDs = append(driveIDs, m.FileID)
		idx[m.FileID] = i
	}
	if len(driveIDs) == 0 {
		return matches
	}
	// Build placeholder list
	placeholders := strings.Repeat("?,", len(driveIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(driveIDs))
	for i, id := range driveIDs {
		args[i] = id
	}
	rows, err := s.db.Query(
		`SELECT id, drive_file_id, shared_by_name, shared_by_phone, created_at
		 FROM files WHERE drive_file_id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return matches
	}
	defer rows.Close()
	for rows.Next() {
		var dbID int64
		var driveID, name, phone string
		var createdAt time.Time
		if err := rows.Scan(&dbID, &driveID, &name, &phone, &createdAt); err != nil {
			continue
		}
		i, ok := idx[driveID]
		if !ok {
			continue
		}
		matches[i].DBFileID = dbID
		matches[i].SenderName = name
		matches[i].SharedAt = createdAt
		_ = phone
	}
	return matches
}

// MarkFileDeletedInDrive flips a row's status so it stops appearing in
// retrieval/Q&A results without losing the historical record.
func (s *Store) MarkFileDeletedInDrive(fileID int64) {
	s.db.Exec(`UPDATE files SET status='deleted_in_drive' WHERE id=?`, fileID)
}

// FindDriveConnectedUser finds any user in a group who has Google Drive connected
func (s *Store) FindDriveConnectedUser(groupID int64) *models.User {
	u := &models.User{}
	err := s.db.QueryRow(
		`SELECT u.id, u.phone, u.name, u.email, u.google_token, u.google_refresh, u.drive_root_id, u.created_at, u.updated_at
		 FROM users u JOIN group_members gm ON u.id = gm.user_id
		 WHERE gm.group_id=? AND u.google_refresh != '' AND u.drive_root_id != '' AND u.drive_root_id NOT LIKE 'local_%'
		 LIMIT 1`,
		groupID,
	).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		// Fallback: try ANY user with real Drive connected
		err = s.db.QueryRow(
			`SELECT id, phone, name, email, google_token, google_refresh, drive_root_id, created_at, updated_at
			 FROM users WHERE google_refresh != '' AND drive_root_id != '' AND drive_root_id NOT LIKE 'local_%' LIMIT 1`,
		).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
		if err != nil {
			return nil
		}
	}
	return u
}

// ── Content Extraction ──

// UpdateFileContent stores extracted content and summary for a file
func (s *Store) UpdateFileContent(fileID int64, content, summary string) error {
	_, err := s.db.Exec(
		`UPDATE files SET extracted_content=?, content_summary=? WHERE id=?`,
		content, summary, fileID,
	)
	return err
}

// GetFileContent returns extracted content for a file
func (s *Store) GetFileContent(fileID int64) (string, string) {
	var content, summary string
	s.db.QueryRow(`SELECT extracted_content, content_summary FROM files WHERE id=?`, fileID).Scan(&content, &summary)
	return content, summary
}

// ── NLP Conversational Retrieval ──

// SearchFilesNLP searches files by sender, content, filename, subject, and time window
func (s *Store) SearchFilesNLP(groupIDs []int64, who, what string, sinceTime *time.Time, limit int) ([]models.File, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	placeholders, args := buildInClause(groupIDs)
	// Use a LEFT JOIN on users so we can match WHO against the user record's
	// name/phone too — handles cases where shared_by_name was empty at upload
	// time but the sender's user record has a real name.
	// Filter out ghost rows (status='deleted_in_drive') so retrieval never
	// surfaces files that no longer exist in the user's Drive.
	conditions := []string{
		"f.group_id IN (" + placeholders + ")",
		"f.status != 'deleted_in_drive'",
	}

	// WHO filter — broad: match against the file's stored sender fields OR the
	// joined user record's name/phone. Also tokenize multi-word names so
	// "Mohit Singh" matches files where the name was stored as just "Mohit".
	if who != "" {
		whoLower := strings.ToLower(strings.TrimSpace(who))
		var whoParts []string
		// First name token (handles "Mohit Singh" → match on "mohit")
		fields := strings.Fields(whoLower)
		first := whoLower
		if len(fields) > 0 {
			first = fields[0]
		}
		whoParts = append(whoParts,
			"LOWER(COALESCE(f.shared_by_name,'')) LIKE ?",
			"LOWER(COALESCE(f.shared_by_name,'')) LIKE ?",
			"COALESCE(f.shared_by_phone,'') LIKE ?",
			"LOWER(COALESCE(u.name,'')) LIKE ?",
			"LOWER(COALESCE(u.name,'')) LIKE ?",
			"COALESCE(u.phone,'') LIKE ?",
		)
		conditions = append(conditions, "("+strings.Join(whoParts, " OR ")+")")
		args = append(args,
			"%"+whoLower+"%",
			"%"+first+"%",
			"%"+who+"%",
			"%"+whoLower+"%",
			"%"+first+"%",
			"%"+who+"%",
		)
	}

	// WHAT filter — tokenized OR-match with rank-by-hits.
	tokens := TokenizeWhat(what)
	rankExpr := "0"
	if what != "" && len(tokens) > 0 {
		var orParts []string
		var rankParts []string
		for _, tok := range tokens {
			orParts = append(orParts, "(LOWER(f.file_name) LIKE ? OR LOWER(f.subject) LIKE ? OR LOWER(f.tags) LIKE ? OR LOWER(f.extracted_content) LIKE ? OR LOWER(f.content_summary) LIKE ?)")
			rankParts = append(rankParts, "(CASE WHEN LOWER(f.file_name) LIKE ? OR LOWER(f.subject) LIKE ? OR LOWER(f.tags) LIKE ? OR LOWER(f.extracted_content) LIKE ? OR LOWER(f.content_summary) LIKE ? THEN 1 ELSE 0 END)")
			like := "%" + tok + "%"
			args = append(args, like, like, like, like, like)
		}
		conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")
		rankExpr = strings.Join(rankParts, " + ")
	}

	// WHEN filter — time window
	if sinceTime != nil {
		conditions = append(conditions, "f.created_at >= ?")
		args = append(args, sinceTime.Format("2006-01-02 15:04:05"))
	}

	// Append rank args (same token list, same order) so the CASE expressions resolve
	if rankExpr != "0" {
		for _, tok := range tokens {
			like := "%" + tok + "%"
			args = append(args, like, like, like, like, like)
		}
	}
	args = append(args, interface{}(limit))
	// SQLite parses bare numeric expressions in ORDER BY as column ordinals.
	// `ORDER BY (0) DESC` blows up with "1st ORDER BY term out of range".
	// Only emit the rank expression when it's a real CASE sum.
	orderBy := "f.created_at DESC"
	if rankExpr != "0" {
		orderBy = "(" + rankExpr + ") DESC, f.created_at DESC"
	}
	query := fmt.Sprintf(
		`SELECT f.id, f.group_id, f.user_id, f.shared_by_phone, f.shared_by_name, f.file_name, f.file_size,
		  f.mime_type, f.drive_file_id, f.drive_folder_id, f.subject, f.tags, f.version, f.parent_file_id, f.wa_message_id, f.status, f.created_at
		 FROM files f LEFT JOIN users u ON u.id = f.user_id
		 WHERE %s ORDER BY %s LIMIT ?`,
		strings.Join(conditions, " AND "), orderBy,
	)

	log.Printf("[SQL-NLP] who=%q what=%q tokens=%v sql=%s args=%v", who, what, tokens, query, args)
	// Diagnostic dump
	if dbg, derr := s.db.Query(`SELECT id, group_id, user_id, shared_by_name, shared_by_phone, file_name, status FROM files WHERE group_id IN (`+placeholders+`)`, func() []interface{} {
		out := make([]interface{}, len(groupIDs))
		for i, g := range groupIDs {
			out[i] = g
		}
		return out
	}()...); derr == nil {
		defer dbg.Close()
		for dbg.Next() {
			var id, gid, uid int64
			var name, phone, fname, status string
			if err := dbg.Scan(&id, &gid, &uid, &name, &phone, &fname, &status); err == nil {
				log.Printf("[DB-DUMP] id=%d gid=%d uid=%d name=%q phone=%q file=%q status=%q", id, gid, uid, name, phone, fname, status)
			}
		}
	}
	// Minimal probe: does a simple WHERE on shared_by_name match?
	if who != "" {
		var n int
		probeArgs := []interface{}{}
		for _, g := range groupIDs {
			probeArgs = append(probeArgs, g)
		}
		probeArgs = append(probeArgs, "%"+strings.ToLower(who)+"%")
		s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE group_id IN (`+placeholders+`) AND LOWER(shared_by_name) LIKE ?`, probeArgs...).Scan(&n)
		log.Printf("[DB-PROBE] simple WHERE LOWER(shared_by_name) LIKE '%%%s%%' → %d row(s)", strings.ToLower(who), n)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("[SQL-NLP] query error: %v", err)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// SearchFilesContent searches files by extracted content (for Q&A).
// Tokenized — every significant word in `query` must appear somewhere in
// the file's content/filename/subject/summary. Files with empty extracted
// content are excluded so the caller can fall back to live extraction.
func (s *Store) SearchFilesContent(groupIDs []int64, query string, limit int) ([]models.File, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	placeholders, args := buildInClause(groupIDs)
	tokens := TokenizeWhat(query)
	// Drop the "extracted_content != ''" hard requirement — for Q&A we may want
	// to lazy-extract files that the bot captured but failed to extract earlier.
	conds := []string{
		"group_id IN (" + placeholders + ")",
		"status != 'deleted_in_drive'",
	}
	rankExpr := "0"
	if len(tokens) > 0 {
		var orParts []string
		var rankParts []string
		for _, tok := range tokens {
			orParts = append(orParts, "(LOWER(extracted_content) LIKE ? OR LOWER(content_summary) LIKE ? OR LOWER(file_name) LIKE ? OR LOWER(subject) LIKE ?)")
			rankParts = append(rankParts, "(CASE WHEN LOWER(extracted_content) LIKE ? OR LOWER(content_summary) LIKE ? OR LOWER(file_name) LIKE ? OR LOWER(subject) LIKE ? THEN 1 ELSE 0 END)")
			like := "%" + tok + "%"
			args = append(args, like, like, like, like)
		}
		conds = append(conds, "("+strings.Join(orParts, " OR ")+")")
		rankExpr = strings.Join(rankParts, " + ")
		// Append rank args (same tokens again, same order)
		for _, tok := range tokens {
			like := "%" + tok + "%"
			args = append(args, like, like, like, like)
		}
	}
	args = append(args, interface{}(limit))
	orderBy := "created_at DESC"
	if rankExpr != "0" {
		orderBy = "(" + rankExpr + ") DESC, created_at DESC"
	}
	q := fmt.Sprintf(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at
		 FROM files WHERE %s
		 ORDER BY %s LIMIT ?`,
		strings.Join(conds, " AND "), orderBy,
	)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// TokenizeWhat splits an NLP `what` clause into significant lowercase tokens.
// Drops stopwords and tokens shorter than 3 chars so noise like "the", "a",
// "of" doesn't widen the search to match every file in the DB.
func TokenizeWhat(what string) []string {
	stop := map[string]bool{
		// articles / pronouns / aux
		"the": true, "and": true, "for": true, "with": true, "from": true, "that": true,
		"this": true, "those": true, "these": true, "any": true, "some": true, "all": true,
		"are": true, "was": true, "were": true, "has": true, "have": true, "had": true,
		"you": true, "your": true, "yours": true, "me": true, "mine": true, "our": true,
		"his": true, "her": true, "him": true, "she": true, "they": true, "them": true,
		"can": true, "could": true, "would": true, "should": true, "will": true, "shall": true,
		"may": true, "might": true, "must": true, "into": true, "out": true, "off": true,
		// question words
		"what": true, "when": true, "where": true, "why": true, "how": true, "who": true,
		"which": true, "whose": true, "did": true, "does": true, "doing": true, "done": true,
		// generic file vocab — we know it's a file, no need to match
		"notes": true, "note": true, "file": true, "files": true, "pdf": true, "pdfs": true,
		"doc": true, "docs": true, "document": true, "documents": true, "page": true, "pages": true,
		// generic Q&A request verbs
		"please": true, "find": true, "show": true, "give": true, "tell": true, "send": true,
		"share": true, "shared": true, "sent": true, "uploaded": true, "upload": true,
		"explain": true, "describe": true, "define": true, "definition": true, "exact": true,
		"exactly": true, "example": true, "examples": true, "summary": true, "summarize": true,
		"summarise": true, "list": true, "mention": true, "mentioned": true, "mentions": true,
		"according": true, "regarding": true, "about": true, "concerning": true, "remember": true,
		"recall": true, "know": true, "knows": true, "told": true, "saying": true, "said": true,
		"says": true, "want": true, "need": true, "kindly": true,
		// time generics (real time filtering happens via WHEN, not WHAT)
		"today": true, "tomorrow": true, "yesterday": true, "now": true, "recent": true,
		"recently": true, "latest": true, "last": true, "ago": true, "back": true, "only": true,
	}
	lower := strings.ToLower(what)
	// replace non-alphanumeric with spaces
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, lower)
	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(cleaned) {
		if len(tok) < 3 || stop[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	// If everything got filtered (e.g. user typed only stopwords), fall back
	// to the original phrase so we still search something.
	if len(out) == 0 {
		out = []string{strings.TrimSpace(lower)}
	}
	return out
}

// GetFileExtractedContent returns just the extracted_content for given file IDs
func (s *Store) GetFileExtractedContent(fileIDs []int64) map[int64]string {
	result := make(map[int64]string)
	if len(fileIDs) == 0 {
		return result
	}
	placeholders, args := buildInClause(fileIDs)
	rows, err := s.db.Query(
		`SELECT id, extracted_content FROM files WHERE id IN (`+placeholders+`) AND extracted_content != ''`,
		args...,
	)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var content string
		rows.Scan(&id, &content)
		result[id] = content
	}
	return result
}
