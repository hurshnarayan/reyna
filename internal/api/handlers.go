package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hurshnarayan/reyna/internal/auth"
	"github.com/hurshnarayan/reyna/internal/config"
	"github.com/hurshnarayan/reyna/internal/repository"
	"github.com/hurshnarayan/reyna/internal/integrations/gdrive"
	"github.com/hurshnarayan/reyna/internal/integrations/llm"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/nlp"
	"github.com/hurshnarayan/reyna/internal/reyna"
	"github.com/hurshnarayan/reyna/internal/search"
)

type Server struct {
	cfg         *config.Config
	store       *repository.Store
	drive       *gdrive.Service
	reyna       *reyna.Reyna
	classifier  *nlp.Classifier
	search      *search.Service
	llm         llm.Provider
	jobs        *jobRegistry
	mux         *http.ServeMux
	uploadLocks sync.Map // groupID(int64) → *sync.Mutex — serializes bot uploads per-group to prevent race-condition duplicates
}

// uploadLockFor returns the per-group mutex used to serialize bot uploads.
// Without this, six identical bot uploads arriving in the same millisecond
// all pass the FindFileByHash check before any of them inserts, defeating
// hash-based dedup.
func (s *Server) uploadLockFor(groupID int64) *sync.Mutex {
	mu, _ := s.uploadLocks.LoadOrStore(groupID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// looksLikePhoneOrLID checks if a string looks like a phone number or WhatsApp LID
// (not a real human name). Used to avoid storing "+1234567890" or "0440" as shared_by_name.
func looksLikePhoneOrLID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" { return false }
	// Strip + prefix
	cleaned := strings.TrimPrefix(s, "+")
	// Remove common separators
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	// If all digits, it's a phone number or LID
	allDigits := true
	for _, c := range cleaned {
		if c < '0' || c > '9' { allDigits = false; break }
	}
	if allDigits && len(cleaned) >= 1 { return true }
	return false
}

func NewServer(cfg *config.Config, store *repository.Store, drive *gdrive.Service, classifier *nlp.Classifier, searchSvc *search.Service, llmProvider llm.Provider) *Server {
	s := &Server{cfg: cfg, store: store, drive: drive, reyna: reyna.New(), classifier: classifier, search: searchSvc, llm: llmProvider, jobs: newJobRegistry(), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" { origin = "*" }
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" { w.WriteHeader(200); return }
			h(w, r)
		}
	}
	protected := func(h http.HandlerFunc) http.HandlerFunc {
		return wrap(func(w http.ResponseWriter, r *http.Request) {
			auth.Middleware(s.cfg.JWTSecret)(http.HandlerFunc(h)).ServeHTTP(w, r)
		})
	}

	s.mux.HandleFunc("/api/health", wrap(s.handleHealth))
	s.mux.HandleFunc("/api/auth/register", wrap(s.handleRegister))
	s.mux.HandleFunc("/api/auth/login", wrap(s.handleLogin))
	s.mux.HandleFunc("/api/auth/google", wrap(s.handleGoogleAuthStart))
	s.mux.HandleFunc("/api/auth/google/callback", wrap(s.handleGoogleCallback))
	s.mux.HandleFunc("/api/waitlist", wrap(s.handleWaitlist))
	s.mux.HandleFunc("/api/bot/command", wrap(s.handleBotCommand))
	s.mux.HandleFunc("/api/bot/upload", s.handleBotUpload) // no wrap — multipart, not JSON
	s.mux.HandleFunc("/api/bot/reaction", wrap(s.handleBotReaction))
	s.mux.HandleFunc("/api/bot/sync-group", wrap(s.handleBotSyncGroup))
	s.mux.HandleFunc("/api/bot/enabled-groups", wrap(s.handleEnabledGroups))
	s.mux.HandleFunc("/api/bot/group-states", wrap(s.handleGroupStates))
	s.mux.HandleFunc("/api/bot/known-groups", wrap(s.handleKnownGroups))
	s.mux.HandleFunc("/api/bot/group-mode", wrap(s.handleGroupMode))

	s.mux.HandleFunc("/api/me", protected(s.handleMe))
	s.mux.HandleFunc("/api/auth/google/status", protected(s.handleGoogleStatus))
	s.mux.HandleFunc("/api/auth/google/connect", protected(s.handleGoogleConnect))
	s.mux.HandleFunc("/api/auth/google/disconnect", protected(s.handleGoogleDisconnect))
	s.mux.HandleFunc("/api/dashboard", protected(s.handleDashboard))
	s.mux.HandleFunc("/api/groups", protected(s.handleGroups))
	s.mux.HandleFunc("/api/groups/settings", protected(s.handleGroupSettings))
	s.mux.HandleFunc("/api/files", protected(s.handleFiles))
	s.mux.HandleFunc("/api/files/search", protected(s.handleSearchFiles))
	s.mux.HandleFunc("/api/files/suggest", protected(s.handleSuggestFiles))
	s.mux.HandleFunc("/api/files/versions", protected(s.handleFileVersions))
	s.mux.HandleFunc("/api/files/upload", protected(s.handleUploadFile))
	s.mux.HandleFunc("/api/files/delete", protected(s.handleDeleteFile))
	s.mux.HandleFunc("/api/files/staged/remove", protected(s.handleRemoveStaged))
	s.mux.HandleFunc("/api/files/staged/commit", protected(s.handleCommitStaged))
	s.mux.HandleFunc("/api/files/download", protected(s.handleDownloadFile))
	s.mux.HandleFunc("/api/files/exists", protected(s.handleFileExists))
	s.mux.HandleFunc("/api/activity", protected(s.handleActivity))
	s.mux.HandleFunc("/api/drive/folders", protected(s.handleDriveFolders))
	s.mux.HandleFunc("/api/drive/tree", protected(s.handleDriveTree))
	s.mux.HandleFunc("/api/drive/root", protected(s.handleDriveRoot))
	s.mux.HandleFunc("/api/drive/root-folders", protected(s.handleDriveRootFolders))
	s.mux.HandleFunc("/api/drive/folder/create", protected(s.handleDriveFolderCreate))
	s.mux.HandleFunc("/api/drive/folder/rename", protected(s.handleDriveFolderRename))
	s.mux.HandleFunc("/api/drive/folder/delete", protected(s.handleDriveFolderDelete))
	s.mux.HandleFunc("/api/drive/ingest", protected(s.handleDriveIngest))
	s.mux.HandleFunc("/api/drive/ingest/status", protected(s.handleDriveIngestStatus))
	s.mux.HandleFunc("/api/jobs/status", protected(s.handleJobsStatus))

	// v3 — NLP Retrieval + Q&A + LLM status
	// retrieve and qa use wrap (not protected) so bot can call them too,
	// but they try JWT first for dashboard calls
	s.mux.HandleFunc("/api/nlp/retrieve", wrap(s.handleNLPRetrieve))
	s.mux.HandleFunc("/api/nlp/qa", wrap(s.handleNotesQA))
	s.mux.HandleFunc("/api/llm/status", wrap(s.handleLLMStatus))

	// v4 — Qdrant backfill. Re-embeds every file in the user's accessible
	// groups that has extracted_content but isn't yet in the vector DB.
	// Rate-limited internally (~5 files/sec) so it doesn't thrash Gemini.
	s.mux.HandleFunc("/api/recall/backfill", protected(s.handleRecallBackfill))

	// v4 — Reyna's Recall (semantic-enabled aliases of retrieve/qa) and
	// Reyna's Memory (persistent user context).
	s.mux.HandleFunc("/api/recall/search", wrap(s.handleNLPRetrieve))
	s.mux.HandleFunc("/api/recall/ask", wrap(s.handleNotesQA))
	s.mux.HandleFunc("/api/memory", protected(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			s.handleMemoryList(w, r)
		case "POST":
			s.handleMemoryCreate(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, 405)
		}
	}))
	s.mux.HandleFunc("/api/memory/update", protected(s.handleMemoryUpdate))
	s.mux.HandleFunc("/api/memory/toggle", protected(s.handleMemoryToggle))
	s.mux.HandleFunc("/api/memory/delete", protected(s.handleMemoryDelete))

	// v4 — Reyna Live (Vapi voice tool webhooks). Secret-authed via header.
	s.mux.HandleFunc("/api/voice/config", wrap(s.handleVoiceConfig))
	s.mux.HandleFunc("/api/voice/tools/recall-search", wrap(s.handleVoiceRecallSearch))
	s.mux.HandleFunc("/api/voice/tools/recall-ask", wrap(s.handleVoiceRecallAsk))
	s.mux.HandleFunc("/api/voice/tools/list-recent", wrap(s.handleVoiceListRecent))
	s.mux.HandleFunc("/api/voice/tools/list-memories", wrap(s.handleVoiceListMemories))
	s.mux.HandleFunc("/api/voice/tools/add-memory", wrap(s.handleVoiceAddMemory))
	s.mux.HandleFunc("/api/voice/tools/toggle-memory", wrap(s.handleVoiceToggleMemory))
	s.mux.HandleFunc("/api/voice/tools/commit-staged", wrap(s.handleVoiceCommitStaged))

	// Bot: handle incoming WhatsApp voice notes for Recall.
	s.mux.HandleFunc("/api/bot/voice-note", wrap(s.handleBotVoiceNote))
}

// ── Health ──
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "service": "reyna-backend", "version": "0.2.0", "timestamp": time.Now().Format(time.RFC3339)})
}

// ── Auth ──
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req struct { Phone, Name string }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Phone == "" { http.Error(w, `{"error":"phone required"}`, 400); return }
	user, err := s.store.UpsertUser(req.Phone, req.Name)
	if err != nil { http.Error(w, `{"error":"registration failed"}`, 500); return }
	s.store.AutoLinkUserToGroups(user.ID, req.Phone)
	token, _ := auth.GenerateToken(user.ID, s.cfg.JWTSecret)
	json.NewEncoder(w).Encode(model.AuthResponse{Token: token, User: user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req model.LoginRequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Phone == "" { http.Error(w, `{"error":"phone required"}`, 400); return }
	user, err := s.store.GetUserByPhone(req.Phone)
	if err != nil { http.Error(w, `{"error":"user not found, register first"}`, 404); return }
	s.store.AutoLinkUserToGroups(user.ID, req.Phone)
	token, _ := auth.GenerateToken(user.ID, s.cfg.JWTSecret)
	json.NewEncoder(w).Encode(model.AuthResponse{Token: token, User: user})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, err := s.store.GetUserByID(uid)
	if err != nil { http.Error(w, `{"error":"user not found"}`, 404); return }
	json.NewEncoder(w).Encode(user)
}

// ── Dashboard ──
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user != nil {
		gids := s.store.GetUserGroupIDs(uid)
		if len(gids) == 0 {
			s.store.AutoLinkUserToGroups(uid, user.Phone)
		}
	}
	stats, _ := s.store.GetDashboardStats(uid)

	// Get real Drive storage if connected
	storageUsed := s.drive.GetStorageUsed(uid)
	storageLimit := int64(15 * 1024 * 1024 * 1024) // 15GB default
	if user != nil && user.GoogleRefresh != "" {
		token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
		if err == nil {
			used, limit := s.drive.GetDriveStorageUsed(token)
			if used > 0 || limit > 0 {
				storageUsed = used
				storageLimit = limit
			}
			s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"stats": stats, "storage_used": storageUsed, "storage_limit": storageLimit})
}

// ── Groups ──
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	if r.Method == "POST" {
		var req struct { WAID string `json:"wa_id"`; Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		group, _ := s.store.UpsertGroup(req.WAID, req.Name, uid)
		if group != nil { s.store.AddGroupMember(group.ID, uid, "", "admin") }
		json.NewEncoder(w).Encode(group)
		return
	}
	groups, _ := s.store.GetUserGroups(uid)
	if groups == nil { groups = []model.Group{} }
	// Hide synthetic "My Drive" personal groups from the WhatsApp-groups
	// list. Same reason as the group-settings endpoint: they're not chats.
	filtered := groups[:0]
	for _, g := range groups {
		if strings.HasPrefix(g.WAID, "personal:") {
			continue
		}
		filtered = append(filtered, g)
	}
	json.NewEncoder(w).Encode(filtered)
}

