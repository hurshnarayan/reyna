package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/reyna-bot/reyna-backend/internal/auth"
	"github.com/reyna-bot/reyna-backend/internal/config"
	"github.com/reyna-bot/reyna-backend/internal/db"
	"github.com/reyna-bot/reyna-backend/internal/gdrive"
	"github.com/reyna-bot/reyna-backend/internal/models"
	"github.com/reyna-bot/reyna-backend/internal/nlp"
	"github.com/reyna-bot/reyna-backend/internal/reyna"
)

type Server struct {
	cfg        *config.Config
	store      *db.Store
	drive      *gdrive.Service
	reyna      *reyna.Reyna
	classifier *nlp.Classifier
	mux        *http.ServeMux
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

func NewServer(cfg *config.Config, store *db.Store, drive *gdrive.Service, classifier *nlp.Classifier) *Server {
	s := &Server{cfg: cfg, store: store, drive: drive, reyna: reyna.New(), classifier: classifier, mux: http.NewServeMux()}
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

	// v3 — NLP Retrieval + Q&A + LLM status
	// retrieve and qa use wrap (not protected) so bot can call them too,
	// but they try JWT first for dashboard calls
	s.mux.HandleFunc("/api/nlp/retrieve", wrap(s.handleNLPRetrieve))
	s.mux.HandleFunc("/api/nlp/qa", wrap(s.handleNotesQA))
	s.mux.HandleFunc("/api/llm/status", wrap(s.handleLLMStatus))
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
	json.NewEncoder(w).Encode(models.AuthResponse{Token: token, User: user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req models.LoginRequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Phone == "" { http.Error(w, `{"error":"phone required"}`, 400); return }
	user, err := s.store.GetUserByPhone(req.Phone)
	if err != nil { http.Error(w, `{"error":"user not found, register first"}`, 404); return }
	s.store.AutoLinkUserToGroups(user.ID, req.Phone)
	token, _ := auth.GenerateToken(user.ID, s.cfg.JWTSecret)
	json.NewEncoder(w).Encode(models.AuthResponse{Token: token, User: user})
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
	if groups == nil { groups = []models.Group{} }
	json.NewEncoder(w).Encode(groups)
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

	var files []models.File
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
	if files == nil { files = []models.File{} }
	json.NewEncoder(w).Encode(files)
}


func (s *Server) handleFileVersions(w http.ResponseWriter, r *http.Request) {
	fid, _ := strconv.ParseInt(r.URL.Query().Get("file_id"), 10, 64)
	versions, _ := s.store.GetFileVersions(fid)
	if versions == nil { versions = []models.FileVersion{} }
	json.NewEncoder(w).Encode(versions)
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)
	var req models.AddFileRequest
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
	file := &models.File{GroupID: group.ID, UserID: uid, SharedByPhone: req.SharedByPhone, SharedByName: req.SharedByName, FileName: req.FileName, FileSize: req.FileSize, MimeType: req.MimeType, DriveFileID: driveID, DriveFolderID: folderID, Subject: req.Subject, WAMessageID: req.WAMessageID}
	saved, _ := s.store.AddFile(file)
	json.NewEncoder(w).Encode(saved)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	gid, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	logs, _ := s.store.GetActivityLog(gid, 50)
	if logs == nil { logs = []models.ActivityLog{} }
	json.NewEncoder(w).Encode(logs)
}

// ══════════════════════════════════════════
// BOT COMMAND HANDLER
// ══════════════════════════════════════════

func (s *Server) handleBotCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req models.CommandRequest
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

	resp := models.CommandResponse{}

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
		file := &models.File{GroupID: groupID, UserID: user.ID, SharedByPhone: req.UserPhone, SharedByName: req.UserName, FileName: fileName, FileSize: req.FileSize, MimeType: req.MimeType, Subject: subject, DriveFileID: fmt.Sprintf("meta_%d", time.Now().UnixNano()), Status: "staged"}
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
		if staged == nil { staged = []models.File{} }
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
		if files == nil { files = []models.File{} }
		resp.Files = files
		if showAuthor {
			resp.Reply = s.reyna.FindWithAuthorResponse(query, files)
		} else {
			resp.Reply = s.reyna.FindResponse(query, files)
		}

	case "log":
		files, _ := s.store.GetGroupFiles(groupID, 10)
		if files == nil { files = []models.File{} }
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

	// NLP classification if subject not provided
	// v3: Use combined extract+classify for PDFs (collapses Agents 1 & 2)
	var extractedContent, contentSummary string
	if subject == "" {
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
		// Try combined extract+classify in one LLM call (PDF only)
		groupName := ""
		if group != nil {
			groupName = group.Name
		}
		fmeta := nlp.FileMeta{
			SenderName:  sharedByName,
			SenderPhone: userPhone,
			GroupName:   groupName,
			SharedAt:    time.Now(),
		}
		if strings.Contains(mimeType, "pdf") && len(fileBytes) > 0 {
			subject, _, _, extractedContent, contentSummary = s.classifier.ClassifyFileWithContent(fileName, mimeType, fileBytes, existingFolders, fmeta)
		} else {
			subject, _, _ = s.classifier.ClassifyFile(fileName, existingFolders)
		}
		log.Printf("[NLP] Classified %s → %s", fileName, subject)
	}

	// Save locally
	localID, folderID, _ := s.drive.SmartUpload("", "", user.ID, subject, fileName, mimeType, fileBytes)
	// Save raw bytes for later Drive upload on commit
	dbFile := &models.File{GroupID: groupID, UserID: user.ID, SharedByPhone: userPhone, SharedByName: sharedByName, FileName: fileName, FileSize: fileSize, MimeType: mimeType, Subject: subject, DriveFileID: localID, DriveFolderID: folderID, Status: "staged"}
	saved, err := s.store.AddFile(dbFile)
	if err != nil { log.Printf("❌ DB: %v", err); http.Error(w, `{"error":"db save failed"}`, 500); return }

	s.drive.SaveLocalFileData(saved.ID, fileBytes)
	log.Printf("✅ Saved file %d: %s (%d bytes) — staged", saved.ID, fileName, len(fileBytes))

	// v3: Content extraction — if combined call already extracted, save it now.
	// Otherwise run async extraction with file data as document blocks.
	if extractedContent != "" {
		s.store.UpdateFileContent(saved.ID, extractedContent, contentSummary)
		log.Printf("[EXTRACT] Combined: %s → %s", fileName, contentSummary)
	} else {
		go func(fileID int64, fName, mime string, fSize int64, data []byte) {
			content, summary := s.classifier.ExtractContent(fName, mime, fSize, data)
			if content != "" {
				s.store.UpdateFileContent(fileID, content, summary)
				log.Printf("[EXTRACT] Async: %s → %s", fName, summary)
			}
		}(saved.ID, fileName, mimeType, fileSize, fileBytes)
	}

	total := s.store.CountGroupFiles(groupID)
	v := 1; if saved != nil { v = saved.Version }
	reply := s.reyna.AddResponse(fileName, v, total)
	s.store.LogActivity(groupID, user.ID, "add", "/reyna add "+fileName, "staged")

	json.NewEncoder(w).Encode(map[string]interface{}{"reply": reply, "file_id": saved.ID, "status": "staged"})
}

// ── Waitlist ──
func (s *Server) handleWaitlist(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req models.WaitlistRequest
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
func (s *Server) handleCommitStaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	uid := auth.GetUserID(r)

	gids := s.store.GetUserGroupIDs(uid)
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
				for _, f := range staged {
					folderID, ferr := s.drive.EnsureSubjectFolder(token, driveUser.DriveRootID, f.Subject)
					if ferr != nil { continue }
					fileData, _ := s.drive.GetLocalFileData(f.ID)
					if len(fileData) == 0 { fileData, _ = s.drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName) }
					if len(fileData) == 0 { continue }
					driveID, uerr := s.drive.UploadFileToDrive(token, folderID, f.FileName, f.MimeType, fileData)
					if uerr == nil && driveID != "" {
						s.store.UpdateFileDriveID(f.ID, driveID, folderID)
						totalUploaded++
					}
				}
			}
		}

		count, _ := s.store.CommitFiles(gid)
		totalCommitted += count
	}

	log.Printf("[DASHBOARD-COMMIT] user=%d committed=%d uploaded=%d", uid, totalCommitted, totalUploaded)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"committed": totalCommitted,
		"uploaded":  totalUploaded,
	})
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
		if contentQuery == "" { json.NewEncoder(w).Encode([]models.File{}); return }
		// Search inside file contents from local storage
		var matches []models.File
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
		if matches == nil { matches = []models.File{} }
		json.NewEncoder(w).Encode(matches)
		return
	}

	// Strict filename search
	files, _ := s.store.FindFilesStrict(gids, q, 20)
	if files == nil { files = []models.File{} }
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
			gs := s.store.GetGroupSettings(g.ID)
			result = append(result, map[string]interface{}{
				"group":    g,
				"settings": gs,
			})
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

	var req models.ReactionRequest
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

	file := &models.File{
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
	if req.Name == "" { req.Name = "WhatsApp Group" }

	// Try to get existing group first, create if not exists
	group, err := s.store.GetGroupByWAID(req.WAID)
	if err != nil || group == nil {
		// Insert directly without foreign key on created_by
		s.store.InsertGroupRaw(req.WAID, req.Name, req.MemberCount)
		group, _ = s.store.GetGroupByWAID(req.WAID)
	}

	if group != nil {
		// Always update name if we have a real one
		if req.Name != "" && req.Name != "WhatsApp Group" {
			s.store.UpdateGroupName(group.ID, req.Name)
			log.Printf("[SYNC-GROUP] Updated group %d name: %q → %q", group.ID, group.Name, req.Name)
		}
		if req.MemberCount > 0 {
			s.store.UpdateGroupMemberCount(group.ID, req.MemberCount)
		}
		// Auto-enable group settings (create if not exists, enable if disabled)
		gs := s.store.GetGroupSettings(group.ID)
		if !gs.Enabled {
			gs.Enabled = true
			s.store.UpsertGroupSettings(gs)
			log.Printf("Auto-enabled group %d (%s) via bot sync", group.ID, req.Name)
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
	var req models.NLPRetrievalRequest
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
		}
		// Fallback: if from bot (no JWT), get groups from phone
		if len(groupIDs) == 0 && req.UserPhone != "" {
			user, err := s.store.GetUserByPhone(req.UserPhone)
			if err == nil {
				groupIDs = s.store.GetUserGroupIDs(user.ID)
			}
		}
	}
	log.Printf("[NLP-RETRIEVE] groupIDs=%v", groupIDs)

	files, _ := s.store.SearchFilesNLP(groupIDs, who, what, sinceTime, 20)
	if files == nil {
		files = []models.File{}
	}

	// Build human-readable reply
	reply := s.buildNLPReply(files, who, what, when)

	json.NewEncoder(w).Encode(models.NLPRetrievalResponse{
		Files: files,
		Query: models.NLPParsedQuery{Who: who, What: what, When: when, Why: why, Raw: req.Query},
		Reply: reply,
	})
}

