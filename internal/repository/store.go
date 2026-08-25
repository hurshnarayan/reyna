package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/relevance"
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
		`ALTER TABLE group_settings ADD COLUMN hidden INTEGER DEFAULT 0`,
	}
	for _, m := range migrations {
		s.db.Exec(m) // ignore errors if columns already exist
	}

	if err := s.migrateAttribution(); err != nil {
		return err
	}
	if err := s.releaseFalselyUnreadable(); err != nil {
		return err
	}
	if err := s.unnameTheDevice(); err != nil {
		return err
	}

	return nil
}

// unnameTheDevice strips the attributions that named the phone as a person.
//
// Everything captured on-device uploads under the identity "device", a user
// record was upserted for it, and the sender lookup found a name that was not
// a phone number and concluded it was human. Those rows were then stamped
// AttrBaileys at confidence 1.0, which claims the sender was stated by the
// WhatsApp protocol as a fact. Answers duly said "(by device)".
//
// The confidence threshold exists precisely so a file whose sender is unknown
// says so. These rows asserted the opposite with full certainty, so they are
// reset to unattributed: no name, no method, no confidence. Nothing is lost
// that was ever real, and the repair screen can still ask the user who shared
// something.
func (s *Store) unnameTheDevice() error {
	res, err := s.db.Exec(`
		UPDATE files
		   SET shared_by_name = '',
		       attribution_method = ?,
		       attribution_confidence = 0
		 WHERE LOWER(TRIM(COALESCE(shared_by_name,''))) = 'device'`,
		model.AttrNone)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[MIGRATE] cleared %d attribution(s) that named the device as the sender", n)
	}
	// The user record itself, so the lookup cannot recreate the name.
	_, err = s.db.Exec(`UPDATE users SET name='' WHERE LOWER(TRIM(COALESCE(name,''))) = 'device'`)
	return err
}

// releaseFalselyUnreadable gives back the files that a failed reading retired.
//
// UnreadableSentinel is permanent on purpose, so a document that genuinely
// holds no text stops costing a model call on every question. Until now it was
// also written whenever the reading itself failed, and on a free allowance of
// twenty calls a day the commonest failure by far is the allowance running
// out. Those files were retired for good without ever having been opened.
//
// The mark cannot be told apart after the fact, so every retired file of a
// type that could yield text is offered one more chance. A file that really
// has nothing in it is read once more and marked again, which costs a single
// call; a file that was only unlucky comes back. Formats read locally by
// internal/docs cost nothing at all to retry.
//
// This runs once. A marker row records that it has, so restarting the server
// does not put the genuinely empty files back into the queue every time.
func (s *Store) releaseFalselyUnreadable() error {
	s.db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT)`)

	var done string
	s.db.QueryRow(`SELECT value FROM meta WHERE key='released_false_unreadable'`).Scan(&done)
	if done != "" {
		return nil
	}

	res, err := s.db.Exec(`
		UPDATE files SET extracted_content='', content_summary=''
		WHERE extracted_content = ?
		  AND (
		    LOWER(file_name) LIKE '%.pdf'  OR LOWER(file_name) LIKE '%.doc'  OR LOWER(file_name) LIKE '%.docx' OR
		    LOWER(file_name) LIKE '%.ppt'  OR LOWER(file_name) LIKE '%.pptx' OR LOWER(file_name) LIKE '%.odp'  OR
		    LOWER(file_name) LIKE '%.xls'  OR LOWER(file_name) LIKE '%.xlsx' OR LOWER(file_name) LIKE '%.ods'  OR
		    LOWER(file_name) LIKE '%.odt'  OR LOWER(file_name) LIKE '%.rtf'  OR LOWER(file_name) LIKE '%.csv'  OR
		    LOWER(file_name) LIKE '%.txt'  OR LOWER(file_name) LIKE '%.md'   OR LOWER(file_name) LIKE '%.epub'
		  )`, UnreadableSentinel)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[MIGRATE] released %d file(s) wrongly marked unreadable", n)
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('released_false_unreadable','1')`)
	return err
}