// ── Files ──
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	gidStr := r.URL.Query().Get("group_id")
	sortBy := r.URL.Query().Get("sort_by")     // name, date, size, subject, version
	sortOrder := r.URL.Query().Get("sort_order") // asc, desc
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 { limit = 100 }

	// Link if not yet linked
	gids := s.store.GetUserGroupIDs(uid)
	if len(gids) == 0 {
		user, _ := s.store.GetUserByID(uid)
		if user != nil { s.store.AutoLinkUserToGroups(uid, user.Phone) }
		gids = s.store.GetUserGroupIDs(uid)
	}

	var files []model.File
	if sortBy != "" {
		// Use sortable query
		targetGids := gids
		if gidStr != "" {
			gid, _ := strconv.ParseInt(gidStr, 10, 64)
			targetGids = []int64{gid}
		}
		if len(targetGids) > 0 {
			files, _ = s.store.GetFilesWithSorting(targetGids, sortBy, sortOrder, limit)
		}
	} else if gidStr != "" {
		gid, _ := strconv.ParseInt(gidStr, 10, 64)
		files, _ = s.store.GetGroupFiles(gid, limit)
	} else {
		if len(gids) > 0 { files, _ = s.store.GetGroupsFiles(gids, limit) }
		if len(files) == 0 { files, _ = s.store.GetUserFiles(uid, limit) }
	}
	if files == nil { files = []model.File{} }
	json.NewEncoder(w).Encode(files)
}


func (s *Server) handleFileVersions(w http.ResponseWriter, r *http.Request) {
	fid, _ := strconv.ParseInt(r.URL.Query().Get("file_id"), 10, 64)
	versions, _ := s.store.GetFileVersions(fid)
	if versions == nil { versions = []model.FileVersion{} }
	json.NewEncoder(w).Encode(versions)
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	var req model.AddFileRequest
	json.NewDecoder(r.Body).Decode(&req)
	group, err := s.store.GetGroupByWAID(req.GroupWAID)
	if err != nil { http.Error(w, `{"error":"group not found"}`, 404); return }
	user, _ := s.store.GetUserByID(uid)
	accessToken, driveRootID := "", ""
	if user != nil {
		accessToken, driveRootID = user.GoogleToken, user.DriveRootID
		if user.GoogleRefresh != "" {
			if t, e := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0); e == nil { accessToken = t }
		}
	}
	fileData := []byte(req.FileData)
	driveID, folderID, _ := s.drive.SmartUpload(accessToken, driveRootID, uid, req.Subject, req.FileName, req.MimeType, fileData)
	file := &model.File{GroupID: group.ID, UserID: uid, SharedByPhone: req.SharedByPhone, SharedByName: req.SharedByName, FileName: req.FileName, FileSize: req.FileSize, MimeType: req.MimeType, DriveFileID: driveID, DriveFolderID: folderID, Subject: req.Subject, WAMessageID: req.WAMessageID}
	saved, _ := s.store.AddFile(file)
	json.NewEncoder(w).Encode(saved)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	gid, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	logs, _ := s.store.GetActivityLog(gid, 50)
	if logs == nil { logs = []model.ActivityLog{} }
	json.NewEncoder(w).Encode(logs)
}

// ══════════════════════════════════════════
// BOT COMMAND HANDLER
// ══════════════════════════════════════════