func (s *Server) buildNLPReply(files []models.File, who, what, when string) string {
	if len(files) == 0 {
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
	return msg
}

// ══════════════════════════════════════════
// v3: NOTES Q&A
// ══════════════════════════════════════════

func (s *Server) handleNotesQA(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req models.NotesQARequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Question == "" {
		http.Error(w, `{"error":"question required"}`, 400)
		return
	}

	// Get group IDs
	var groupIDs []int64
	if req.GroupWAID != "" {
		group, err := s.store.GetGroupByWAID(req.GroupWAID)
		if err == nil {
			groupIDs = []int64{group.ID}
		}
	}
	if len(groupIDs) == 0 {
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
		}
		if len(groupIDs) == 0 && req.UserPhone != "" {
			user, err := s.store.GetUserByPhone(req.UserPhone)
			if err == nil {
				groupIDs = s.store.GetUserGroupIDs(user.ID)
			}
		}
	}

	// Search for relevant files by content
	relevantFiles, _ := s.store.SearchFilesContent(groupIDs, req.Question, 5)

	// Also try a broader search by filename/subject
	if len(relevantFiles) == 0 {
		relevantFiles, _ = s.store.SearchFilesNLP(groupIDs, "", req.Question, nil, 5)
	}

	if len(relevantFiles) == 0 {
		json.NewEncoder(w).Encode(models.NotesQAResponse{
			Answer:   "I couldn't find any relevant notes to answer that question. Make sure files have been shared in your groups.",
			Sources:  []string{},
			Question: req.Question,
		})
		return
	}

	// Gather file contents
	var fileIDs []int64
	for _, f := range relevantFiles {
		fileIDs = append(fileIDs, f.ID)
	}
	contentMap := s.store.GetFileExtractedContent(fileIDs)

	// Build content for LLM
	fileContents := make(map[string]string)
	var sources []string
	for _, f := range relevantFiles {
		content := contentMap[f.ID]
		if content == "" {
			content = fmt.Sprintf("File: %s, Subject: %s, Size: %d bytes, Shared by: %s",
				f.FileName, f.Subject, f.FileSize, f.SharedByName)
		}
		fileContents[f.FileName] = content
		sources = append(sources, f.FileName)
	}

	answer := s.classifier.AnswerFromNotes(req.Question, fileContents)

	json.NewEncoder(w).Encode(models.NotesQAResponse{
		Answer:   answer,
		Sources:  sources,
		Question: req.Question,
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
