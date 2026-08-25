package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hurshnarayan/reyna/internal/api"
	"github.com/hurshnarayan/reyna/internal/config"
	"github.com/hurshnarayan/reyna/internal/integrations/gdrive"
	"github.com/hurshnarayan/reyna/internal/integrations/llm"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/nlp"
	"github.com/hurshnarayan/reyna/internal/repository"
)

// unreadableSentinel marks a file whose bytes are gone, so the backfill queue
// does not offer it again on every pass.
const unreadableSentinel = repository.UnreadableSentinel

const (
	// backfillDailyReads is how much of the daily model allowance the
	// background reader may spend. The rest is kept for questions.
	backfillDailyReads = 8

	// quietBefore is how long after a question the background reader waits
	// before touching the model again.
	quietBefore = 10 * time.Minute
)

func main() {
	cfg := config.Load()

	// Initialize database
	store, err := repository.New(cfg.DatabaseURL)
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

	if cfg.DeviceTokenIsTemp {
		log.Printf("")
		log.Printf("   ⚠  DEVICE_TOKEN is not set. Generated a temporary one for this process:")
		log.Printf("")
		log.Printf("        DEVICE_TOKEN=%s", cfg.DeviceToken)
		log.Printf("")
		log.Printf("   The bot cannot authenticate until this is in your .env. It changes on")
		log.Printf("   every restart, so add it there rather than copying it each time.")
		log.Printf("")
	}

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

	// Read the documents that were stored but never read.
	//
	// Files arrive faster than a rate limited model can read them, so a phone
	// uploading a backlog leaves most of them with no extracted text and
	// therefore findable only by filename. This walks that queue slowly and
	// forever, behind the rate gate, so the library becomes searchable over
	// time instead of never. One at a time on purpose: a question asked right
	// now should not queue behind a hundred documents.
	go func() {
		// Backs off hard when the model refuses.
		//
		// On a free tier capped per day, a worker that retries every twenty
		// seconds spends the entire day's allowance within minutes of it
		// resetting, and it does so at four in the morning when nobody is
		// asking anything. The user then finds no quota left for the one
		// question they actually wanted answered. A refusal now means an hour
		// of silence, which leaves the allowance for interactive use.
		backoff := 2 * time.Minute
		spentToday, day := 0, time.Now().YearDay()
		for {
			time.Sleep(backoff)

			// The daily allowance is small and shared. Most of it is held back
			// for questions, because a document read because somebody asked
			// about it is worth several read because it happened to be next in
			// a queue. Without this the worker spent the whole day's calls
			// within minutes of midnight and every question after that was
			// answered from filenames alone.
			if d := time.Now().YearDay(); d != day {
				day, spentToday = d, 0
			}
			if spentToday >= backfillDailyReads {
				continue
			}

			// Never read while somebody is mid-conversation. Their question
			// gets the next call.
			if api.SinceLastQuestion() < quietBefore {
				continue
			}

			pending, err := store.FilesMissingContent(20)
			if err != nil || len(pending) == 0 {
				continue
			}
			var f *model.File
			for i := range pending {
				if api.ReadableForExtraction(pending[i].MimeType, pending[i].FileName) {
					f = &pending[i]
					break
				}
				// Nothing in here to read. Retire it so the queue moves on
				// instead of offering the same photograph forever.
				store.UpdateFileContent(pending[i].ID, unreadableSentinel, "")
			}
			if f == nil {
				continue
			}
			data, derr := drive.GetLocalFileData(f.ID)
			if derr != nil || len(data) == 0 {
				// Nothing to read. Marked with a sentinel rather than an empty
				// string, because the queue selects on empty and writing empty
				// would hand back the same unreadable file forever.
				store.UpdateFileContent(f.ID, unreadableSentinel, "")
				continue
			}
			subject, _, _, content, summary := classifier.ClassifyFileWithContent(
				f.FileName, f.MimeType, data, nil, nlp.FileMeta{},
			)
			if content == "" && summary == "" {
				// Almost always a refused call rather than an empty document.
				if backoff < time.Hour {
					backoff = time.Hour
				}
				continue
			}
			backoff = 2 * time.Minute
			spentToday++
			store.UpdateFileContent(f.ID, content, summary)
			if subject != "" && (f.Subject == "" || f.Subject == "Uncategorized" || f.Subject == "Documents") {
				store.UpdateFileSubject(f.ID, subject)
			}
			log.Printf("[BACKFILL] read %s (%d chars)", f.FileName, len(content))
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