func (s *Server) handleBotCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req model.CommandRequest
	json.NewDecoder(r.Body).Decode(&req)

	action, args := s.reyna.ProcessCommand(req.Command)
	userName := req.UserName
	if userName == "" { userName = req.UserPhone }
	user, _ := s.store.UpsertUser(req.UserPhone, userName)

	group, err := s.store.GetGroupByWAID(req.GroupWAID)
	if err != nil { group, _ = s.store.UpsertGroup(req.GroupWAID, "WhatsApp Group", user.ID) }
	if group != nil { s.store.AddGroupMember(group.ID, user.ID, req.UserPhone, "member") }
	groupID := int64(0)
	if group != nil { groupID = group.ID }

	resp := model.CommandResponse{}

	switch action {
	case "add":
		// Only metadata add (no file data) — used as fallback
		fileName := args
		// Parse --folder / -f flag
		folderFlag := ""
		if strings.Contains(args, "--folder ") {
			parts := strings.SplitN(args, "--folder ", 2)
			fileName = strings.TrimSpace(parts[0])
			folderFlag = strings.TrimSpace(parts[1])
		} else if strings.Contains(args, "-f ") {
			parts := strings.SplitN(args, "-f ", 2)
			fileName = strings.TrimSpace(parts[0])
			folderFlag = strings.TrimSpace(parts[1])
		}
		if fileName == "" || fileName == "." { fileName = req.FileName }
		if fileName == "" { fileName = "unknown_file" }
		subject := req.Subject
		if folderFlag != "" { subject = folderFlag }
		file := &model.File{GroupID: groupID, UserID: user.ID, SharedByPhone: req.UserPhone, SharedByName: req.UserName, FileName: fileName, FileSize: req.FileSize, MimeType: req.MimeType, Subject: subject, DriveFileID: fmt.Sprintf("meta_%d", time.Now().UnixNano()), Status: "staged"}
		saved, _ := s.store.AddFile(file)
		total := s.store.CountGroupFiles(groupID)
		v := 1; if saved != nil { v = saved.Version }
		resp.Reply = s.reyna.AddResponse(fileName, v, total) + s.reyna.AddReminder()

	case "commit":
		// Parse -m flag for commit message
		commitMsg := ""
		commitFileName := args
		if strings.Contains(args, "-m ") {
			parts := strings.SplitN(args, "-m ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitMsg = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
		// Parse --folder / -f flag
		commitFolder := ""
		if strings.Contains(commitFileName, "--folder ") {
			parts := strings.SplitN(commitFileName, "--folder ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitFolder = strings.TrimSpace(parts[1])
		} else if strings.Contains(commitFileName, "-f ") {
			parts := strings.SplitN(commitFileName, "-f ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitFolder = strings.TrimSpace(parts[1])
		}
		_ = commitFolder // Used for folder override during upload

		staged, _ := s.store.GetStagedFiles(groupID)
		log.Printf("[COMMIT] staged=%d group=%d msg=%q", len(staged), groupID, commitMsg)
		if len(staged) == 0 {
			resp.Reply = s.reyna.CommitEmptyResponse()
		} else {
			driveUser := s.store.FindDriveConnectedUser(groupID)
			if driveUser != nil {
				log.Printf("[COMMIT] drive_user=%s id=%d has_refresh=%v root=%s configured=%v",
					driveUser.Email, driveUser.ID, driveUser.GoogleRefresh != "", driveUser.DriveRootID, s.drive.IsConfigured())
			} else {
				log.Printf("[COMMIT] NO drive user found")
			}
			uploaded := 0
			skipped := 0
			for i, f := range staged {
				log.Printf("[COMMIT][%d] file=%s id=%d size=%d subject=%s", i, f.FileName, f.ID, f.FileSize, f.Subject)

				canUpload := driveUser != nil && driveUser.GoogleRefresh != "" && s.drive.IsConfigured() && driveUser.DriveRootID != "" && !strings.HasPrefix(driveUser.DriveRootID, "local_")
				log.Printf("[COMMIT][%d] canUpload=%v", i, canUpload)

				if !canUpload {
					log.Printf("[COMMIT][%d] SKIP no drive", i)
					skipped++
					continue
				}

				token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
				if terr != nil {
					log.Printf("[COMMIT][%d] SKIP token error: %v", i, terr)
					skipped++
					continue
				}
				log.Printf("[COMMIT][%d] token_len=%d", i, len(token))
				s.store.UpdateUserGoogle(driveUser.ID, driveUser.Email, token, driveUser.GoogleRefresh, driveUser.DriveRootID)

				// Use commit folder override if provided, otherwise use file subject
				subjectForFolder := f.Subject
				if commitFolder != "" { subjectForFolder = commitFolder }
				folderID, ferr := s.drive.EnsureSubjectFolder(token, driveUser.DriveRootID, subjectForFolder)
				if ferr != nil {
					log.Printf("[COMMIT][%d] SKIP folder error: %v", i, ferr)
					skipped++
					continue
				}
				log.Printf("[COMMIT][%d] folder=%s", i, folderID)

				fileData, rerr := s.drive.GetLocalFileData(f.ID)
				log.Printf("[COMMIT][%d] raw_data len=%d err=%v", i, len(fileData), rerr)
				if len(fileData) == 0 {
					fileData, _ = s.drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName)
					log.Printf("[COMMIT][%d] fallback len=%d", i, len(fileData))
				}
				if len(fileData) == 0 {
					log.Printf("[COMMIT][%d] SKIP no data", i)
					skipped++
					continue
				}

				log.Printf("[COMMIT][%d] uploading %d bytes...", i, len(fileData))
				driveID, uerr := s.drive.UploadFileToDrive(token, folderID, f.FileName, f.MimeType, fileData)
				if uerr != nil {
					log.Printf("[COMMIT][%d] SKIP upload error: %v", i, uerr)
					skipped++
					continue
				}
				if driveID != "" {
					s.store.UpdateFileDriveID(f.ID, driveID, folderID)
					uploaded++
					log.Printf("[COMMIT][%d] SUCCESS drive_id=%s", i, driveID)
				} else {
					log.Printf("[COMMIT][%d] SKIP empty drive id", i)
					skipped++
				}
			}
			count, _ := s.store.CommitFiles(groupID)
			log.Printf("[COMMIT] DONE committed=%d uploaded=%d skipped=%d", count, uploaded, skipped)
			if commitMsg != "" {
				resp.Reply = s.reyna.CommitWithMessageResponse(int(count), commitMsg)
			} else if commitFileName != "" {
				s.store.CommitFileByName(groupID, commitFileName)
				resp.Reply = s.reyna.CommitFileResponse(commitFileName)
			} else {
				resp.Reply = s.reyna.CommitAllResponse(int(count))
			}
			if uploaded > 0 { resp.Reply += fmt.Sprintf("\n\n☁️ %d file(s) pushed to Google Drive!", uploaded) }
			if skipped > 0 && uploaded == 0 {
				if driveUser == nil { resp.Reply += "\n\nNo one has connected Drive yet." } else { resp.Reply += "\n\nDrive upload failed. Check backend logs." }
			}
		}

	case "rm":
		if args == "" || args == "." {
			count, _ := s.store.RemoveAllStaged(groupID)
			if count == 0 { resp.Reply = s.reyna.RmEmptyResponse() } else { resp.Reply = s.reyna.RmAllResponse(int(count)) }
		} else {
			ok, _ := s.store.RemoveStagedFile(groupID, args)
			if ok { resp.Reply = s.reyna.RmFileResponse(args) } else { resp.Reply = s.reyna.RmNotFoundResponse(args) }
		}

	case "staged":
		staged, _ := s.store.GetStagedFiles(groupID)
		if staged == nil { staged = []model.File{} }
		resp.Files = staged
		resp.Reply = s.reyna.StagedResponse(staged)

	case "find":
		if args == "" { resp.Reply = "Find kya? Usage: /reyna find \"DSA notes\" 🤦‍♀️"; break }
		// Parse -a flag for author details
		showAuthor := false
		query := args
		if strings.Contains(args, " -a") {
			showAuthor = true
			query = strings.TrimSpace(strings.Replace(args, " -a", "", 1))
			query = strings.TrimSpace(strings.Replace(query, "-a ", "", 1))
		}
		files, _ := s.store.FindFiles(groupID, query)
		if files == nil { files = []model.File{} }
		resp.Files = files
		if showAuthor {
			resp.Reply = s.reyna.FindWithAuthorResponse(query, files)
		} else {
			resp.Reply = s.reyna.FindResponse(query, files)
		}

	case "log":
		files, _ := s.store.GetGroupFiles(groupID, 10)
		if files == nil { files = []model.File{} }
		total := s.store.CountGroupFiles(groupID)
		resp.Files = files; resp.LogCount = total
		resp.Reply = s.reyna.LogResponse(files, total)

	case "status":
		staged, _ := s.store.GetStagedFiles(groupID)
		total := s.store.CountGroupFiles(groupID)
		// Return git-style status
		resp.Files = staged
		resp.Reply = s.reyna.StatusResponseGit(staged, nil, total-len(staged), total)

	case "help":
		resp.Reply = s.reyna.HelpResponse()

	case "enable":
		if groupID > 0 {
			gs := s.store.GetGroupSettings(groupID)
			gs.GroupID = groupID
			gs.Enabled = true
			gs.Hidden = false // un-hide so it reappears in dashboard
			s.store.UpsertGroupSettings(gs)
			log.Printf("[BOT] Enabled tracking for group %d via /reyna init", groupID)
			resp.Reply = "Tracking enabled."
		} else {
			resp.Reply = "Group not found."
		}

	case "disable":
		if groupID > 0 {
			gs := s.store.GetGroupSettings(groupID)
			gs.GroupID = groupID
			gs.Enabled = false
			s.store.UpsertGroupSettings(gs)
			log.Printf("[BOT] Disabled tracking for group %d via /reyna stop", groupID)
			resp.Reply = "Tracking disabled."
		} else {
			resp.Reply = "Group not found."
		}

	case "tracking":
		// Handled by bot directly, but provide a fallback
		resp.Reply = "📊 Use this in WhatsApp to see tracked/untracked files in real time."

	default:
		// Try to give a helpful suggestion
		parts := strings.Fields(req.Command)
		attempted := ""
		if len(parts) > 1 { attempted = parts[1] }
		if attempted != "" {
			resp.Reply = s.reyna.InvalidCommandResponse(attempted)
		} else {
			resp.Reply = s.reyna.GenericResponse()
		}
	}

	s.store.LogActivity(groupID, user.ID, action, req.Command, "")
	json.NewEncoder(w).Encode(resp)
}

// ══════════════════════════════════════════
// BOT FILE UPLOAD (multipart/form-data)
// ══════════════════════════════════════════

func (s *Server) handleBotUpload(w http.ResponseWriter, r *http.Request) {
	// CORS
	origin := r.Header.Get("Origin")
	if origin == "" { origin = "*" }
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" { w.WriteHeader(200); return }
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }

	// 50MB max
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		log.Printf("❌ ParseMultipartForm: %v", err)
		http.Error(w, `{"error":"parse error: `+err.Error()+`"}`, 400)
		return
	}

	groupWAID := r.FormValue("group_wa_id")
	userPhone := r.FormValue("user_phone")
	userName := r.FormValue("user_name")
	fileName := r.FormValue("file_name")
	mimeType := r.FormValue("mime_type")
	subject := r.FormValue("subject")
	fileSizeStr := r.FormValue("file_size")
	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

	// Read uploaded file
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("❌ FormFile error: %v", err)
		http.Error(w, `{"error":"file missing from form"}`, 400)
		return
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		log.Printf("❌ Read file error: %v (len=%d)", err, len(fileBytes))
		http.Error(w, `{"error":"empty file"}`, 400)
		return
	}

	if fileSize == 0 { fileSize = int64(len(fileBytes)) }
	log.Printf("📤 Bot upload: %s (%d bytes) from %s (phone=%s)", fileName, len(fileBytes), userName, userPhone)

	// ── Content-hash duplicate detection ──
	// Compute SHA-256 of the file bytes BEFORE we do anything expensive. If
	// this exact file already exists in the same WhatsApp group, dismiss the
	// upload as a duplicate (don't classify, don't extract, don't push to
	// Drive). Same hash + different filename → still a duplicate. Same name
	// + different hash → genuine update, save as v2.
	hashSum := sha256.Sum256(fileBytes)
	contentHash := hex.EncodeToString(hashSum[:])
	log.Printf("🔑 hash=%s for %s", contentHash[:12], fileName)

	// We need the group resolved before the dedup check. Resolve the user +
	// group here, then run the dedup check, then continue with classification.
	sharedByName := userName // preserve original name (pushName from WhatsApp)
	// For the user record, use pushName if available, otherwise phone
	userRecordName := userName
	if userRecordName == "" { userRecordName = userPhone }
	user, _ := s.store.UpsertUser(userPhone, userRecordName)

	// If shared_by_name is still empty, try to get it from an existing user record
	if sharedByName == "" && user != nil && user.Name != "" {
		// Only use stored name if it doesn't look like a phone/LID
		stored := strings.TrimSpace(user.Name)
		if stored != "" && !looksLikePhoneOrLID(stored) {
			sharedByName = stored
		}
	}

	group, err := s.store.GetGroupByWAID(groupWAID)
	if err != nil { group, _ = s.store.UpsertGroup(groupWAID, "WhatsApp Group", user.ID) }
	if group != nil { s.store.AddGroupMember(group.ID, user.ID, userPhone, "member") }
	groupID := int64(0)
	if group != nil { groupID = group.ID }

	// ── Per-group serialization lock ──
	// Without this, simultaneous uploads of byte-identical files all pass the
	// dedup check before any of them inserts, leading to N copies of the same
	// file landing in the staging area. The lock makes the check + insert
	// atomic for one group at a time.
	uploadMu := s.uploadLockFor(groupID)
	uploadMu.Lock()
	defer uploadMu.Unlock()

	// ── Dedup check: same hash already in this group? ──
	if existing := s.store.FindFileByHash(groupID, contentHash); existing != nil {
		log.Printf("♻️  Duplicate ignored: %s (matches existing %q, id=%d)", fileName, existing.FileName, existing.ID)
		reply := fmt.Sprintf("Already saved as **%s** — no duplicate created.", existing.FileName)
		if existing.FileName != fileName {
			reply = fmt.Sprintf("This file is already saved as **%s** (uploaded earlier). Skipping duplicate.", existing.FileName)
		}
		s.store.LogActivity(groupID, user.ID, "duplicate_ignored", "/reyna add "+fileName, "duplicate")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"reply":     reply,
			"file_id":   existing.ID,
			"status":    "duplicate",
			"duplicate": true,
		})
		return
	}

	// ── Same filename, different hash → versioned update ──
	// AddFile already auto-bumps version if a row with the same group_id +
	// file_name exists, but we want the *Drive* filename to also reflect the
	// version so users see hii.v2.pdf, not three "hii.pdf" copies.
	versionedName := fileName
	if existingVer := s.store.LatestVersionByName(groupID, fileName); existingVer >= 1 {
		ext := ""
		base := fileName
		if dot := strings.LastIndex(fileName, "."); dot > 0 {
			base = fileName[:dot]
			ext = fileName[dot:]
		}
		versionedName = fmt.Sprintf("%s.v%d%s", base, existingVer+1, ext)
		log.Printf("📝 New version detected for %s — uploading as %s", fileName, versionedName)
	}

	// ── Save file IMMEDIATELY — classify async ──
	// The bot gets an instant response so the checkmark reaction fires
	// within milliseconds. Classification + extraction happen in a background
	// goroutine. The file shows up in the dashboard as "classifying..." and
	// updates to the real subject once Gemini responds.
	if subject == "" {
		subject = "classifying..."
	}

	localID, folderID, _ := s.drive.SmartUpload("", "", user.ID, subject, versionedName, mimeType, fileBytes)
	dbFile := &model.File{GroupID: groupID, UserID: user.ID, SharedByPhone: userPhone, SharedByName: sharedByName, FileName: versionedName, FileSize: fileSize, MimeType: mimeType, Subject: subject, DriveFileID: localID, DriveFolderID: folderID, Status: "staged", ContentHash: contentHash}
	saved, err := s.store.AddFile(dbFile)
	if err != nil { log.Printf("❌ DB: %v", err); http.Error(w, `{"error":"db save failed"}`, 500); return }

	s.drive.SaveLocalFileData(saved.ID, fileBytes)
	log.Printf("✅ Saved file %d: %s (%d bytes) — staged (classifying async)", saved.ID, fileName, len(fileBytes))

	total := s.store.CountGroupFiles(groupID)
	v := 1; if saved != nil { v = saved.Version }
	reply := s.reyna.AddResponse(fileName, v, total)
	s.store.LogActivity(groupID, user.ID, "add", "/reyna add "+fileName, "staged")

	// Respond instantly — bot can send checkmark NOW
	json.NewEncoder(w).Encode(map[string]interface{}{"reply": reply, "file_id": saved.ID, "status": "staged"})

	// ── Background: classify + extract ──
	go func(fileID int64, gID int64, fName, mime string, fSize int64, data []byte, grp *model.Group) {
		classifySubject := ""

		// Build candidate folders
		seen := map[string]bool{}
		var existingFolders []string
		for _, sub := range s.store.DistinctSubjectsForGroup(gID) {
			if sub == "" || sub == "classifying..." || seen[sub] { continue }
			seen[sub] = true
			existingFolders = append(existingFolders, sub)
		}
		driveUser := s.store.FindDriveConnectedUser(gID)
		if driveUser != nil && driveUser.GoogleRefresh != "" && driveUser.DriveRootID != "" {
			token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
			if terr == nil {
				folders, _ := s.drive.ListDriveFolders(token, driveUser.DriveRootID)
				for _, f := range folders {
					n := f["name"]
					if n == "" || seen[n] { continue }
					seen[n] = true
					existingFolders = append(existingFolders, n)
				}
			}
		}

		groupName := ""
		if grp != nil { groupName = grp.Name }
		fmeta := nlp.FileMeta{
			SenderName: sharedByName, SenderPhone: userPhone,
			GroupName: groupName, SharedAt: time.Now(),
		}

		var extractedContent, contentSummary string
		if len(data) > 0 && (strings.Contains(mime, "pdf") || nlp.IsOfficeDoc(mime)) {
			classifySubject, _, _, extractedContent, contentSummary = s.classifier.ClassifyFileWithContent(fName, mime, data, existingFolders, fmeta)
		} else {
			classifySubject, _, _ = s.classifier.ClassifyFile(fName, existingFolders)
		}

		if classifySubject == "" {
			classifySubject = "Uncategorized"
			log.Printf("⚠️  Classification failed for %s — marking as Uncategorized", fName)
			s.store.UpdateFileSubject(fileID, classifySubject)
		} else {
			log.Printf("[NLP] Classified %s → %s (async)", fName, classifySubject)
			s.store.UpdateFileSubject(fileID, classifySubject)
		}

		// Content extraction
		finalContent := extractedContent
		finalSummary := contentSummary
		if finalContent != "" {
			s.store.UpdateFileContent(fileID, finalContent, finalSummary)
			log.Printf("[EXTRACT] Combined: %s → %s", fName, finalSummary)
		} else {
			content, summary := s.classifier.ExtractContent(fName, mime, fSize, data)
			if content != "" {
				s.store.UpdateFileContent(fileID, content, summary)
				finalContent = content
				finalSummary = summary
				log.Printf("[EXTRACT] Async: %s → %s", fName, summary)
			}
		}

		// Index to Qdrant for Reyna's Recall (semantic search). No-op if search
		// service is disabled. Done after extraction so we index the richest text.
		if s.search.IsEnabled() && finalContent != "" {
			file, _ := s.store.GetFileByID(fileID)
			if file != nil {
				groupName := ""
				if group != nil {
					groupName = group.Name
				}
				meta := search.FileMetadata{
					FileID: file.ID, UserID: file.UserID, GroupID: file.GroupID,
					FileName: file.FileName, Subject: file.Subject,
					SenderName: file.SharedByName, GroupName: groupName,
					SharedAt: file.CreatedAt, Summary: finalSummary,
				}
				if err := s.search.IndexFile(meta, finalContent); err != nil {
					log.Printf("[RECALL] index file %d failed: %v", file.ID, err)
				} else {
					log.Printf("[RECALL] indexed %s", fName)
				}
			}
		}
	}(saved.ID, groupID, fileName, mimeType, fileSize, fileBytes, group)
}

// ── Waitlist ──
func (s *Server) handleWaitlist(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req model.WaitlistRequest
		json.NewDecoder(r.Body).Decode(&req)
		s.store.AddWaitlist(strings.TrimSpace(req.Contact))
		count := s.store.CountWaitlist()
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "You're in! 💅", "position": count})
	} else {
		json.NewEncoder(w).Encode(map[string]int{"count": s.store.CountWaitlist()})
	}
}

