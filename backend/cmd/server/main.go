package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reyna-bot/reyna-backend/internal/api"
	"github.com/reyna-bot/reyna-backend/internal/config"
	"github.com/reyna-bot/reyna-backend/internal/db"
	"github.com/reyna-bot/reyna-backend/internal/gdrive"
	"github.com/reyna-bot/reyna-backend/internal/llm"
	"github.com/reyna-bot/reyna-backend/internal/nlp"
)

func main() {
	cfg := config.Load()

	// Initialize database
	store, err := db.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.Close()

	// Initialize Drive service
	drive := gdrive.New(cfg.GoogleClientID, cfg.GoogleSecret, cfg.GoogleRedirectURL, "./drive_storage")

	// Initialize LLM provider (swappable: claude / gemini / grok)
	llmProvider := llm.New(cfg.LLMProvider, cfg.LLMAPIKey())

	// Initialize NLP classifier with LLM provider
	classifier := nlp.New(llmProvider)
	if classifier.IsEnabled() {
		log.Printf("   LLM: %s", llmProvider.Name())
	} else {
		log.Printf("   LLM: keyword-only (set ANTHROPIC_API_KEY, GEMINI_API_KEY, or XAI_API_KEY)")
	}

	// Initialize API server
	server := api.NewServer(cfg, store, drive, classifier)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Reyna backend starting on %s", addr)
	log.Printf("   Frontend URL: %s", cfg.FrontendURL)
	log.Printf("   Database: %s", cfg.DatabaseURL)
	log.Printf("   Auto-commit: %d hours", cfg.AutoCommitHours)

	// ── Auto-Commit Scheduler ──
	// Runs every 15 minutes, commits staged files older than AutoCommitHours
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			expired, err := store.GetStagedFilesOlderThan(cfg.AutoCommitHours)
			if err != nil || len(expired) == 0 {
				continue
			}
			log.Printf("[AUTO-COMMIT] Found %d expired staged files", len(expired))

			// Group by group_id for batch processing
			groups := make(map[int64][]int64)
			for _, f := range expired {
				groups[f.GroupID] = append(groups[f.GroupID], f.ID)
			}

			for gid := range groups {
				driveUser := store.FindDriveConnectedUser(gid)
				if driveUser == nil || driveUser.GoogleRefresh == "" || driveUser.DriveRootID == "" {
					// No drive user — just mark as committed locally
					store.CommitFiles(gid)
					log.Printf("[AUTO-COMMIT] Committed group %d locally (no Drive user)", gid)
					continue
				}

				token, terr := drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
				if terr != nil {
					store.CommitFiles(gid)
					log.Printf("[AUTO-COMMIT] Committed group %d locally (token error)", gid)
					continue
				}
				store.UpdateUserGoogle(driveUser.ID, driveUser.Email, token, driveUser.GoogleRefresh, driveUser.DriveRootID)

				staged, _ := store.GetStagedFilesByGroupID(gid)
				uploaded := 0
				for _, f := range staged {
					folderID, ferr := drive.EnsureSubjectFolder(token, driveUser.DriveRootID, f.Subject)
					if ferr != nil {
						continue
					}
					fileData, _ := drive.GetLocalFileData(f.ID)
					if len(fileData) == 0 {
						fileData, _ = drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName)
					}
					if len(fileData) == 0 {
						continue
					}
					driveID, uerr := drive.UploadFileToDrive(token, folderID, f.FileName, f.MimeType, fileData)
					if uerr == nil && driveID != "" {
						store.UpdateFileDriveID(f.ID, driveID, folderID)
						uploaded++
					}
				}

				count, _ := store.CommitFiles(gid)
				log.Printf("[AUTO-COMMIT] Group %d: committed %d, uploaded %d to Drive", gid, count, uploaded)
			}
		}
	}()

	// Graceful shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("Shutting down...")
		store.Close()
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