// migrateAttribution adds the schema that lets a file's sender and share time
// be recorded separately from the row's insert time, and be qualified by how
// confident we are.
//
// Why this exists: created_at is when the server inserted the row. Under the
// Baileys bot that is within seconds of when the message was sent, so the two
// were used interchangeably and created_at is what every answer is timestamped
// from. Once capture moves to the user's phone that stops being true — a file
// lands on disk when someone taps download, which can be days after it was
// posted — and Reyna would state the wrong date with no way for the user to
// tell. posted_at holds the real message time; created_at keeps meaning what it
// always meant.
//
// Sender attribution changes in the same way. Baileys supplies the sender as a
// fact. On-device it is a join between a file and a notification or an export
// line, which can fail or be ambiguous, so every attribution now carries a
// method and a confidence and callers can refuse to name a person below a
// threshold.
func (s *Store) migrateAttribution() error {
	stmts := []string{
		// When the message was actually sent, as opposed to when we inserted
		// the row. Nullable: NULL means "we only know when we saw it".
		`ALTER TABLE files ADD COLUMN posted_at DATETIME`,
		// How the sender was determined, and how much to trust it.
		//   baileys        — from the WhatsApp Web protocol. Authoritative.
		//   export         — matched to a line in a chat export. Authoritative.
		//   self_sent      — file was in WhatsApp's /Sent/ folder.
		//   notification   — matched to a notification we observed.
		//   date_unique    — only one candidate message that day.
		//   time_window    — nearest message in time. Ambiguous.
		//   user           — the user told us.
		//   none           — unattributed.
		`ALTER TABLE files ADD COLUMN attribution_method TEXT DEFAULT ''`,
		`ALTER TABLE files ADD COLUMN attribution_confidence REAL DEFAULT 0`,

		// Rows that predate this migration came from Baileys, which reports the
		// sender authoritatively, so they are marked as such rather than left
		// looking unattributed. Rows with no sender stay at zero.
		`UPDATE files SET posted_at = created_at WHERE posted_at IS NULL`,
		`UPDATE files
		    SET attribution_method = 'baileys', attribution_confidence = 1.0
		  WHERE attribution_method = ''
		    AND COALESCE(shared_by_name,'') || COALESCE(shared_by_phone,'') != ''`,

		`CREATE INDEX IF NOT EXISTS idx_files_posted ON files(posted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_files_attribution ON files(attribution_confidence)`,

		// A person seen in a chat. WhatsApp does not hand out a stable
		// identifier for a group member, so identity is resolved from whatever
		// keys we do get: a notification Person key, a phone number, or just a
		// display name from an export. merged_into_id lets two records that
		// turn out to be the same person be joined without losing either.
		`CREATE TABLE IF NOT EXISTS people (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sender_key TEXT DEFAULT '',
			display_name TEXT DEFAULT '',
			phone TEXT DEFAULT '',
			merged_into_id INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_people_key ON people(sender_key)`,
		`CREATE INDEX IF NOT EXISTS idx_people_phone ON people(phone)`,
		`CREATE INDEX IF NOT EXISTS idx_people_name ON people(display_name)`,

		// A message we know about, independent of whether a file ever turned up
		// for it. Stored unconditionally: an event with no file is what lets a
		// file that arrives hours later still be attributed, and the join runs
		// in both directions for exactly that reason.
		//
		// group_id may be 0 when an export is imported before the chat has been
		// matched to a known group.
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER DEFAULT 0,
			chat_key TEXT DEFAULT '',
			chat_name TEXT DEFAULT '',
			is_group INTEGER DEFAULT 1,
			person_id INTEGER DEFAULT 0,
			sender_key TEXT DEFAULT '',
			sender_display TEXT DEFAULT '',
			posted_at DATETIME NOT NULL,
			raw_text TEXT DEFAULT '',
			attachment_name TEXT DEFAULT '',
			has_attachment INTEGER DEFAULT 0,
			source TEXT DEFAULT '',
			source_ref TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_posted ON events(posted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_group ON events(group_id, posted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_attachment ON events(attachment_name)`,
		`CREATE INDEX IF NOT EXISTS idx_events_sender ON events(sender_display)`,
		// An export re-imported over the same range must not double up. Two
		// messages genuinely identical in chat, time, sender and text are
		// indistinguishable and collapsing them is correct.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_events_dedup
			ON events(chat_key, posted_at, sender_display, attachment_name, raw_text)`,

		// Candidate file-to-event matches. Every candidate is kept rather than
		// only the winner, with is_active marking the current best, so a later
		// export can promote a better match without the earlier reasoning being
		// lost — and so a wrong guess can be explained after the fact.
		`CREATE TABLE IF NOT EXISTS links (
			file_id INTEGER NOT NULL,
			event_id INTEGER NOT NULL,
			method TEXT DEFAULT '',
			confidence REAL DEFAULT 0,
			is_active INTEGER DEFAULT 0,
			linked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (file_id, event_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_links_file ON links(file_id, is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_links_event ON links(event_id)`,
	}
	for _, stmt := range stmts {
		// ALTER TABLE ADD COLUMN fails when the column is already there, which
		// is the normal path on every start after the first. CREATE ... IF NOT
		// EXISTS and the UPDATEs are idempotent.
		if _, err := s.db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
			return fmt.Errorf("attribution migration %.60q: %w", stmt, err)
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

// ── User Operations ──

func (s *Store) UpsertUser(phone, name string) (*model.User, error) {
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

func (s *Store) GetUserByPhone(phone string) (*model.User, error) {
	u := &model.User{}
	err := s.db.QueryRow(
		`SELECT id, phone, name, email, google_token, google_refresh, drive_root_id, created_at, updated_at FROM users WHERE phone=?`,
		phone,
	).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) GetUserByID(id int64) (*model.User, error) {
	u := &model.User{}
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

func (s *Store) UpsertGroup(waID, name string, createdBy int64) (*model.Group, error) {
	_, err := s.db.Exec(
		`INSERT INTO groups_ (wa_id, name, created_by, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(wa_id) DO UPDATE SET
		   name=CASE WHEN excluded.name != '' AND excluded.name != 'WhatsApp Group' THEN excluded.name ELSE groups_.name END,
		   updated_at=CURRENT_TIMESTAMP`,
		waID, name, createdBy,
	)
	if err != nil {
		return nil, err
	}
	return s.GetGroupByWAID(waID)
}

func (s *Store) GetGroupByWAID(waID string) (*model.Group, error) {
	g := &model.Group{}
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
	// Only update the name if we actually have one — never overwrite a real name with empty
	if name == "" || name == "WhatsApp Group" {
		name = "WhatsApp Group"
	}
	_, err := s.db.Exec(
		`INSERT INTO groups_ (wa_id, name, member_count, created_by, updated_at) VALUES (?, ?, ?, NULL, CURRENT_TIMESTAMP)
		 ON CONFLICT(wa_id) DO UPDATE SET
		   name=CASE WHEN excluded.name != '' AND excluded.name != 'WhatsApp Group' THEN excluded.name ELSE groups_.name END,
		   member_count=CASE WHEN excluded.member_count > 0 THEN excluded.member_count ELSE groups_.member_count END,
		   updated_at=CURRENT_TIMESTAMP`,
		waID, name, memberCount,
	)
	return err
}

func (s *Store) UpdateGroupName(groupID int64, name string) error {
	_, err := s.db.Exec(`UPDATE groups_ SET name=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, groupID)
	return err
}

func (s *Store) GetAllGroups() ([]model.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, wa_id, name, member_count, created_by, created_at, updated_at FROM groups_ ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.Group
	for rows.Next() {
		var g model.Group
		rows.Scan(&g.ID, &g.WAID, &g.Name, &g.MemberCount, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) GetUserGroups(userID int64) ([]model.Group, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.wa_id, g.name, g.member_count, g.created_by, g.created_at, g.updated_at
		 FROM groups_ g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_id=? ORDER BY g.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.Group
	for rows.Next() {
		var g model.Group
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

	// Deliberately NOT linking every existing group. An earlier revision added
	// a third pass selecting all rows from groups_ so that "every user sees all
	// data" during a demo. That made every registered user a member of every
	// group in the database, exposing every other user's files. Membership must
	// come from evidence that this phone actually participated: files they
	// shared (1), or an existing membership row for the same phone (2).

	// Now insert memberships (all rows closed, no lock contention)
	for _, gid := range groupIDs {
		s.AddGroupMember(gid, userID, phone, "member")
	}
}

// ── File Operations ──

func (s *Store) AddFile(f *model.File) (*model.File, error) {
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
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, content_hash,
		  posted_at, attribution_method, attribution_confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.GroupID, f.UserID, f.SharedByPhone, f.SharedByName, f.FileName, f.FileSize,
		f.MimeType, f.DriveFileID, f.DriveFolderID, f.Subject, f.Tags, f.Version, f.ParentFileID, f.WAMessageID, f.Status, f.ContentHash,
		postedAtArg(f), f.AttributionMethod, f.AttributionConfidence,
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
func (s *Store) GetStagedFiles(groupID int64) ([]model.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func (s *Store) FindFiles(groupID int64, query string) ([]model.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func (s *Store) GetGroupFiles(groupID int64, limit int) ([]model.File, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE group_id=? ORDER BY created_at DESC LIMIT ?`,
		groupID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetUserFiles(userID int64, limit int) ([]model.File, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE user_id=? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) GetFileVersions(fileID int64) ([]model.FileVersion, error) {
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
	var versions []model.FileVersion
	for rows.Next() {
		var v model.FileVersion
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

func (s *Store) GetNewFilesSince(groupID int64, since time.Time) ([]model.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE group_id=? AND COALESCE(posted_at, created_at) > ?
		 ORDER BY COALESCE(posted_at, created_at) DESC`,
		groupID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// ── Dashboard Stats ──

func (s *Store) GetDashboardStats(userID int64) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{
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
		// Only count files we can actually attribute. Grouping by phone alone
		// swept every unattributed file into a single blank-named entry that
		// then sat at the top of the leaderboard as a phantom contributor.
		fmt.Sprintf(
			`SELECT shared_by_name, shared_by_phone, COUNT(*) as cnt FROM files
			 WHERE group_id IN (`+placeholders+`)
			   AND COALESCE(shared_by_name,'') != ''
			   AND COALESCE(attribution_confidence,0) >= %v
			 GROUP BY shared_by_phone, shared_by_name ORDER BY cnt DESC LIMIT 5`,
			model.AttributionMinNamed),
		args...,
	)
	if contribRows != nil {
		for contribRows.Next() {
			var c model.Contributor
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
func (s *Store) GetGroupsFiles(groupIDs []int64, limit int) ([]model.File, error) {
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
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func (s *Store) GetActivityLog(groupID int64, limit int) ([]model.ActivityLog, error) {
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
	var logs []model.ActivityLog
	for rows.Next() {
		var l model.ActivityLog
		rows.Scan(&l.ID, &l.GroupID, &l.UserID, &l.Action, &l.Command, &l.Result, &l.CreatedAt)
		logs = append(logs, l)
	}
	return logs, nil
}

// ── File Delete ──

func (s *Store) GetFileByID(fileID int64) (*model.File, error) {
	f := &model.File{}
	var posted sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE id=?`, fileID,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
		&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
		&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt,
		&posted, &f.AttributionMethod, &f.AttributionConfidence)
	if err != nil {
		return nil, err
	}
	applyPostedAt(f, posted)
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
func (s *Store) FindFilesStrict(groupIDs []int64, query string, limit int) ([]model.File, error) {
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
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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
func (s *Store) FileExistsInGroup(groupID int64, fileName string) (*model.File, bool) {
	f := &model.File{}
	var posted sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE group_id=? AND file_name=? AND status='committed' ORDER BY version DESC LIMIT 1`,
		groupID, fileName,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
		&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
		&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt,
		&posted, &f.AttributionMethod, &f.AttributionConfidence)
	if err != nil {
		return nil, false
	}
	applyPostedAt(f, posted)
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

func (s *Store) GetGroupSettings(groupID int64) *model.GroupSettings {
	gs := &model.GroupSettings{GroupID: groupID, Enabled: false, Hidden: false, TrackingMode: "auto", AutoCommitHours: 24, ReactionEmoji: "📌"}
	err := s.db.QueryRow(
		`SELECT group_id, enabled, tracking_mode, auto_commit_hours, reaction_emoji, COALESCE(hidden, 0) FROM group_settings WHERE group_id=?`,
		groupID,
	).Scan(&gs.GroupID, &gs.Enabled, &gs.TrackingMode, &gs.AutoCommitHours, &gs.ReactionEmoji, &gs.Hidden)
	if err != nil {
		return gs // return defaults
	}
	return gs
}

func (s *Store) UpsertGroupSettings(gs *model.GroupSettings) error {
	_, err := s.db.Exec(
		`INSERT INTO group_settings (group_id, enabled, hidden, tracking_mode, auto_commit_hours, reaction_emoji)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET enabled=excluded.enabled, hidden=excluded.hidden,
		   tracking_mode=excluded.tracking_mode,
		   auto_commit_hours=excluded.auto_commit_hours, reaction_emoji=excluded.reaction_emoji`,
		gs.GroupID, gs.Enabled, gs.Hidden, gs.TrackingMode, gs.AutoCommitHours, gs.ReactionEmoji,
	)
	return err
}

func (s *Store) GroupSettingsExist(groupID int64) bool {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM group_settings WHERE group_id=?`, groupID).Scan(&count)
	return count > 0
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

func (s *Store) GetFilesWithSorting(groupIDs []int64, sortBy, sortOrder string, limit int) ([]model.File, error) {
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
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func (s *Store) GetStagedFilesOlderThan(hours int) ([]model.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func (s *Store) GetStagedFilesByGroupID(groupID int64) ([]model.File, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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

func scanFiles(rows *sql.Rows) ([]model.File, error) {
	var files []model.File
	for rows.Next() {
		var f model.File
		var posted sql.NullTime
		err := rows.Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName,
			&f.FileName, &f.FileSize, &f.MimeType, &f.DriveFileID, &f.DriveFolderID,
			&f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt,
			&posted, &f.AttributionMethod, &f.AttributionConfidence)
		if err != nil {
			// Historically this skipped silently, which turns a column or type
			// mismatch into "the user has no files" with nothing in the log to
			// explain it. Still skip the row, but say so.
			log.Printf("[SCAN] skipping file row: %v", err)
			continue
		}
		applyPostedAt(&f, posted)
		files = append(files, f)
	}
	return files, nil
}

// applyPostedAt resolves the effective share time. posted_at is selected raw
// rather than wrapped in COALESCE because go-sqlite3 only converts a value to
// time.Time when the *declared column type* is a date type; the result of an
// expression has no declared type, comes back as a string, and fails to scan.
// So the fallback to created_at happens here instead of in SQL.
func applyPostedAt(f *model.File, posted sql.NullTime) {
	if posted.Valid && !posted.Time.IsZero() {
		f.PostedAt = posted.Time
		return
	}
	f.PostedAt = f.CreatedAt
}

// postedAtArg renders File.PostedAt for insertion, writing NULL rather than the
// zero time when it is unknown so reads fall back to created_at.
func postedAtArg(f *model.File) interface{} {
	if f.PostedAt.IsZero() {
		return nil
	}
	return f.PostedAt
}

// UpdateFileSubject updates the subject/folder after async classification completes
func (s *Store) UpdateFileSubject(fileID int64, subject string) error {
	_, err := s.db.Exec(`UPDATE files SET subject=? WHERE id=?`, subject, fileID)
	return err
}

// UpdateFileDriveID updates the drive file ID after uploading to Google Drive
func (s *Store) UpdateFileDriveID(fileID int64, driveFileID, driveFolderID string) error {
	_, err := s.db.Exec(
		`UPDATE files SET drive_file_id=?, drive_folder_id=? WHERE id=?`,
		driveFileID, driveFolderID, fileID,
	)
	return err
}

// DistinctSubjectsForGroup returns every distinct subject (folder name) that
// has ever been used by files in this group. Used as the local-truth source
// for the classifier's "existing folders" list, so a file uploaded right
// after another similar one sees the just-created folder even if Drive's
// API hasn't surfaced it yet.
func (s *Store) DistinctSubjectsForGroup(groupID int64) []string {
	if groupID == 0 {
		return nil
	}
	rows, err := s.db.Query(
		`SELECT DISTINCT subject FROM files WHERE group_id=? AND subject != '' AND status != 'deleted_in_drive'`,
		groupID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// FindFileByHash returns the most recent file in the group whose content hash
// matches. Used for byte-identical duplicate detection on bot upload.
func (s *Store) FindFileByHash(groupID int64, hash string) *model.File {
	if hash == "" {
		return nil
	}
	f := &model.File{}
	var posted sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
		 FROM files WHERE group_id=? AND content_hash=? AND status != 'deleted_in_drive'
		 ORDER BY created_at DESC LIMIT 1`,
		groupID, hash,
	).Scan(&f.ID, &f.GroupID, &f.UserID, &f.SharedByPhone, &f.SharedByName, &f.FileName, &f.FileSize,
		&f.MimeType, &f.DriveFileID, &f.DriveFolderID, &f.Subject, &f.Tags, &f.Version, &f.ParentFileID, &f.WAMessageID, &f.Status, &f.CreatedAt,
		&posted, &f.AttributionMethod, &f.AttributionConfidence)
	if err != nil {
		return nil
	}
	applyPostedAt(f, posted)
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
func (s *Store) EnrichDriveMatches(matches []model.DriveMatch) []model.DriveMatch {
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
func (s *Store) FindDriveConnectedUser(groupID int64) *model.User {
	u := &model.User{}
	err := s.db.QueryRow(
		`SELECT u.id, u.phone, u.name, u.email, u.google_token, u.google_refresh, u.drive_root_id, u.created_at, u.updated_at
		 FROM users u JOIN group_members gm ON u.id = gm.user_id
		 WHERE gm.group_id=? AND u.google_refresh != '' AND u.drive_root_id != '' AND u.drive_root_id NOT LIKE 'local_%'
		 LIMIT 1`,
		groupID,
	).Scan(&u.ID, &u.Phone, &u.Name, &u.Email, &u.GoogleToken, &u.GoogleRefresh, &u.DriveRootID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		// No fallback. An earlier revision fell back to "any user in the database
		// with Drive connected", which uploaded one group's files into the Drive
		// of someone who was never in that group. Callers must handle nil by
		// telling the user nobody in this group has connected Drive yet.
		return nil
	}
	return u
}

// ── Content Extraction ──

// UnreadableSentinel marks a file whose bytes could not be turned into text.
//
// Stored rather than left empty so the file stops being offered for reading on
// every pass. An empty extracted_content means "not read yet"; this means
// "tried, and there is nothing here".
const UnreadableSentinel = "[unreadable]"


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
func (s *Store) SearchFilesNLP(groupIDs []int64, who, what string, sinceTime *time.Time, limit int) ([]model.File, error) {
	ranked, err := s.SearchFilesNLPScored(groupIDs, who, what, sinceTime, limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.File)
	}
	return out, nil
}

// SearchFilesNLPScored is SearchFilesNLP with the ranking kept.
//
// The scores travel with the files because the caller has to decide whether
// one match was clear enough to answer from, or whether two were close enough
// that the honest move is to ask which was meant.
func (s *Store) SearchFilesNLPScored(groupIDs []int64, who, what string, sinceTime *time.Time, limit int) ([]ScoredFile, error) {
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

	// WHO — match against the file's stored sender fields or the joined user
	// record's name/phone, tokenizing multi-word names so "Mohit Singh" matches
	// a file stored as just "Mohit".
	//
	// This used to be a hard AND, which was correct only while Baileys supplied
	// the sender as a fact. Once attribution can fail, an unattributed file that
	// really was Mohit's could never match "what did Mohit share", and the user
	// saw an empty result rather than an uncertain one — indistinguishable from
	// data loss.
	//
	// So the filter tests knowledge rather than absence. A file is excluded only
	// when we know enough to rule it out: its sender is attributed confidently
	// and is somebody else. A file we cannot attribute is still a candidate, and
	// ranks below the confident matches. Callers must present those honestly —
	// see model.File.SenderKnown.
	whoRankExpr := "0"
	var whoRankArgs []interface{}
	if who != "" {
		whoLower := strings.ToLower(strings.TrimSpace(who))
		// First name token (handles "Mohit Singh" → match on "mohit")
		first := whoLower
		if fields := strings.Fields(whoLower); len(fields) > 0 {
			first = fields[0]
		}
		// The joined users row is the account that *uploaded* the file, which is
		// not the same thing as the person who shared it. Under the bot they
		// coincide, because a user record is upserted per sender. On-device they
		// never will: the phone's owner uploads everything, so matching WHO
		// against u.name would match every file in the database to the owner's
		// name. So the user record is only consulted when the file carries no
		// sender of its own, which is the gap it was added to cover.
		whoMatch := "(" + strings.Join([]string{
			"LOWER(COALESCE(f.shared_by_name,'')) LIKE ?",
			"LOWER(COALESCE(f.shared_by_name,'')) LIKE ?",
			"COALESCE(f.shared_by_phone,'') LIKE ?",
			`(COALESCE(f.shared_by_name,'') = '' AND COALESCE(f.shared_by_phone,'') = '' AND (
				LOWER(COALESCE(u.name,'')) LIKE ? OR LOWER(COALESCE(u.name,'')) LIKE ? OR COALESCE(u.phone,'') LIKE ?))`,
		}, " OR ") + ")"
		whoMatchArgs := []interface{}{
			"%" + whoLower + "%",
			"%" + first + "%",
			"%" + who + "%",
			"%" + whoLower + "%",
			"%" + first + "%",
			"%" + who + "%",
		}

		// Attribution too weak to name anyone, so too weak to exclude anyone.
		unattributed := fmt.Sprintf(
			"(COALESCE(f.attribution_confidence,0) < %v OR COALESCE(f.shared_by_name,'') || COALESCE(f.shared_by_phone,'') = '')",
			model.AttributionMinNamed)

		conditions = append(conditions, "("+whoMatch+" OR "+unattributed+")")
		args = append(args, whoMatchArgs...)

		// Confident matches outrank files that merely could not be ruled out.
		whoRankExpr = "(CASE WHEN " + whoMatch + " THEN 1 ELSE 0 END)"
		whoRankArgs = whoMatchArgs
	}

	// WHAT filter — tokenized OR-match with weighted rank-by-hits.
	tokens := TokenizeWhat(what)
	rankExpr := "0"
	var rankArgs []interface{}
	if what != "" && len(tokens) > 0 {
		cleanPhrase := strings.TrimSpace(strings.ToLower(what))
		var orParts []string
		var rankParts []string

		// 1. Exact phrase match boost in filename or subject (e.g. "c programming", "reyna script")
		if strings.Contains(cleanPhrase, " ") {
			phraseLike := "%" + cleanPhrase + "%"
			orParts = append(orParts, "LOWER(f.file_name) LIKE ?", "LOWER(f.subject) LIKE ?")
			args = append(args, phraseLike, phraseLike)
			rankParts = append(rankParts, "(CASE WHEN LOWER(f.file_name) LIKE ? THEN 60 WHEN LOWER(f.subject) LIKE ? THEN 40 ELSE 0 END)")
			rankArgs = append(rankArgs, phraseLike, phraseLike)
		}

		// 2. Individual token matches with heavy weighting on filename and subject
		for _, tok := range tokens {
			like := "%" + tok + "%"
			if len(tok) <= 2 {
				// Short token / acronym (e.g. "c", "os", "ai", "db"): match only filename, subject, tags
				orParts = append(orParts, "(LOWER(f.file_name) LIKE ? OR LOWER(f.subject) LIKE ? OR LOWER(f.tags) LIKE ?)")
				args = append(args, like, like, like)
				rankParts = append(rankParts, "(CASE WHEN LOWER(f.file_name) LIKE ? THEN 25 WHEN LOWER(f.subject) LIKE ? THEN 15 ELSE 0 END)")
				rankArgs = append(rankArgs, like, like)
			} else {
				orParts = append(orParts, "(LOWER(f.file_name) LIKE ? OR LOWER(f.subject) LIKE ? OR LOWER(f.tags) LIKE ? OR LOWER(f.extracted_content) LIKE ? OR LOWER(f.content_summary) LIKE ?)")
				args = append(args, like, like, like, like, like)
				rankParts = append(rankParts, "(CASE WHEN LOWER(f.file_name) LIKE ? THEN 25 WHEN LOWER(f.subject) LIKE ? THEN 15 WHEN LOWER(f.extracted_content) LIKE ? THEN 2 ELSE 0 END)")
				rankArgs = append(rankArgs, like, like, like)
			}
		}

		// 3. Multi-token AND boost in filename (when all tokens appear together in filename)
		if len(tokens) >= 2 {
			var andParts []string
			for _, tok := range tokens {
				andParts = append(andParts, "LOWER(f.file_name) LIKE ?")
				rankArgs = append(rankArgs, "%"+tok+"%")
			}
			rankParts = append(rankParts, "(CASE WHEN ("+strings.Join(andParts, " AND ")+") THEN 40 ELSE 0 END)")
		}

		conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")
		rankExpr = strings.Join(rankParts, " + ")
	}

	// WHEN — time window against when the message was sent, not when we
	// inserted the row. Those differ by days once capture is on-device, so
	// filtering on created_at would silently drop files from "last week".
	if sinceTime != nil {
		conditions = append(conditions, "COALESCE(f.posted_at, f.created_at) >= ?")
		args = append(args, sinceTime.Format("2006-01-02 15:04:05"))
	}

	// ORDER BY args follow the WHERE args, in the order the expressions appear
	// in the statement: WHO rank first, then content rank.
	args = append(args, whoRankArgs...)
	if rankExpr != "0" {
		args = append(args, rankArgs...)
	}
	// Over-fetch, because the real filter runs after this in Go.
	//
	// The SQL rank cannot tell "module 1" from "Module4_part1", so cutting at
	// `limit` here throws away the file the person asked for before anything
	// has looked at it properly. Fetch a wide band and let rankByRelevance
	// choose, bounded so a two word question cannot pull the whole library.
	fetchLimit := limit * 8
	if fetchLimit > 300 {
		fetchLimit = 300
	}
	if fetchLimit < limit {
		fetchLimit = limit
	}
	args = append(args, interface{}(fetchLimit))

	// SQLite parses bare numeric expressions in ORDER BY as column ordinals, so
	// `ORDER BY (0) DESC` fails with "1st ORDER BY term out of range". Only emit
	// a rank term when it is a real CASE expression.
	orderParts := []string{}
	if whoRankExpr != "0" {
		orderParts = append(orderParts, whoRankExpr+" DESC")
	}
	if rankExpr != "0" {
		orderParts = append(orderParts, "("+rankExpr+") DESC")
	}
	orderParts = append(orderParts, "COALESCE(f.posted_at, f.created_at) DESC")
	orderBy := strings.Join(orderParts, ", ")
	query := fmt.Sprintf(
		`SELECT f.id, f.group_id, f.user_id, f.shared_by_phone, f.shared_by_name, f.file_name, f.file_size,
		  f.mime_type, f.drive_file_id, f.drive_folder_id, f.subject, f.tags, f.version, f.parent_file_id, f.wa_message_id, f.status, f.created_at, f.posted_at, COALESCE(f.attribution_method,''), COALESCE(f.attribution_confidence,0)
		 FROM files f LEFT JOIN users u ON u.id = f.user_id
		 WHERE %s ORDER BY %s LIMIT ?`,
		strings.Join(conditions, " AND "), orderBy,
	)

	// Log shape only, never contents. An earlier revision dumped every row in
	// every searched group (id, sender name, phone, filename) on every single
	// query, which put user data in the logs and cost a full table scan per
	// search.
	log.Printf("[SQL-NLP] who=%t what_tokens=%d since=%t groups=%d", who != "", len(tokens), sinceTime != nil, len(groupIDs))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("[SQL-NLP] query error: %v", err)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates, err := scanFiles(rows)
	if err != nil {
		return nil, err
	}

	// The SQL above is a recall net, not the answer.
	//
	// Every one of its tests is LIKE '%token%', which has no notion of a word:
	// it matches "ode" inside "diodes" and "1" inside "part1", and because the
	// token tests are joined with OR, a file needed only the word "module" to
	// qualify for a question about module 1 ODE. So the query casts wide and
	// deliberately over-fetches, and the decision about what is actually
	// relevant is made here, on whole words, where it can be tested.
	ranked := rankByRelevance(candidates, s.snippets(candidates), tokens)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

// ScoredFile is a file together with how well it answered the query.
//
// The scores travel with the file because the caller has to decide whether
// the match was clear enough to answer from, or close enough to a rival that
// the honest move is to ask which one was meant.
type ScoredFile struct {
	model.File
	Score    float64
	Coverage float64
	Adjacent int
}

// snippets fetches each candidate's stored text, for relevance scoring.
//
// The whole of it, not the opening. A term the question is about can sit
// anywhere in a document, and an earlier revision that read only the first few
// thousand characters made a lecture naming Bernoulli's equation on its
// fifteenth slide unfindable by anything but its filename, which did not
// mention it either.
//
// Capped per file rather than per query, at a size no real document reaches,
// so one pathological row cannot pull the whole table into memory.
func (s *Store) snippets(files []model.File) map[int64]string {
	out := make(map[int64]string, len(files))
	if len(files) == 0 {
		return out
	}
	ids := make([]int64, 0, len(files))
	for _, f := range files {
		ids = append(ids, f.ID)
	}
	placeholders, args := buildInClause(ids)
	rows, err := s.db.Query(
		`SELECT id, substr(COALESCE(extracted_content,''),1,200000) FROM files WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var snip string
		if rows.Scan(&id, &snip) == nil {
			out[id] = snip
		}
	}
	return out
}

// rankByRelevance keeps the files that actually answer the query and orders
// them best first.
//
// The cut is relative to the best match rather than an absolute threshold.
// When some file accounts for every word of the question, files accounting
// for fewer are not near misses, they are other documents, and listing them
// under an answer is what made a correct answer look like a guess. When
// nothing accounts for the whole question, the best available is still the
// best there is and must survive, or a library that plainly contains
// something answers that it has never seen it.
func rankByRelevance(files []model.File, snippets map[int64]string, tokens []string) []ScoredFile {
	if len(files) == 0 {
		return nil
	}
	if len(tokens) == 0 {
		out := make([]ScoredFile, 0, len(files))
		for _, f := range files {
			out = append(out, ScoredFile{File: f})
		}
		return out
	}

	scored := make([]ScoredFile, 0, len(files))
	best := 0.0
	for _, f := range files {
		r := relevance.Scored(f.FileName, f.Subject+" "+f.Tags, snippets[f.ID], tokens)
		if r.Matched == 0 {
			continue
		}
		if r.Coverage > best {
			best = r.Coverage
		}
		scored = append(scored, ScoredFile{File: f, Score: r.Score, Coverage: r.Coverage, Adjacent: r.Adjacent})
	}

	floor := relevance.Floor(best)
	kept := scored[:0]
	for _, sf := range scored {
		if sf.Coverage >= floor {
			kept = append(kept, sf)
		}
	}

	// Stable, so files that scored the same keep the SQL order, which already
	// put confident sender matches and recent files first.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	log.Printf("[RANK] candidates=%d kept=%d best_coverage=%.2f floor=%.2f", len(files), len(kept), best, floor)
	return kept
}

// SearchFilesContent searches files by extracted content (for Q&A).
// Tokenized — every significant word in `query` must appear somewhere in
// the file's content/filename/subject/summary. Files with empty extracted
// content are excluded so the caller can fall back to live extraction.
func (s *Store) SearchFilesContent(groupIDs []int64, query string, limit int) ([]model.File, error) {
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
		  mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id, wa_message_id, status, created_at, posted_at, COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
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
	if strings.TrimSpace(what) == "" {
		return nil
	}
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
		"received": true, "receive": true, "got": true, "gets": true, "getting": true,
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
	shortAllowed := map[string]bool{
		"c": true, "r": true, "go": true, "os": true, "ai": true, "ml": true,
		"ds": true, "db": true, "cn": true, "se": true, "ia": true, "be": true,
		"it": true, "cs": true, "ee": true, "ec": true, "me": true, "cv": true,
		"py": true, "js": true, "ui": true, "ux": true, "ip": true,
	}
	// A bare number is always kept, however short.
	//
	// "module 1" and "module 4" differ by exactly one character, and that
	// character is the entire question. Dropping it as too short left the
	// query as just "module", every module in the library matched equally,
	// and a question about module 1 was answered from module 4. The phone
	// side of this had already been fixed; this side had not.
	isNumber := func(tok string) bool {
		for _, r := range tok {
			if r < '0' || r > '9' {
				return false
			}
		}
		return tok != ""
	}

	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(cleaned) {
		if (len(tok) < 3 && !shortAllowed[tok] && !isNumber(tok)) || stop[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	// If everything got filtered (e.g. user typed only stopwords), fall back
	// to the original phrase so we still search something.
	if len(out) == 0 {
		trimmed := strings.TrimSpace(lower)
		if trimmed != "" {
			out = []string{trimmed}
		}
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

// CountCommittedFiles is how many of a group's files actually reached Drive.
//
// Counted by status rather than by a non-empty drive_file_id, because a file
// held only in the local store carries a synthetic "local_" id that would
// otherwise read as filed.
func (s *Store) CountCommittedFiles(groupID int64) int {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM files
		  WHERE group_id = ? AND status = 'committed'
		    AND drive_file_id <> '' AND drive_file_id NOT LIKE 'local_%'`,
		groupID,
	).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

// FilesMissingContent lists files stored but never actually read.
//
// A file whose text was never extracted is only findable by its name, which
// for a WhatsApp document called DOC-20260818-WA0041.pdf means not findable at
// all. These are the ones a backfill has to revisit.
func (s *Store) FilesMissingContent(limit int) ([]model.File, error) {
	// Documents only, never photographs.
	//
	// A phone's WhatsApp folder is overwhelmingly images: 762 of 903 in the
	// library this was written against, most of them forwards and memes.
	// Reading them costs the same as reading a contract and returns a
	// description nobody will ever search for. On a metered or rate limited
	// model that is where the entire allowance goes, and the documents people
	// actually want never get read at all.
	rows, err := s.db.Query(
		`SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name,
		        file_size, mime_type, drive_file_id, drive_folder_id, subject, tags,
		        version, parent_file_id, wa_message_id, status, created_at, posted_at,
		        attribution_method, attribution_confidence
		   FROM files
		  WHERE (extracted_content IS NULL OR extracted_content = '')
		    AND status != 'deleted_in_drive'
		    AND mime_type NOT LIKE 'image/%'
		  ORDER BY posted_at DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// GetFilesByIDs returns these files, in the order the ids were given.
//
// Order matters because the ids come from a user choosing documents from a
// list, and the first one they picked is the one they meant first.
func (s *Store) GetFilesByIDs(ids []int64) ([]model.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders, args := buildInClause(ids)
	rows, err := s.db.Query(
		`SELECT f.id, f.group_id, f.user_id, f.shared_by_phone, f.shared_by_name, f.file_name, f.file_size,
		  f.mime_type, f.drive_file_id, f.drive_folder_id, f.subject, f.tags, f.version, f.parent_file_id,
		  f.wa_message_id, f.status, f.created_at, f.posted_at,
		  COALESCE(f.attribution_method,''), COALESCE(f.attribution_confidence,0)
		 FROM files f WHERE f.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found, err := scanFiles(rows)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]model.File, len(found))
	for _, f := range found {
		byID[f.ID] = f
	}
	out := make([]model.File, 0, len(ids))
	for _, id := range ids {
		if f, ok := byID[id]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}