// ── Google OAuth ──
func (s *Server) handleGoogleAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.drive.IsConfigured() { json.NewEncoder(w).Encode(map[string]interface{}{"configured": false}); return }
	state := r.URL.Query().Get("token")
	if state == "" { http.Error(w, `{"error":"token required"}`, 400); return }
	json.NewEncoder(w).Encode(map[string]string{"url": s.drive.GetAuthURL(state)})
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" { http.Error(w, "Missing code", 400); return }
	tokenInfo, email, err := s.drive.ExchangeCode(code)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><script>window.opener.postMessage({type:"google_auth_error"},"*");window.close();</script></body></html>`)
		return
	}
	if uid, e := auth.ValidateToken(state, s.cfg.JWTSecret); e == nil && uid > 0 {
		existingUser, _ := s.store.GetUserByID(uid)
		rootID := ""

		// Check if user has an existing root that's still valid on Drive
		if existingUser != nil && existingUser.DriveRootID != "" && !strings.HasPrefix(existingUser.DriveRootID, "local_") {
			// Verify the folder still exists on Drive (could be any folder, not just "Reyna")
			if s.drive.DriveFolderExists(tokenInfo.AccessToken, existingUser.DriveRootID) {
				rootID = existingUser.DriveRootID
				log.Printf("☁️ Google Drive reconnected for user %d (%s) — reusing root: %s", uid, email, rootID)
			} else {
				log.Printf("☁️ Previous root folder %s no longer exists on Drive for user %d", existingUser.DriveRootID, uid)
			}
		}

		// No valid root found — find or create a "Reyna" folder
		if rootID == "" {
			var createErr error
			rootID, createErr = s.drive.CreateUserRootFolder(tokenInfo.AccessToken)
			if createErr != nil {
				log.Printf("❌ Failed to create Drive root folder for user %d: %v", uid, createErr)
			} else {
				log.Printf("☁️ Google Drive connected for user %d (%s) — root: %s", uid, email, rootID)
			}
		}

		s.store.UpdateUserGoogle(uid, email, tokenInfo.AccessToken, tokenInfo.RefreshToken, rootID)
		s.store.UpdateUserGoogleExpiry(uid, tokenInfo.ExpiresAt)
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><script>window.opener.postMessage({type:"google_auth_success",email:"%s"},"*");window.close();</script><p>Connected!</p></body></html>`, email)
}

func (s *Server) handleGoogleStatus(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil { http.Error(w, `{"error":"not found"}`, 404); return }

	rootName := ""
	if user.GoogleRefresh != "" && user.DriveRootID != "" {
		token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
		if err == nil {
			rootName = s.drive.GetFolderName(token, user.DriveRootID)
			if rootName == "" { rootName = "Reyna" }
			// Update token if refreshed
			s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": user.GoogleRefresh != "",
		"email": user.Email,
		"drive_root": user.DriveRootID,
		"drive_root_name": rootName,
		"configured": s.drive.IsConfigured(),
	})
}

func (s *Server) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if !s.drive.IsConfigured() { json.NewEncoder(w).Encode(map[string]interface{}{"configured": false}); return }
	uid := auth.GetUserID(r)
	token, _ := auth.GenerateToken(uid, s.cfg.JWTSecret)
	json.NewEncoder(w).Encode(map[string]string{"url": s.drive.GetAuthURL(token)})
}

// ── Google Disconnect ──
func (s *Server) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	s.store.ClearUserGoogle(uid)
	log.Printf("☁️ Google Drive disconnected for user %d", uid)
	json.NewEncoder(w).Encode(map[string]interface{}{"disconnected": true})
}

// ── File Delete (from DB + Drive) ──
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "DELETE" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	var req struct {
		FileID  int64   `json:"file_id"`
		FileIDs []int64 `json:"file_ids"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ids := req.FileIDs
	if req.FileID > 0 && len(ids) == 0 {
		ids = []int64{req.FileID}
	}
	if len(ids) == 0 { http.Error(w, `{"error":"file_id(s) required"}`, 400); return }

	user, _ := s.store.GetUserByID(uid)
	deleted := 0
	for _, fid := range ids {
		file, err := s.store.GetFileByID(fid)
		if err != nil || file == nil { continue }

		// Delete from Drive if connected and has drive ID
		if user != nil && user.GoogleRefresh != "" && file.DriveFileID != "" && !strings.HasPrefix(file.DriveFileID, "local_") && !strings.HasPrefix(file.DriveFileID, "meta_") {
			token, terr := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
			if terr == nil {
				s.drive.DeleteFromDrive(token, file.DriveFileID)
			}
		}
		// Delete local data
		s.drive.DeleteLocalFileData(fid)
		// Delete from DB
		s.store.DeleteFile(fid)
		deleted++
	}

	s.store.LogActivity(0, uid, "delete", fmt.Sprintf("deleted %d file(s)", deleted), "")
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": deleted})
}

// ── Remove Staged Files ──
func (s *Server) handleRemoveStaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req struct {
		FileID  int64   `json:"file_id"`
		FileIDs []int64 `json:"file_ids"`
		All     bool    `json:"all"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	uid := auth.GetUserID(r)
	if req.All {
		gids := s.store.GetUserGroupIDs(uid)
		total := int64(0)
		for _, gid := range gids {
			n, _ := s.store.RemoveAllStaged(gid)
			total += n
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"removed": total})
		return
	}

	ids := req.FileIDs
	if req.FileID > 0 && len(ids) == 0 {
		ids = []int64{req.FileID}
	}
	if len(ids) == 0 { http.Error(w, `{"error":"file_id(s) required"}`, 400); return }

	removed, _ := s.store.DeleteStagedFiles(ids)
	json.NewEncoder(w).Encode(map[string]interface{}{"removed": removed})
}

// ── Commit Staged Files (from dashboard) ──
// handleCommitStaged kicks off an async "push staged files to Drive" job
// and returns immediately. The actual upload work happens in a goroutine
// so the UI can switch tabs or sit idle without aborting the push. The
// frontend polls /api/jobs/status to surface a sticky progress pill.
func (s *Server) handleCommitStaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	job, fresh := s.jobs.start(uid, JobKindPushStaged)
	if !fresh {
		// Already in progress — return the existing job so the frontend
		// can show the pill without starting a duplicate push.
		json.NewEncoder(w).Encode(job)
		return
	}
	go s.runCommitStaged(uid)
	json.NewEncoder(w).Encode(job)
}

// runCommitStaged is the goroutine body for push-staged. Mirrors the old
// sync handler's semantics — walks every group the user has staged files
// in, uploads what's eligible to Drive, then marks the DB rows committed.
// Differs only in that it reports progress into the job registry so the
// dashboard can show "pushing 7/12..." even after the user leaves the
// Files tab.
func (s *Server) runCommitStaged(uid int64) {
	gids := s.store.GetUserGroupIDs(uid)
	// Total = sum of staged across all groups. Seed it so the progress
	// bar has a meaningful denominator from the first frame.
	total := 0
	for _, gid := range gids {
		staged, _ := s.store.GetStagedFiles(gid)
		total += len(staged)
	}
	s.jobs.update(uid, JobKindPushStaged, func(j *Job) {
		j.Total = total; j.Message = "starting push..."
	})

	totalCommitted := int64(0)
	totalUploaded := 0

	for _, gid := range gids {
		staged, _ := s.store.GetStagedFiles(gid)
		if len(staged) == 0 { continue }

		driveUser := s.store.FindDriveConnectedUser(gid)
		canUpload := driveUser != nil && driveUser.GoogleRefresh != "" && s.drive.IsConfigured() && driveUser.DriveRootID != "" && !strings.HasPrefix(driveUser.DriveRootID, "local_")

		log.Printf("[DASHBOARD-COMMIT] group=%d staged=%d driveUser=%v canUpload=%v", gid, len(staged), driveUser != nil, canUpload)
		if driveUser != nil {
			log.Printf("[DASHBOARD-COMMIT] driveUser=%s root=%s hasRefresh=%v", driveUser.Email, driveUser.DriveRootID, driveUser.GoogleRefresh != "")
		}

		if canUpload {
			token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
			if terr == nil {
				s.store.UpdateUserGoogle(driveUser.ID, driveUser.Email, token, driveUser.GoogleRefresh, driveUser.DriveRootID)
				sem := make(chan struct{}, 10)
				var uploadMu sync.Mutex
				var wg sync.WaitGroup
				for _, f := range staged {
					if f.Subject == "classifying..." { continue }
					wg.Add(1)
					go func(f model.File) {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						// Note the file we're working on so the pill can
						// show "pushing <filename>".
						s.jobs.update(uid, JobKindPushStaged, func(j *Job) {
							j.Message = "pushing " + f.FileName
						})
						folderID, ferr := s.drive.EnsureSubjectFolder(token, driveUser.DriveRootID, f.Subject)
						if ferr != nil { return }
						fileData, _ := s.drive.GetLocalFileData(f.ID)
						if len(fileData) == 0 { fileData, _ = s.drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName) }
						if len(fileData) == 0 { return }
						driveID, uerr := s.drive.UploadFileToDrive(token, folderID, f.FileName, f.MimeType, fileData)
						if uerr == nil && driveID != "" {
							s.store.UpdateFileDriveID(f.ID, driveID, folderID)
							uploadMu.Lock()
							totalUploaded++
							uploadMu.Unlock()
							s.jobs.update(uid, JobKindPushStaged, func(j *Job) {
								j.Done = totalUploaded
							})
						} else if uerr != nil {
							log.Printf("⚠️  Drive upload failed for %s: %v", f.FileName, uerr)
							s.jobs.update(uid, JobKindPushStaged, func(j *Job) { j.Errored++ })
						}
					}(f)
				}
				wg.Wait()
			}
		}

		count, _ := s.store.CommitFiles(gid)
		totalCommitted += count
	}

	log.Printf("[DASHBOARD-COMMIT] user=%d committed=%d uploaded=%d", uid, totalCommitted, totalUploaded)
	s.jobs.update(uid, JobKindPushStaged, func(j *Job) {
		j.Done = totalUploaded
		j.Total = total
	})
	msg := "committed " + fmt.Sprintf("%d", totalCommitted) + " files"
	if totalUploaded > 0 {
		msg += ", " + fmt.Sprintf("%d", totalUploaded) + " uploaded to drive"
	}
	s.jobs.finish(uid, JobKindPushStaged, JobStateDone, msg)
}

// handleJobsStatus returns the current state of every kind of background
// job for the caller in one roundtrip. Frontend polls this once to render
// the progress pill regardless of which tab is active.
func (s *Server) handleJobsStatus(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	if uid == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	out := map[string]any{}
	if j := s.jobs.get(uid, JobKindIngestDrive); j != nil {
		out["ingest_drive"] = j
	}
	if j := s.jobs.get(uid, JobKindPushStaged); j != nil {
		out["push_staged"] = j
	}
	json.NewEncoder(w).Encode(out)
}

// ── File Suggest (autocomplete) ──
func (s *Server) handleSuggestFiles(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	q := r.URL.Query().Get("q")
	if q == "" { json.NewEncoder(w).Encode([]string{}); return }
	gids := s.store.GetUserGroupIDs(uid)
	names := s.store.SuggestFiles(gids, q, 10)
	if names == nil { names = []string{} }
	json.NewEncoder(w).Encode(names)
}

// ── Strict File Search (filename only, no false positives) ──
func (s *Server) handleSearchFiles(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	q := r.URL.Query().Get("q")
	if q == "" { http.Error(w, `{"error":"q required"}`, 400); return }

	gids := s.store.GetUserGroupIDs(uid)

	// Check for /content: prefix for content search
	if strings.HasPrefix(q, "/content:") {
		contentQuery := strings.TrimSpace(strings.TrimPrefix(q, "/content:"))
		if contentQuery == "" { json.NewEncoder(w).Encode([]model.File{}); return }
		// Search inside file contents from local storage
		var matches []model.File
		for _, gid := range gids {
			allFiles, _ := s.store.GetGroupFiles(gid, 500)
			for _, f := range allFiles {
				data, err := s.drive.GetLocalFileData(f.ID)
				if err != nil {
					data, _ = s.drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName)
				}
				if len(data) > 0 {
					content := string(data)
					if strings.Contains(strings.ToLower(content), strings.ToLower(contentQuery)) {
						matches = append(matches, f)
					}
				}
			}
		}
		if matches == nil { matches = []model.File{} }
		json.NewEncoder(w).Encode(matches)
		return
	}

	// Strict filename search
	files, _ := s.store.FindFilesStrict(gids, q, 20)
	if files == nil { files = []model.File{} }
	json.NewEncoder(w).Encode(files)
}

