package model

import "time"

// User represents a registered Reyna user
type User struct {
	ID            int64     `json:"id"`
	Phone         string    `json:"phone"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	GoogleToken   string    `json:"-"`
	GoogleRefresh string    `json:"-"`
	DriveRootID   string    `json:"drive_root_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Group represents a WhatsApp group connected to Reyna
type Group struct {
	ID          int64     `json:"id"`
	WAID        string    `json:"wa_id"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	CreatedBy   int64     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GroupSettings holds per-group configuration
type GroupSettings struct {
	GroupID          int64  `json:"group_id"`
	Enabled          bool   `json:"enabled"`             // toggle: tracking on/off
	Hidden           bool   `json:"hidden"`              // remove button: hides from dashboard until /reyna init
	TrackingMode     string `json:"tracking_mode"`       // "auto" | "reaction"
	AutoCommitHours  int    `json:"auto_commit_hours"`   // 0 = use server default
	ReactionEmoji    string `json:"reaction_emoji"`      // default "📌"
}

// GroupMember links users to groups
type GroupMember struct {
	ID       int64     `json:"id"`
	GroupID  int64     `json:"group_id"`
	UserID   int64     `json:"user_id"`
	Phone    string    `json:"phone"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// File represents a stored file in a user's Drive repo
type File struct {
	ID            int64     `json:"id"`
	GroupID       int64     `json:"group_id"`
	UserID        int64     `json:"user_id"`
	SharedByPhone string    `json:"shared_by_phone"`
	SharedByName  string    `json:"shared_by_name"`
	FileName      string    `json:"file_name"`
	FileSize      int64     `json:"file_size"`
	MimeType      string    `json:"mime_type"`
	DriveFileID   string    `json:"drive_file_id"`
	DriveFolderID string    `json:"drive_folder_id"`
	Subject       string    `json:"subject"`
	Tags          string    `json:"tags"`
	Version       int       `json:"version"`
	ParentFileID  int64     `json:"parent_file_id"`
	WAMessageID      string    `json:"wa_message_id"`
	Status           string    `json:"status"` // "staged" | "committed" | "deleted_in_drive"
	ExtractedContent string    `json:"extracted_content,omitempty"`
	ContentSummary   string    `json:"content_summary,omitempty"`
	ContentHash      string    `json:"content_hash,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// FileVersion tracks version history
type FileVersion struct {
	ID          int64     `json:"id"`
	FileID      int64     `json:"file_id"`
	Version     int       `json:"version"`
	DriveFileID string    `json:"drive_file_id"`
	FileSize    int64     `json:"file_size"`
	ChangedBy   int64     `json:"changed_by"`
	ChangeNote  string    `json:"change_note"`
	CreatedAt   time.Time `json:"created_at"`
}

// ActivityLog tracks all bot interactions
type ActivityLog struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	UserID    int64     `json:"user_id"`
	Action    string    `json:"action"`
	Command   string    `json:"command"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

// WaitlistEntry for the landing page
type WaitlistEntry struct {
	ID        int64     `json:"id"`
	Contact   string    `json:"contact"`
	CreatedAt time.Time `json:"created_at"`
}

// --- API Request/Response types ---

type LoginRequest struct {
	Phone string `json:"phone"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

type AddFileRequest struct {
	GroupWAID     string `json:"group_wa_id"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	MimeType      string `json:"mime_type"`
	FileData      string `json:"file_data"`
	SharedByPhone string `json:"shared_by_phone"`
	SharedByName  string `json:"shared_by_name"`
	Subject       string `json:"subject"`
	WAMessageID   string `json:"wa_message_id"`
}

type FindRequest struct {
	GroupWAID string `json:"group_wa_id"`
	Query     string `json:"query"`
}

type CommandRequest struct {
	GroupWAID     string `json:"group_wa_id"`
	Command       string `json:"command"`
	Args          string `json:"args"`
	UserPhone     string `json:"user_phone"`
	UserName      string `json:"user_name"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	MimeType      string `json:"mime_type"`
	Subject       string `json:"subject"`
}

type CommandResponse struct {
	Reply    string `json:"reply"`
	Files    []File `json:"files,omitempty"`
	LogCount int    `json:"log_count,omitempty"`
}

// ReactionRequest is sent by the bot when a user reacts to a file message
type ReactionRequest struct {
	GroupWAID     string `json:"group_wa_id"`
	UserPhone     string `json:"user_phone"`
	UserName      string `json:"user_name"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	MimeType      string `json:"mime_type"`
	Emoji         string `json:"emoji"`
	WAMessageID   string `json:"wa_message_id"`
}

// NLPClassifyRequest asks the NLP service to classify a file into a folder
type NLPClassifyRequest struct {
	FileName        string   `json:"file_name"`
	ExistingFolders []string `json:"existing_folders"`
}

// NLPClassifyResponse is the result of NLP folder classification
type NLPClassifyResponse struct {
	Folder     string  `json:"folder"`
	IsNew      bool    `json:"is_new"`
	Confidence float64 `json:"confidence"`
}

// NLPIntentRequest parses natural language into a Reyna intent
type NLPIntentRequest struct {
	Message   string `json:"message"`
	GroupWAID string `json:"group_wa_id"`
	UserPhone string `json:"user_phone"`
}

// NLPIntentResponse is the classified intent from natural language
type NLPIntentResponse struct {
	Intent string `json:"intent"` // "save", "search", "history", "status", "help", "push", "unknown"
	Query  string `json:"query"`  // extracted search query if intent is "search"
	Raw    string `json:"raw"`    // original message
}

type DashboardStats struct {
	TotalFiles      int            `json:"total_files"`
	TotalGroups     int            `json:"total_groups"`
	TotalSize       int64          `json:"total_size"`
	RecentFiles     []File         `json:"recent_files"`
	SubjectBreak    map[string]int `json:"subject_breakdown"`
	TopContributors []Contributor  `json:"top_contributors"`
}

type Contributor struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Count int    `json:"count"`
}

type WaitlistRequest struct {
	Contact string `json:"contact"`
}

// ── NLP Conversational Retrieval ──

type NLPRetrievalRequest struct {
	Query     string `json:"query"`      // natural language query
	GroupWAID string `json:"group_wa_id"` // optional: scope to a group
	UserPhone string `json:"user_phone"`
}

type NLPRetrievalResponse struct {
	Files        []File         `json:"files"`
	DriveMatches []DriveMatch   `json:"drive_matches,omitempty"`
	Query        NLPParsedQuery `json:"parsed_query"`
	Reply        string         `json:"reply"`
}

// DriveMatch represents a file found by walking the user's existing Drive folder
// tree (rather than the bot-captured DB). Surfaced alongside DB hits in NLP
// retrieval and Q&A so files organized in Drive before Reyna existed remain
// findable. Enriched from the Reyna DB by drive_file_id when possible — so
// files that were captured by the bot AND organised in Drive carry full
// WHO/WHEN metadata even when surfaced via the Drive walker.
type DriveMatch struct {
	FolderName string    `json:"folder_name"`
	FolderID   string    `json:"folder_id"`
	FileName   string    `json:"file_name"`
	FileID     string    `json:"file_id"`
	MimeType   string    `json:"mime_type,omitempty"`
	SenderName string    `json:"sender_name,omitempty"`
	SharedAt   time.Time `json:"shared_at,omitempty"`
	DBFileID   int64     `json:"db_file_id,omitempty"`
}

type NLPParsedQuery struct {
	Who    string `json:"who"`
	What   string `json:"what"`
	When   string `json:"when"`
	Why    string `json:"why"`
	Raw    string `json:"raw"`
}

// ── Notes Q&A ──

type NotesQARequest struct {
	Question         string   `json:"question"`
	GroupWAID        string   `json:"group_wa_id"`
	UserPhone        string   `json:"user_phone"`
	PreviousQuestion string   `json:"previous_question,omitempty"`
	PreviousAnswer   string   `json:"previous_answer,omitempty"`
	PreviousSources  []string `json:"previous_sources,omitempty"`
}

type NotesQAResponse struct {
	Answer       string       `json:"answer"`
	Sources      []string     `json:"sources"` // filenames used
	DriveSources []DriveMatch `json:"drive_sources,omitempty"`
	Question     string       `json:"question"`
}
