package repository

import (
	"database/sql"
	"time"

	"github.com/hurshnarayan/reyna/internal/attribution"
	"github.com/hurshnarayan/reyna/internal/model"
)

// Storage for the attribution join.
//
// Events are messages we know about, from a notification the phone saw or a
// line in a chat export. They are stored whether or not a file ever turns up
// for them, because a file downloaded hours later still needs the event to
// exist. Links are the candidate matches between the two.

// InsertEvent records one message. Duplicates collapse on the unique index,
// so re-importing an overlapping export is safe and returns 0.
func (s *Store) InsertEvent(e attribution.Event, groupID int64) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO events
		   (group_id, chat_key, chat_name, sender_display, posted_at, raw_text,
		    attachment_name, has_attachment, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		groupID, e.ChatKey, e.ChatName, e.SenderDisplay, e.PostedAt,
		e.Text, e.AttachmentName, boolToInt(e.HasAttachment), e.Source,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, nil
	}
	return res.LastInsertId()
}

// EventsAround returns events within a window of a file's arrival.
//
// Windowed rather than "all events" so the join stays constant-time as the
// table grows. A message a year away cannot explain a file that landed today.
func (s *Store) EventsAround(groupIDs []int64, at time.Time, window time.Duration) ([]attribution.Event, error) {
	from := at.Add(-window)
	to := at.Add(window)

	query := `SELECT id, chat_key, chat_name, sender_display, posted_at, raw_text,
	                 attachment_name, has_attachment, source
	          FROM events WHERE posted_at BETWEEN ? AND ?`
	args := []interface{}{from, to}
	if len(groupIDs) > 0 {
		placeholders, gargs := buildInClause(groupIDs)
		query += ` AND (group_id IN (` + placeholders + `) OR group_id = 0)`
		args = append(args, gargs...)
	}
	query += ` ORDER BY posted_at DESC LIMIT 500`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]attribution.Event, error) {
	var out []attribution.Event
	for rows.Next() {
		var e attribution.Event
		var hasAttachment int
		if err := rows.Scan(&e.ID, &e.ChatKey, &e.ChatName, &e.SenderDisplay,
			&e.PostedAt, &e.Text, &e.AttachmentName, &hasAttachment, &e.Source); err != nil {
			continue
		}
		e.HasAttachment = hasAttachment != 0
		out = append(out, e)
	}
	return out, nil
}

// SaveLinks records every candidate match, marking the winner active.
//
// All candidates are kept rather than only the best, so a chat export imported
// later can promote a different one without the earlier reasoning being lost,
// and so a wrong guess can be explained after the fact.
func (s *Store) SaveLinks(fileID int64, result attribution.Result) error {
	if _, err := s.db.Exec(`UPDATE links SET is_active = 0 WHERE file_id = ?`, fileID); err != nil {
		return err
	}
	for _, l := range result.Candidates {
		active := 0
		if result.Best != nil && l.EventID == result.Best.EventID && l.Method == result.Best.Method {
			active = 1
		}
		if _, err := s.db.Exec(
			`INSERT OR REPLACE INTO links (file_id, event_id, method, confidence, is_active, linked_at)
			 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			fileID, l.EventID, l.Method, l.Confidence, active,
		); err != nil {
			return err
		}
	}
	return nil
}

// ApplyAttribution writes the winning attribution onto the file.
//
// The sender is blanked below the naming threshold, so a weak guess can still
// rank a result without any surface being able to print it.
func (s *Store) ApplyAttribution(fileID int64, sender, chat string, postedAt time.Time, result attribution.Result) error {
	if result.Confidence() < model.AttributionMinNamed {
		sender = ""
	}
	_, err := s.db.Exec(
		`UPDATE files
		    SET shared_by_name = COALESCE(NULLIF(?, ''), shared_by_name),
		        subject = COALESCE(NULLIF(?, ''), subject),
		        posted_at = ?,
		        attribution_method = ?,
		        attribution_confidence = ?
		  WHERE id = ?`,
		sender, chat, postedAt, result.Method(), result.Confidence(), fileID,
	)
	return err
}

// WeaklyAttributed returns files Reyna cannot name, newest first.
//
// Only the weak ones are ever re-joined: a file already attributed at full
// confidence has nothing to gain, and re-deciding it risks replacing a fact
// with a guess.
func (s *Store) WeaklyAttributed(groupIDs []int64, limit int) ([]model.File, error) {
	if limit <= 0 {
		limit = 200
	}
	query := `SELECT id, group_id, user_id, shared_by_phone, shared_by_name, file_name, file_size,
	           mime_type, drive_file_id, drive_folder_id, subject, tags, version, parent_file_id,
	           wa_message_id, status, created_at, posted_at,
	           COALESCE(attribution_method,''), COALESCE(attribution_confidence,0)
	          FROM files WHERE COALESCE(attribution_confidence,0) < ?`
	args := []interface{}{model.AttributionMinNamed}
	if len(groupIDs) > 0 {
		placeholders, gargs := buildInClause(groupIDs)
		query += ` AND group_id IN (` + placeholders + `)`
		args = append(args, gargs...)
	}
	query += ` ORDER BY COALESCE(posted_at, created_at) DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// SetSenderManually records the user telling us who shared a file.
//
// Full confidence, because they know and we do not. This is what sits behind
// every "who shared this?" affordance.
func (s *Store) SetSenderManually(fileID int64, sender string) error {
	_, err := s.db.Exec(
		`UPDATE files SET shared_by_name = ?, attribution_method = ?, attribution_confidence = 1.0
		 WHERE id = ?`,
		sender, model.AttrUser, fileID,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