// ── File Download ──
func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	fidStr := r.URL.Query().Get("file_id")
	fid, _ := strconv.ParseInt(fidStr, 10, 64)
	if fid == 0 { http.Error(w, `{"error":"file_id required"}`, 400); return }

	file, err := s.store.GetFileByID(fid)
	if err != nil || file == nil { http.Error(w, `{"error":"file not found"}`, 404); return }

	// Try local data first
	data, err := s.drive.GetLocalFileData(fid)
	if err != nil || len(data) == 0 {
		data, _ = s.drive.GetFileFromLocalStore(file.UserID, file.Subject, file.FileName)
	}

	// If still no data, try downloading from Drive
	if len(data) == 0 && file.DriveFileID != "" && !strings.HasPrefix(file.DriveFileID, "local_") && !strings.HasPrefix(file.DriveFileID, "meta_") {
		user, _ := s.store.GetUserByID(uid)
		if user != nil && user.GoogleRefresh != "" {
			token, terr := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
			if terr == nil {
				data, _ = s.drive.DownloadFromDrive(token, file.DriveFileID)
			}
		}
	}

	if len(data) == 0 {
		// Return a JSON response with drive link if available
		resp := map[string]interface{}{"available": false, "file_name": file.FileName}
		if file.DriveFileID != "" && !strings.HasPrefix(file.DriveFileID, "local_") && !strings.HasPrefix(file.DriveFileID, "meta_") {
			resp["drive_url"] = fmt.Sprintf("https://drive.google.com/file/d/%s/view", file.DriveFileID)
			resp["preview_url"] = fmt.Sprintf("https://drive.google.com/file/d/%s/preview", file.DriveFileID)
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	mimeType := file.MimeType
	if mimeType == "" { mimeType = "application/octet-stream" }
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, file.FileName))
	w.Header().Del("Access-Control-Allow-Headers") // Let browser handle content type
	w.Write(data)
}

// ── File Exists Check ──
func (s *Server) handleFileExists(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	gidStr := r.URL.Query().Get("group_id")
	gid, _ := strconv.ParseInt(gidStr, 10, 64)
	if name == "" { http.Error(w, `{"error":"name required"}`, 400); return }
	file, exists := s.store.FileExistsInGroup(gid, name)
	resp := map[string]interface{}{"exists": exists}
	if exists && file != nil {
		resp["file"] = file
	}
	json.NewEncoder(w).Encode(resp)
}

// ── Drive Folders ──
func (s *Server) handleDriveFolders(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" { json.NewEncoder(w).Encode([]interface{}{}); return }
	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil { json.NewEncoder(w).Encode([]interface{}{}); return }

	parentID := r.URL.Query().Get("parent_id")
	if parentID == "" { parentID = user.DriveRootID }
	if parentID == "" { json.NewEncoder(w).Encode([]interface{}{}); return }

	folders, _ := s.drive.ListDriveFolders(token, parentID)
	if folders == nil { folders = []map[string]string{} }
	json.NewEncoder(w).Encode(folders)
}

// ── Drive Tree (folders + files for a parent) ──
func (s *Server) handleDriveTree(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"folders": []interface{}{}, "files": []interface{}{}})
		return
	}
	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"folders": []interface{}{}, "files": []interface{}{}})
		return
	}

	parentID := r.URL.Query().Get("parent_id")
	if parentID == "" { parentID = user.DriveRootID }
	if parentID == "" {
		// No root set — return empty
		json.NewEncoder(w).Encode(map[string]interface{}{"folders": []interface{}{}, "files": []interface{}{}, "root_name": "", "parent_id": "", "no_root": true})
		return
	}

	s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)

	folders, _ := s.drive.ListDriveFolders(token, parentID)
	files, _ := s.drive.ListDriveFiles(token, parentID)
	if folders == nil { folders = []map[string]string{} }
	if files == nil { files = []map[string]interface{}{} }

	rootName := ""
	if parentID == user.DriveRootID {
		rootName = s.drive.GetFolderName(token, parentID)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"folders": folders, "files": files, "root_name": rootName, "parent_id": parentID})
}

// ── Change Drive Root ──
func (s *Server) handleDriveRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	var req struct {
		FolderID string `json:"folder_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.FolderID == "" { http.Error(w, `{"error":"folder_id required"}`, 400); return }

	user, _ := s.store.GetUserByID(uid)
	if user == nil { http.Error(w, `{"error":"user not found"}`, 404); return }
	s.store.UpdateUserGoogle(uid, user.Email, user.GoogleToken, user.GoogleRefresh, req.FolderID)
	log.Printf("📁 Drive root changed for user %d to %s", uid, req.FolderID)
	json.NewEncoder(w).Encode(map[string]interface{}{"updated": true, "drive_root": req.FolderID})
}

// ── List Root-Level Drive Folders (for folder picker) ──
func (s *Server) handleDriveRootFolders(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" { json.NewEncoder(w).Encode([]interface{}{}); return }
	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil { json.NewEncoder(w).Encode([]interface{}{}); return }
	folders, _ := s.drive.ListRootFolders(token)
	if folders == nil { folders = []map[string]string{} }
	json.NewEncoder(w).Encode(folders)
}

// ── Folder Create (in Drive) ──
func (s *Server) handleDriveFolderCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" { http.Error(w, `{"error":"drive not connected"}`, 400); return }

	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
		SetRoot  bool   `json:"set_as_root"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" { http.Error(w, `{"error":"name required"}`, 400); return }

	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil {
		log.Printf("❌ Token error for user %d: %v", uid, err)
		http.Error(w, `{"error":"token refresh failed — try disconnecting and reconnecting Drive"}`, 500)
		return
	}
	s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)

	// Determine parent: use explicit parent, or current root, or Drive root (empty = My Drive root)
	parentID := req.ParentID
	if parentID == "" && !req.SetRoot {
		// Creating subfolder inside current tracking root
		parentID = user.DriveRootID
	}
	// If parentID is still empty, folder gets created at My Drive root level

	folderID, err := s.drive.CreateDriveFolder(token, req.Name, parentID)
	if err != nil {
		log.Printf("❌ Create folder error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"failed to create folder: %s"}`, err.Error()), 500)
		return
	}

	setAsRoot := req.SetRoot || (user.DriveRootID == "" && req.ParentID == "")
	if setAsRoot {
		s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, folderID)
		log.Printf("📁 Created + set as root: %s (%s) for user %d", req.Name, folderID, uid)
	} else {
		log.Printf("📁 Created subfolder: %s (%s) under %s for user %d", req.Name, folderID, parentID, uid)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"id": folderID, "name": req.Name, "set_as_root": setAsRoot})
}

// ── Folder Rename (in Drive) ──
func (s *Server) handleDriveFolderRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" { http.Error(w, `{"error":"drive not connected"}`, 400); return }

	var req struct {
		FolderID string `json:"folder_id"`
		NewName  string `json:"new_name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.FolderID == "" || req.NewName == "" { http.Error(w, `{"error":"folder_id and new_name required"}`, 400); return }

	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil { http.Error(w, `{"error":"token error"}`, 500); return }
	s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)

	err = s.drive.RenameDriveFolder(token, req.FolderID, req.NewName)
	if err != nil {
		log.Printf("❌ Rename folder error: %v", err)
		http.Error(w, `{"error":"failed to rename folder"}`, 500)
		return
	}

	log.Printf("📁 Renamed folder %s → %s for user %d", req.FolderID, req.NewName, uid)
	json.NewEncoder(w).Encode(map[string]interface{}{"renamed": true, "id": req.FolderID, "name": req.NewName})
}

// ── Folder Delete (in Drive — moves to trash) ──
func (s *Server) handleDriveFolderDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	user, _ := s.store.GetUserByID(uid)
	if user == nil || user.GoogleRefresh == "" { http.Error(w, `{"error":"drive not connected"}`, 400); return }

	var req struct {
		FolderID string `json:"folder_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.FolderID == "" { http.Error(w, `{"error":"folder_id required"}`, 400); return }

	token, err := s.drive.GetValidToken(user.GoogleToken, user.GoogleRefresh, 0)
	if err != nil { http.Error(w, `{"error":"token error"}`, 500); return }
	s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, user.DriveRootID)

	err = s.drive.DeleteFromDrive(token, req.FolderID)
	if err != nil {
		log.Printf("❌ Delete folder error: %v", err)
		http.Error(w, `{"error":"failed to delete folder"}`, 500)
		return
	}

	// If this was the root folder, clear the root
	if req.FolderID == user.DriveRootID {
		s.store.UpdateUserGoogle(uid, user.Email, token, user.GoogleRefresh, "")
		log.Printf("📁 Deleted root folder %s for user %d — root cleared", req.FolderID, uid)
	} else {
		log.Printf("📁 Deleted folder %s for user %d", req.FolderID, uid)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "root_cleared": req.FolderID == user.DriveRootID})
}

// ══════════════════════════════════════════
// GROUP SETTINGS
// ══════════════════════════════════════════

func (s *Server) handleGroupSettings(w http.ResponseWriter, r *http.Request) {
	uid := auth.GetUserID(r)

	if r.Method == "POST" {
		var req struct {
			GroupID         int64  `json:"group_id"`
			Enabled         *bool  `json:"enabled"`
			Hidden          *bool  `json:"hidden"`
			TrackingMode    string `json:"tracking_mode"`
			AutoCommitHours int    `json:"auto_commit_hours"`
			ReactionEmoji   string `json:"reaction_emoji"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.GroupID == 0 {
			http.Error(w, `{"error":"group_id required"}`, 400)
			return
		}

		gs := s.store.GetGroupSettings(req.GroupID)
		if req.Enabled != nil {
			gs.Enabled = *req.Enabled
		}
		if req.Hidden != nil {
			gs.Hidden = *req.Hidden
		}
		if req.TrackingMode == "auto" || req.TrackingMode == "reaction" {
			gs.TrackingMode = req.TrackingMode
		}
		if req.AutoCommitHours > 0 {
			gs.AutoCommitHours = req.AutoCommitHours
		}
		if req.ReactionEmoji != "" {
			gs.ReactionEmoji = req.ReactionEmoji
		}

		s.store.UpsertGroupSettings(gs)
		log.Printf("Group %d settings updated by user %d: enabled=%v mode=%s", req.GroupID, uid, gs.Enabled, gs.TrackingMode)
		json.NewEncoder(w).Encode(gs)
		return
	}

	// GET
	gidStr := r.URL.Query().Get("group_id")
	gid, _ := strconv.ParseInt(gidStr, 10, 64)
	if gid == 0 {
		// Auto-link user to any new groups first
		user, _ := s.store.GetUserByID(uid)
		if user != nil {
			s.store.AutoLinkUserToGroups(uid, user.Phone)
		}
		// Get ALL groups (not just user's — for simplicity, show everything)
		groups, _ := s.store.GetAllGroups()
		var result []map[string]interface{}
		for _, g := range groups {
			// Skip synthetic "My Drive" personal groups. They exist in the
			// schema so Recall can scope files, but they're not WhatsApp
			// groups — the Active Groups UI is about bot-monitored chats.
			if strings.HasPrefix(g.WAID, "personal:") {
				continue
			}
			gs := s.store.GetGroupSettings(g.ID)
			// Only show groups that aren't hidden. Hidden groups were removed
			// via the "remove group" button and only come back via /reyna init.
			// Disabled (toggle off) groups still show — just with the toggle off.
			if !gs.Hidden {
				result = append(result, map[string]interface{}{
					"group":    g,
					"settings": gs,
				})
			}
		}
		if result == nil {
			result = []map[string]interface{}{}
		}
		json.NewEncoder(w).Encode(result)
		return
	}
	gs := s.store.GetGroupSettings(gid)
	json.NewEncoder(w).Encode(gs)
}

// ══════════════════════════════════════════
// BOT REACTION HANDLER (emoji staging)
// ══════════════════════════════════════════

func (s *Server) handleBotReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	var req model.ReactionRequest
	json.NewDecoder(r.Body).Decode(&req)

	if req.FileName == "" {
		http.Error(w, `{"error":"file_name required"}`, 400)
		return
	}

	user, _ := s.store.UpsertUser(req.UserPhone, req.UserName)
	group, err := s.store.GetGroupByWAID(req.GroupWAID)
	if err != nil {
		group, _ = s.store.UpsertGroup(req.GroupWAID, "WhatsApp Group", user.ID)
	}
	if group != nil {
		s.store.AddGroupMember(group.ID, user.ID, req.UserPhone, "member")
	}
	groupID := int64(0)
	if group != nil {
		groupID = group.ID
	}

	// Use NLP to classify the file into a folder
	var existingFolders []string
	driveUser := s.store.FindDriveConnectedUser(groupID)
	if driveUser != nil && driveUser.GoogleRefresh != "" && driveUser.DriveRootID != "" {
		token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
		if terr == nil {
			folders, _ := s.drive.ListDriveFolders(token, driveUser.DriveRootID)
			for _, f := range folders {
				existingFolders = append(existingFolders, f["name"])
			}
		}
	}

	subject, _, _ := s.classifier.ClassifyFile(req.FileName, existingFolders)
	log.Printf("[REACTION] File=%s classified as=%s by NLP", req.FileName, subject)

	file := &model.File{
		GroupID:       groupID,
		UserID:        user.ID,
		SharedByPhone: req.UserPhone,
		SharedByName:  req.UserName,
		FileName:      req.FileName,
		FileSize:      req.FileSize,
		MimeType:      req.MimeType,
		Subject:       subject,
		WAMessageID:   req.WAMessageID,
		Status:        "staged",
	}
	saved, _ := s.store.AddFile(file)
	s.store.LogActivity(groupID, user.ID, "reaction_stage", req.FileName, "staged via "+req.Emoji)

	reply := s.reyna.ReactionStagedResponse(req.FileName)
	json.NewEncoder(w).Encode(map[string]interface{}{"reply": reply, "file_id": saved.ID, "status": "staged", "subject": subject})
}

// ── Enabled Groups (for bot allowlist) ──
func (s *Server) handleEnabledGroups(w http.ResponseWriter, r *http.Request) {
	ids := s.store.GetAllEnabledGroupWAIDs()
	if ids == nil {
		ids = []string{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": ids})
}

// handleGroupStates returns enabled + hidden status for every known group.
// Used by the bot's state-change watcher to detect dashboard toggles/removes
// and send appropriate WhatsApp messages.
func (s *Server) handleGroupStates(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.GetAllGroups()
	states := make(map[string]map[string]bool)
	for _, g := range groups {
		gs := s.store.GetGroupSettings(g.ID)
		states[g.WAID] = map[string]bool{"enabled": gs.Enabled, "hidden": gs.Hidden}
	}
	json.NewEncoder(w).Encode(states)
}

// ── Sync Group from Bot (auto-registers groups from WhatsApp) ──
func (s *Server) handleBotSyncGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req struct {
		WAID        string `json:"wa_id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.WAID == "" { http.Error(w, `{"error":"wa_id required"}`, 400); return }
	// Don't default to "WhatsApp Group" — if the bot couldn't get the name,
	// keep whatever name the DB already has. Only update if we got a real one.

	// Try to get existing group first, create if not exists
	group, err := s.store.GetGroupByWAID(req.WAID)
	if err != nil || group == nil {
		// Insert directly without foreign key on created_by
		s.store.InsertGroupRaw(req.WAID, req.Name, req.MemberCount)
		group, _ = s.store.GetGroupByWAID(req.WAID)
	}

	if group != nil {
		// Only update name if we got a real one — never overwrite with empty or placeholder
		if req.Name != "" && req.Name != "WhatsApp Group" && req.Name != group.Name {
			s.store.UpdateGroupName(group.ID, req.Name)
			log.Printf("[SYNC-GROUP] Updated group %d name: %q → %q", group.ID, group.Name, req.Name)
		}
		if req.MemberCount > 0 {
			s.store.UpdateGroupMemberCount(group.ID, req.MemberCount)
		}
		// Create group settings with defaults if they don't exist yet.
		// Do NOT re-enable if the user explicitly disabled the group —
		// that was overriding the dashboard toggle on every bot sync.
		gs := s.store.GetGroupSettings(group.ID)
		if gs.GroupID == 0 || !s.store.GroupSettingsExist(group.ID) {
			gs.GroupID = group.ID
			gs.Enabled = true
			s.store.UpsertGroupSettings(gs)
			log.Printf("Auto-enabled group %d (%s) via first sync", group.ID, req.Name)
		}
	}

	gid := int64(0)
	if group != nil { gid = group.ID }
	json.NewEncoder(w).Encode(map[string]interface{}{"synced": true, "group_id": gid})
}

// ── Known Groups (all groups in DB — for bot preload) ──
func (s *Server) handleKnownGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.GetAllGroups()
	var ids []string
	for _, g := range groups {
		ids = append(ids, g.WAID)
	}
	if ids == nil { ids = []string{} }
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": ids})
}

// ── Group Tracking Mode (for bot) ──
func (s *Server) handleGroupMode(w http.ResponseWriter, r *http.Request) {
	waID := r.URL.Query().Get("wa_id")
	if waID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"mode": "auto", "enabled": false})
		return
	}
	enabled := s.store.IsGroupEnabled(waID)
	mode := s.store.GetGroupTrackingMode(waID)
	json.NewEncoder(w).Encode(map[string]interface{}{"mode": mode, "enabled": enabled})
}

// ══════════════════════════════════════════
// v3: NLP CONVERSATIONAL RETRIEVAL
// ══════════════════════════════════════════

func (s *Server) handleNLPRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req model.NLPRetrievalRequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Query == "" {
		http.Error(w, `{"error":"query required"}`, 400)
		return
	}

	// Parse the natural language query into WHO/WHAT/WHEN/WHY
	who, what, when, why := s.classifier.ParseNLPQuery(req.Query)
	log.Printf("[NLP-RETRIEVE] query=%q → who=%q what=%q when=%q why=%q", req.Query, who, what, when, why)

	// Resolve time window
	var sinceTime *time.Time
	now := time.Now()
	switch when {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		sinceTime = &t
	case "yesterday":
		t := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
		sinceTime = &t
	case "last_week", "this_week":
		t := now.AddDate(0, 0, -7)
		sinceTime = &t
	case "last_month":
		t := now.AddDate(0, -1, 0)
		sinceTime = &t
	}

	// Get group IDs to search
	var groupIDs []int64
	if req.GroupWAID != "" {
		group, err := s.store.GetGroupByWAID(req.GroupWAID)
		if err == nil {
			groupIDs = []int64{group.ID}
		}
	}
	// If no specific group, search all groups
	if len(groupIDs) == 0 {
		// Try JWT token from Authorization header (dashboard calls)
		userID := int64(0)
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if parts := strings.SplitN(authHeader, " ", 2); len(parts) == 2 {
				if uid, err := auth.ValidateToken(parts[1], s.cfg.JWTSecret); err == nil {
					userID = uid
				}
			}
		}
		if userID > 0 {
			groupIDs = s.store.GetUserGroupIDs(userID)
			// Auto-link fallback — mirrors handleFiles's behaviour so a
			// dashboard user who hasn't yet been stitched into group_members
			// doesn't see empty Recall results even though their files exist.
			if len(groupIDs) == 0 {
				if user, _ := s.store.GetUserByID(userID); user != nil {
					s.store.AutoLinkUserToGroups(userID, user.Phone)
					groupIDs = s.store.GetUserGroupIDs(userID)
				}
			}
		}
		// Fallback: if from bot (no JWT), get groups from phone
		if len(groupIDs) == 0 && req.UserPhone != "" {
			user, err := s.store.GetUserByPhone(req.UserPhone)
			if err == nil {
				s.store.AutoLinkUserToGroups(user.ID, user.Phone)
				groupIDs = s.store.GetUserGroupIDs(user.ID)
			}
		}
	}
	log.Printf("[NLP-RETRIEVE] groupIDs=%v", groupIDs)

	files, _ := s.store.SearchFilesNLP(groupIDs, who, what, sinceTime, 20)
	if files == nil {
		files = []model.File{}
	}
	// Defensive: if WHO is set but the strict filter found nothing, retry by sender only
	if len(files) == 0 && who != "" {
		fallback, _ := s.store.SearchFilesNLP(groupIDs, who, "", nil, 20)
		if len(fallback) > 0 {
			files = fallback
		}
	}

	// Also walk the user's existing Drive folder tree for matches that were
	// never captured by the bot. This is the fix for "Reyna only sees its own
	// staging table" — older notes already organised in Drive are now searchable.
	driveMatches := s.collectDriveContext(groupIDs, what, 25)
	// If WHO was specified, drop Drive matches that can't be attributed.
	if who != "" {
		driveMatches = filterDriveMatchesByWho(driveMatches, who)
	}
	log.Printf("[NLP-RETRIEVE] db_files=%d drive_matches=%d (after metadata pass)", len(files), len(driveMatches))

	// ── Semantic search via Qdrant (Reyna's Recall) ──
	// Augments the keyword/metadata search above with meaning-based matches.
	// A file called asn2_v3.pdf about gradient descent will surface here for
	// a query like "that ML assignment on gradients" even though no token
	// matches. When semantic finds hits we skip the expensive deep retrieve.
	semanticFound := false
	if s.search.IsEnabled() {
		if hits, err := s.search.SearchFiles(req.Query, groupIDs, 10); err == nil && len(hits) > 0 {
			seen := map[int64]bool{}
			merged := make([]model.File, 0, len(hits)+len(files))
			for _, h := range hits {
				file, err := s.store.GetFileByID(h.FileID)
				if err == nil && file != nil && !seen[file.ID] {
					merged = append(merged, *file)
					seen[file.ID] = true
				}
			}
			for _, f := range files {
				if !seen[f.ID] {
					merged = append(merged, f)
					seen[f.ID] = true
				}
			}
			files = merged
			semanticFound = true
			log.Printf("[RECALL] merged %d semantic hits → %d total", len(hits), len(files))
		}
	}

	// ── Deep content retrieval (Fix 3 from earlier) ──
	// If metadata + semantic both missed, OR the query has specific content
	// cues that metadata can't catch ("the diagram with R1 R2", "the page
	// mentioning Wien bridge"), send candidate PDFs to Gemini and ask which
	// ones actually match. Cost: ~₹0.05 per candidate, capped at 5.
	// Skipped when semantic already found strong hits — Qdrant is cheaper.
	if (!semanticFound && len(files) == 0) || hasContentCues(req.Query) {
		log.Printf("[NLP-RETRIEVE] triggering deep content retrieval")
		deepHits := s.deepContentRetrieve(groupIDs, req.Query, who, sinceTime, 5)
		if len(deepHits) > 0 {
			// Merge: deep hits take priority, then add metadata hits not already present
			seen := map[int64]bool{}
			merged := make([]model.File, 0, len(deepHits)+len(files))
			for _, f := range deepHits {
				if !seen[f.ID] {
					merged = append(merged, f)
					seen[f.ID] = true
				}
			}
			for _, f := range files {
				if !seen[f.ID] {
					merged = append(merged, f)
					seen[f.ID] = true
				}
			}
			files = merged
			log.Printf("[NLP-RETRIEVE] after deep retrieve: db_files=%d", len(files))
		}
	}

	// Build a conversational LLM-generated reply (multi-language, intent-aware).
	// We pre-compute the time strings so the LLM can't hallucinate "2 days ago"
	// when the file was actually shared 2 minutes ago. All times are in IST.
	dbView := make([]nlp.RetrievalFile, 0, len(files))
	for _, f := range files {
		dbView = append(dbView, nlp.RetrievalFile{
			Name:     f.FileName,
			Folder:   f.Subject,
			Sender:   f.SharedByName,
			SharedAt: formatSharedAt(f.CreatedAt),
			Summary:  s.store.GetFileExtractedContent([]int64{f.ID})[f.ID],
		})
	}
	driveView := make([]nlp.RetrievalFile, 0, len(driveMatches))
	for _, m := range driveMatches {
		shared := ""
		if !m.SharedAt.IsZero() {
			shared = formatSharedAt(m.SharedAt)
		}
		driveView = append(driveView, nlp.RetrievalFile{
			Name:     m.FileName,
			Folder:   m.FolderName,
			Sender:   m.SenderName,
			SharedAt: shared,
		})
	}
	reply := s.classifier.GenerateRetrievalReply(req.Query, who, what, when, why, dbView, driveView)

	json.NewEncoder(w).Encode(model.NLPRetrievalResponse{
		Files:        files,
		DriveMatches: driveMatches,
		Query:        model.NLPParsedQuery{Who: who, What: what, When: when, Why: why, Raw: req.Query},
		Reply:        reply,
	})
}

func (s *Server) buildNLPReply(files []model.File, driveMatches []model.DriveMatch, who, what, when string) string {
	if len(files) == 0 && len(driveMatches) == 0 {
		msg := "No files found"
		if who != "" {
			msg += " from " + who
		}
		if what != "" {
			msg += " matching \"" + what + "\""
		}
		if when != "" {
			msg += " in that time period"
		}
		msg += ".\n\nTry being more specific — I can search by:\n• Person: \"What did Priya share?\"\n• Topic: \"Find compiler design notes\"\n• Time: \"What's new since yesterday?\"\n• Combo: \"Rakesh's quantum mechanics PDF from last week\""
		return msg
	}

	if len(files) == 0 && len(driveMatches) > 0 {
		msg := fmt.Sprintf("Found %d file(s) already in your Drive matching \"%s\":", len(driveMatches), what)
		for i, m := range driveMatches {
			if i >= 8 {
				msg += fmt.Sprintf("\n... and %d more", len(driveMatches)-8)
				break
			}
			msg += fmt.Sprintf("\n• %s — in %s/", m.FileName, m.FolderName)
		}
		return msg
	}

	msg := fmt.Sprintf("Found %d file(s)", len(files))
	if who != "" {
		msg += " from " + who
	}
	if what != "" {
		msg += " matching \"" + what + "\""
	}
	if when != "" {
		switch when {
		case "today":
			msg += " from today"
		case "yesterday":
			msg += " from yesterday"
		case "last_week", "this_week":
			msg += " in the last 7 days"
		case "last_month":
			msg += " in the last month"
		}
	}
	msg += ":\n"
	for i, f := range files {
		if i >= 5 {
			msg += fmt.Sprintf("... and %d more", len(files)-5)
			break
		}
		folder := f.Subject
		if folder == "" {
			folder = "Unsorted"
		}
		msg += fmt.Sprintf("\n• %s — %s folder", f.FileName, folder)
		if f.SharedByName != "" {
			msg += " (by " + f.SharedByName + ")"
		}
	}
	if len(driveMatches) > 0 {
		msg += fmt.Sprintf("\n\nPlus %d match(es) already in your Drive:", len(driveMatches))
		for i, m := range driveMatches {
			if i >= 5 {
				msg += fmt.Sprintf("\n... and %d more", len(driveMatches)-5)
				break
			}
			msg += fmt.Sprintf("\n• %s — in %s/", m.FileName, m.FolderName)
		}
	}
	return msg
}

// ══════════════════════════════════════════
// v3: DRIVE FOLDER WALKER (used by NLP retrieval + Q&A)
// ══════════════════════════════════════════

// folderMatchesWhat returns true if a Drive folder name plausibly matches the
// "what" of an NLP query. Uses substring + a small abbrev table so that
// "OS notes" matches "Operating Systems", "compiler" matches "Compiler Design",
// "dbms" matches "Database Management Systems", etc.
func folderMatchesWhat(folderName, what string) bool {
	if what == "" {
		return false
	}
	fn := strings.ToLower(folderName)
	w := strings.ToLower(strings.TrimSpace(what))
	if fn == w || strings.Contains(fn, w) || strings.Contains(w, fn) {
		return true
	}
	// token-level overlap: any significant token in `what` appearing in folder name
	for _, tok := range repository.TokenizeWhat(w) {
		if strings.Contains(fn, tok) {
			return true
		}
	}
	// abbreviation map
	abbrevs := map[string][]string{
		"os":       {"operating system", "operating systems"},
		"dbms":     {"database", "dbms"},
		"cn":       {"computer network", "networking"},
		"daa":      {"design and analysis", "algorithm"},
		"coa":      {"computer organization", "architecture"},
		"dsa":      {"data structure", "algorithm"},
		"caed":     {"computer aided", "engineering drawing", "cad"},
		"ml":       {"machine learning"},
		"ai":       {"artificial intelligence"},
		"oop":      {"object oriented", "object-oriented"},
		"toc":      {"theory of computation", "automata"},
		"compiler": {"compiler design", "compilers"},
		"se":       {"software engineering"},
		"pyq":      {"previous year", "question paper"},
	}
	for short, longs := range abbrevs {
		if strings.Contains(w, short) {
			for _, l := range longs {
				if strings.Contains(fn, l) {
					return true
				}
			}
		}
		for _, l := range longs {
			if strings.Contains(w, l) && strings.Contains(fn, short) {
				return true
			}
		}
	}
	return false
}

// fileMatchesWhat returns true if a Drive filename plausibly matches `what`.
func fileMatchesWhat(fileName, what string) bool {
	if what == "" {
		return false
	}
	fn := strings.ToLower(fileName)
	w := strings.ToLower(strings.TrimSpace(what))
	if strings.Contains(fn, w) {
		return true
	}
	for _, tok := range repository.TokenizeWhat(w) {
		if strings.Contains(fn, tok) {
			return true
		}
	}
	return false
}

// collectDriveContext walks the user's Drive root (the folder set via
// "change folder") and returns files matching `what`. It searches:
//  1. Subfolders whose name matches `what` → returns all files in them
//  2. Within all other subfolders, files whose name matches `what`
// Capped at `maxMatches` to avoid huge responses.
func (s *Server) collectDriveContext(groupIDs []int64, what string, maxMatches int) []model.DriveMatch {
	if what == "" || len(groupIDs) == 0 {
		return nil
	}
	if maxMatches <= 0 {
		maxMatches = 25
	}

	var driveUser *model.User
	for _, gid := range groupIDs {
		if u := s.store.FindDriveConnectedUser(gid); u != nil {
			driveUser = u
			break
		}
	}
	if driveUser == nil || driveUser.GoogleRefresh == "" || driveUser.DriveRootID == "" {
		return nil
	}

	token, err := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
	if err != nil {
		log.Printf("[DRIVE-CTX] token refresh failed: %v", err)
		return nil
	}

	subfolders, err := s.drive.ListDriveFolders(token, driveUser.DriveRootID)
	if err != nil {
		log.Printf("[DRIVE-CTX] list folders failed: %v", err)
		return nil
	}

	var matches []model.DriveMatch
	// Pass 1: any subfolder whose name matches → take all its files
	for _, folder := range subfolders {
		if len(matches) >= maxMatches {
			break
		}
		if !folderMatchesWhat(folder["name"], what) {
			continue
		}
		files, err := s.drive.ListDriveFiles(token, folder["id"])
		if err != nil {
			continue
		}
		for _, f := range files {
			if len(matches) >= maxMatches {
				break
			}
			matches = append(matches, model.DriveMatch{
				FolderName: folder["name"],
				FolderID:   folder["id"],
				FileName:   asString(f["name"]),
				FileID:     asString(f["id"]),
				MimeType:   asString(f["mime_type"]),
			})
		}
	}

	// Pass 2: scan files in *non-matching* subfolders for filename hits
	if len(matches) < maxMatches {
		for _, folder := range subfolders {
			if len(matches) >= maxMatches {
				break
			}
			if folderMatchesWhat(folder["name"], what) {
				continue // already covered in pass 1
			}
			files, err := s.drive.ListDriveFiles(token, folder["id"])
			if err != nil {
				continue
			}
			for _, f := range files {
				if len(matches) >= maxMatches {
					break
				}
				name := asString(f["name"])
				if fileMatchesWhat(name, what) {
					matches = append(matches, model.DriveMatch{
						FolderName: folder["name"],
						FolderID:   folder["id"],
						FileName:   name,
						FileID:     asString(f["id"]),
						MimeType:   asString(f["mime_type"]),
					})
				}
			}
		}
	}

	// Enrich each Drive match with WHO/WHEN metadata from Reyna's DB by
	// joining on drive_file_id. Files captured by the bot AND organised in
	// Drive get full sender + shared-at info; pure Drive-natives stay empty.
	matches = s.store.EnrichDriveMatches(matches)
	log.Printf("[DRIVE-CTX] what=%q → %d match(es) under root %s (enriched)", what, len(matches), driveUser.DriveRootID)
	return matches
}

// istLocation is the timezone Reyna displays times in. Hardcoded IST so the
// hackathon demo doesn't depend on server locale. If you ever need a per-user
// timezone, store it on the user record and look it up here.
var istLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// Fallback: hardcoded +05:30 offset
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}()

// formatSharedAt returns a precomputed "X minutes ago, on Mon DD HH:MM IST"
// string. We pre-compute this and feed it to the LLM rather than letting the
// LLM compute relative time itself — otherwise it hallucinates ("2 days ago"
// when the file was 2 minutes ago).
func formatSharedAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	delta := now.Sub(t)
	var rel string
	switch {
	case delta < 60*time.Second:
		rel = "just now"
	case delta < 60*time.Minute:
		rel = fmt.Sprintf("%d minute(s) ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		rel = fmt.Sprintf("%d hour(s) ago", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		rel = fmt.Sprintf("%d day(s) ago", int(delta.Hours()/24))
	default:
		rel = fmt.Sprintf("%d day(s) ago", int(delta.Hours()/24))
	}
	return fmt.Sprintf("%s (on %s)", rel, t.In(istLocation).Format("Mon Jan 2 15:04 IST"))
}

// hasContentCues returns true if the query mentions something specific that
// metadata search can't easily catch — diagrams, figures, equations, tables,
// page references, "the one about X", multi-line recall, etc. When true, we
// trigger deep content retrieval even if metadata search found something,
// because the user is clearly asking for content-level matching.
func hasContentCues(query string) bool {
	q := strings.ToLower(query)
	cues := []string{
		"diagram", "figure", "chart", "graph", "table", "image", "picture",
		"equation", "formula", "theorem", "lemma", "proof", "derivation",
		"the one with", "the one about", "the one that", "the page",
		"page ", "section ", "chapter ", "module ", "unit ",
		"mentioning", "mentions", "talks about", "explains", "discusses",
		"contains", "shows", "depicts", "labelled", "labeled",
		"r1", "r2", "r3", "fig.", "fig ",
	}
	for _, c := range cues {
		if strings.Contains(q, c) {
			return true
		}
	}
	// Long descriptive query (>10 words) usually means specific recall
	if len(strings.Fields(query)) > 10 {
		return true
	}
	return false
}

// deepContentRetrieve does an expensive but accurate AI-powered search:
// for each candidate file (ranked by metadata + recency), it sends the
// actual PDF bytes to Gemini and asks "does this document match the user's
// natural-language description?". Used when metadata search fails or when
// the query contains specific content cues ("the PDF with the wien bridge
// diagram", "the one mentioning Coulomb's law").
//
// Cost: ~₹0.05 per candidate. Capped at 5 candidates per query.
// Latency: ~3-5s per candidate, sequential.
func (s *Server) deepContentRetrieve(groupIDs []int64, rawQuery string, who string, sinceTime *time.Time, maxCandidates int) []model.File {
	if maxCandidates <= 0 {
		maxCandidates = 5
	}
	// Pull candidate pool: WHO + WHEN filter only, no WHAT (we're going to
	// let Gemini decide WHAT). If WHO is empty, get the most recent files.
	candidates, _ := s.store.SearchFilesNLP(groupIDs, who, "", sinceTime, maxCandidates*2)
	if len(candidates) == 0 && who != "" {
		// Drop the WHO filter — maybe the user misspelled the sender
		candidates, _ = s.store.SearchFilesNLP(groupIDs, "", "", sinceTime, maxCandidates*2)
	}
	if len(candidates) == 0 {
		return nil
	}

	log.Printf("[DEEP-RETRIEVE] %d candidate(s) for query=%q", len(candidates), rawQuery)

	// Cap candidates we actually send to Gemini
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	type scored struct {
		file  model.File
		score float64
	}
	var hits []scored
	for _, f := range candidates {
		// Only PDFs/images can be sent as inline doc blocks; for others, fall
		// back to the cached extracted_content textual match.
		canSendAsDoc := strings.Contains(f.MimeType, "pdf") || strings.Contains(f.MimeType, "image")

		var matched bool
		var confidence float64

		if canSendAsDoc {
			data, derr := s.drive.GetLocalFileData(f.ID)
			if derr != nil || len(data) == 0 {
				log.Printf("[DEEP-RETRIEVE] skip %s — no local bytes", f.FileName)
				continue
			}
			matched, confidence = s.classifier.MatchesQuery(rawQuery, f.FileName, f.MimeType, data)
		} else {
			// Text-mode: feed cached content to the matcher
			content := s.store.GetFileExtractedContent([]int64{f.ID})[f.ID]
			if content == "" {
				continue
			}
			matched, confidence = s.classifier.MatchesQueryText(rawQuery, f.FileName, content)
		}
		if matched && confidence >= 0.3 {
			log.Printf("[DEEP-RETRIEVE] HIT %s (confidence=%.0f%%)", f.FileName, confidence*100)
			hits = append(hits, scored{f, confidence})
		} else {
			log.Printf("[DEEP-RETRIEVE] miss %s (confidence=%.0f%%)", f.FileName, confidence*100)
		}
	}

	// Sort by confidence DESC
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j-1].score < hits[j].score; j-- {
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
	out := make([]model.File, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.file)
	}
	return out
}

// filterDriveMatchesByWho drops Drive matches whose enriched sender doesn't
// contain `who`. Pure Drive-native files (no sender info from the DB) are
// dropped when WHO is set, since they can't be attributed.
func filterDriveMatchesByWho(matches []model.DriveMatch, who string) []model.DriveMatch {
	if who == "" {
		return matches
	}
	whoLower := strings.ToLower(strings.TrimSpace(who))
	first := whoLower
	if fields := strings.Fields(whoLower); len(fields) > 0 {
		first = fields[0]
	}
	out := matches[:0]
	for _, m := range matches {
		name := strings.ToLower(m.SenderName)
		if name == "" {
			continue
		}
		if strings.Contains(name, whoLower) || strings.Contains(name, first) {
			out = append(out, m)
		}
	}
	return out
}

// downloadDriveMatchesForQA downloads up to `maxFiles` PDFs from the given
// Drive matches and runs them through the LLM extractor so Q&A can answer
// from the actual document content. Returns a filename → extracted content
// map ready to feed into AnswerFromNotes.
func (s *Server) downloadDriveMatchesForQA(groupIDs []int64, matches []model.DriveMatch, maxFiles int) map[string]string {
	out := map[string]string{}
	if len(matches) == 0 || maxFiles <= 0 {
		return out
	}

	var driveUser *model.User
	for _, gid := range groupIDs {
		if u := s.store.FindDriveConnectedUser(gid); u != nil {
			driveUser = u
			break
		}
	}
	if driveUser == nil {
		return out
	}
	token, err := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
	if err != nil {
		return out
	}

	count := 0
	for _, m := range matches {
		if count >= maxFiles {
			break
		}
		if !strings.Contains(m.MimeType, "pdf") {
			continue
		}
		data, err := s.drive.DownloadFromDrive(token, m.FileID)
		if err != nil {
			log.Printf("[QA] download %s failed: %v", m.FileName, err)
			continue
		}
		log.Printf("[QA] downloaded %s (%d bytes) from Drive folder %s", m.FileName, len(data), m.FolderName)
		content, _ := s.classifier.ExtractContent(m.FileName, "application/pdf", int64(len(data)), data)
		if content != "" {
			out[m.FolderName+"/"+m.FileName] = content
			count++
		}
	}
	return out
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ══════════════════════════════════════════
// v3: NOTES Q&A
// ══════════════════════════════════════════

func (s *Server) handleNotesQA(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req model.NotesQARequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Question == "" {
		http.Error(w, `{"error":"question required"}`, 400)
		return
	}

	// Get group IDs + resolve userID (needed for Memory context below).
	var groupIDs []int64
	userID := int64(0)
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if parts := strings.SplitN(authHeader, " ", 2); len(parts) == 2 {
			if uid, err := auth.ValidateToken(parts[1], s.cfg.JWTSecret); err == nil {
				userID = uid
			}
		}
	}
	if req.GroupWAID != "" {
		group, err := s.store.GetGroupByWAID(req.GroupWAID)
		if err == nil {
			groupIDs = []int64{group.ID}
		}
	}
	if len(groupIDs) == 0 {
		if userID > 0 {
			groupIDs = s.store.GetUserGroupIDs(userID)
			// Auto-link: if the user hasn't yet been stitched into group_members
			// for the groups they own files in, do it now based on their phone.
			// Without this, dashboard users who hadn't used /reyna init from
			// within the group end up with groupIDs=[] here, which silently
			// returns "no notes found" even when files clearly exist.
			if len(groupIDs) == 0 {
				if user, _ := s.store.GetUserByID(userID); user != nil {
					s.store.AutoLinkUserToGroups(userID, user.Phone)
					groupIDs = s.store.GetUserGroupIDs(userID)
				}
			}
		}
		if len(groupIDs) == 0 && req.UserPhone != "" {
			user, err := s.store.GetUserByPhone(req.UserPhone)
			if err == nil {
				s.store.AutoLinkUserToGroups(user.ID, user.Phone)
				groupIDs = s.store.GetUserGroupIDs(user.ID)
				if userID == 0 {
					userID = user.ID
				}
			}
		}
	}

	// Run the NLP parser to extract topic + sender + time hints from natural
	// questions. The student may write "explain wien bridge oscillator from
	// the notes mohit sent today" → who=mohit, what="wien bridge oscillator",
	// when=today. We use what for content search and pass everything to the
	// answerer for attribution.
	who, what, when, _ := s.classifier.ParseNLPQuery(req.Question)
	if what == "" {
		what = req.Question
	}
	log.Printf("[QA] question=%q → who=%q what=%q when=%q", req.Question, who, what, when)

	// Resolve time window from `when` (same logic as retrieval handler)
	var sinceTime *time.Time
	now := time.Now()
	switch when {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		sinceTime = &t
	case "yesterday":
		t := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
		sinceTime = &t
	case "last_week", "this_week":
		t := now.AddDate(0, 0, -7)
		sinceTime = &t
	case "last_month":
		t := now.AddDate(0, -1, 0)
		sinceTime = &t
	}

	// 1. DB hit search — use ALL parsed dimensions (who/what/when) so a question
	//    like "what did mohit share about oscillators today?" filters by sender
	//    AND time AND topic, not just topic.
	relevantFiles, _ := s.store.SearchFilesNLP(groupIDs, who, what, sinceTime, 5)
	// Loosening ladder — each step drops a constraint so we don't bail to
	// "no files found" just because the keyword fallback produced junk
	// tokens. Order matters: we peel off the least-informative filter first.
	if len(relevantFiles) == 0 && who != "" {
		// WHO alone — "anything by harsh" should return harsh's files even
		// if the junk tokens ("anything") match nothing in filename/content.
		relevantFiles, _ = s.store.SearchFilesNLP(groupIDs, who, "", nil, 5)
	}
	if len(relevantFiles) == 0 {
		// Drop time/who, try content-only.
		relevantFiles, _ = s.store.SearchFilesContent(groupIDs, what, 5)
	}
	if len(relevantFiles) == 0 {
		// Last loosening: content only, no time filter.
		relevantFiles, _ = s.store.SearchFilesNLP(groupIDs, "", what, nil, 5)
	}
	if len(relevantFiles) == 0 && who == "" && strings.TrimSpace(what) != "" {
		// Very last resort: fall back to most-recent files in the user's
		// groups so vague questions ("hi", "any notes?") at least get context.
		relevantFiles, _ = s.store.GetGroupsFiles(groupIDs, 3)
	}

	// 1b. Semantic search (Reyna's Recall). Merge Qdrant hits with the
	//     metadata-ranked files so meaning-based matches surface too.
	if s.search.IsEnabled() {
		if hits, err := s.search.SearchFiles(req.Question, groupIDs, 5); err == nil && len(hits) > 0 {
			seen := map[int64]bool{}
			for _, f := range relevantFiles {
				seen[f.ID] = true
			}
			for _, h := range hits {
				if seen[h.FileID] {
					continue
				}
				if file, err := s.store.GetFileByID(h.FileID); err == nil && file != nil {
					relevantFiles = append(relevantFiles, *file)
					seen[file.ID] = true
				}
			}
		}
	}

	// 2. Drive folder walker — files organised in Drive that the bot never captured.
	driveMatches := s.collectDriveContext(groupIDs, what, 10)

	// 3. Build QA sources. For the TOP DB hit we always re-extract from the
	//    saved bytes (full-document mode) so the answer is grounded in the real
	//    document, not the truncated cached summary. Subsequent hits use cached
	//    summary to keep cost/latency sane.
	var qaSources []nlp.QASource
	var sourceNames []string
	// filesUsed mirrors qaSources for DB-backed files. Powers the unified
	// Recall UI so the frontend can render sender/timestamp/subject cards
	// alongside the answer without a second round-trip.
	var filesUsed []model.File

	for i, f := range relevantFiles {
		if i >= 3 {
			break
		}
		var content string
		if i == 0 {
			// Top hit: always re-extract the full PDF live
			if data, derr := s.drive.GetLocalFileData(f.ID); derr == nil && len(data) > 0 {
				log.Printf("[QA] full-document extract for top hit: %s (%d bytes)", f.FileName, len(data))
				extracted, summary := s.classifier.ExtractContent(f.FileName, f.MimeType, f.FileSize, data)
				if extracted != "" {
					content = extracted
					_ = s.store.UpdateFileContent(f.ID, extracted, summary)
				}
			}
		}
		// Fallback / non-top: use cached extracted_content
		if content == "" {
			if cached := s.store.GetFileExtractedContent([]int64{f.ID})[f.ID]; cached != "" {
				content = cached
			}
		}
		// Lazy fallback: cached content empty → try a one-time extract
		if content == "" {
			if data, derr := s.drive.GetLocalFileData(f.ID); derr == nil && len(data) > 0 {
				log.Printf("[QA] lazy-extracting %s (%d bytes)", f.FileName, len(data))
				extracted, summary := s.classifier.ExtractContent(f.FileName, f.MimeType, f.FileSize, data)
				if extracted != "" {
					_ = s.store.UpdateFileContent(f.ID, extracted, summary)
					content = extracted
				}
			}
		}
		if content == "" {
			continue
		}
		qaSources = append(qaSources, nlp.QASource{
			FileName:   f.FileName,
			Content:    content,
			SenderName: f.SharedByName,
			Subject:    f.Subject,
			SharedAt:   f.CreatedAt.In(istLocation),
		})
		sourceNames = append(sourceNames, f.FileName)
		filesUsed = append(filesUsed, f)
	}

	// 4. Drive-only hits: download the top 2 matching PDFs and extract live.
	if len(qaSources) < 3 {
		needed := 3 - len(qaSources)
		if needed > 2 {
			needed = 2 // hard cap to control cost/latency
		}
		driveContent := s.downloadDriveMatchesForQA(groupIDs, driveMatches, needed)
		for i, m := range driveMatches {
			if i >= needed {
				break
			}
			content := driveContent[m.FolderName+"/"+m.FileName]
			if content == "" {
				continue
			}
			qaSources = append(qaSources, nlp.QASource{
				FileName: m.FolderName + "/" + m.FileName,
				Content:  content,
				Subject:  m.FolderName,
			})
			sourceNames = append(sourceNames, m.FileName)
		}
	}

	if len(qaSources) == 0 {
		// Last resort: at least mention any drive matches by name
		if len(driveMatches) > 0 {
			var hint strings.Builder
			hint.WriteString("I couldn't read inside any notes for that, but I found these files in your Drive that might be relevant:\n")
			for i, m := range driveMatches {
				if i >= 5 {
					break
				}
				hint.WriteString(fmt.Sprintf("• %s — in %s/\n", m.FileName, m.FolderName))
			}
			json.NewEncoder(w).Encode(model.NotesQAResponse{
				Answer:       hint.String(),
				Sources:      []string{},
				DriveSources: driveMatches,
				Question:     req.Question,
			})
			return
		}
		json.NewEncoder(w).Encode(model.NotesQAResponse{
			Answer:   "I couldn't find any relevant notes to answer that question. Make sure files have been shared in your groups or organised in your Drive folder.",
			Sources:  []string{},
			Question: req.Question,
		})
		return
	}

	// ── Inject Reyna's Memory as pseudo-sources ──
	// Always-include memories go first (prepended to every Recall). Then, if
	// Qdrant is up, semantically-relevant memory chunks get added so large
	// memories (a full syllabus) only bring in the parts that matter.
	if userID > 0 {
		memSources := s.collectMemorySources(userID, req.Question)
		if len(memSources) > 0 {
			// Prepend so they appear first in the LLM's context (higher weight).
			qaSources = append(memSources, qaSources...)
			log.Printf("[QA] injected %d memory sources for user %d", len(memSources), userID)
		}
	}

	var prev *nlp.QAFollowup
	if req.PreviousQuestion != "" && req.PreviousAnswer != "" {
		prev = &nlp.QAFollowup{
			PrevQuestion: req.PreviousQuestion,
			PrevAnswer:   req.PreviousAnswer,
			PrevSources:  req.PreviousSources,
		}
		log.Printf("[QA] follow-up turn — prev question=%q", req.PreviousQuestion)
	}
	answer := s.classifier.AnswerFromNotesWithContext(req.Question, qaSources, prev)

	json.NewEncoder(w).Encode(model.NotesQAResponse{
		Answer:       answer,
		Sources:      sourceNames,
		Files:        filesUsed,
		DriveSources: driveMatches,
		Question:     req.Question,
	})
}

// ══════════════════════════════════════════
// v3: LLM STATUS
// ══════════════════════════════════════════

func (s *Server) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":  s.classifier.IsEnabled(),
		"provider": s.classifier.ProviderName(),
	})
}
