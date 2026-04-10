package reyna

import (
	"fmt"
	"strings"

	"github.com/reyna-bot/reyna-backend/internal/model"
)

// Reyna is the response engine for the bot — professional, English-only
type Reyna struct{}

func New() *Reyna { return &Reyna{} }

// ── Command Parsing ──

// ProcessCommand parses a /reyna command or natural language message into an action
func (r *Reyna) ProcessCommand(cmd string) (action string, args string) {
	cmd = strings.TrimSpace(cmd)

	// Handle slash commands (legacy support)
	if strings.HasPrefix(strings.ToLower(cmd), "/reyna") {
		parts := strings.Fields(cmd)
		if len(parts) < 2 {
			return "help", ""
		}
		action = strings.ToLower(parts[1])
		if len(parts) > 2 {
			args = strings.Join(parts[2:], " ")
			args = strings.Trim(args, "\"'")
		}
		switch action {
		case "add", "find", "log", "status", "help", "commit", "rm", "staged", "tracking", "push", "save", "search":
			// Normalize aliases
			if action == "push" {
				action = "commit"
			}
			if action == "save" {
				action = "add"
			}
			if action == "search" {
				action = "find"
			}
			return action, args
		default:
			return "unknown", cmd
		}
	}

	return "unknown", cmd
}

// ── Add / Stage Responses ──

func (r *Reyna) AddResponse(fileName string, version int, totalFiles int) string {
	if version > 1 {
		return fmt.Sprintf("Staged `%s` (v%d). %d files in your repository.", fileName, version, totalFiles)
	}
	return fmt.Sprintf("Staged `%s`. %d files in your repository.", fileName, totalFiles)
}

func (r *Reyna) AddReminder() string {
	return "\n\nUse `push` to commit to Drive, or `remove [filename]` to unstage."
}

func (r *Reyna) NotAFileResponse() string {
	return "That's not a document. I can only stage files shared in this chat — documents, PDFs, images, etc."
}

func (r *Reyna) DuplicateWarning(fileName string, count int) string {
	return fmt.Sprintf("`%s` already exists (v%d). Saved as a new version.", fileName, count+1)
}

func (r *Reyna) FileAlreadyInDriveResponse(fileName string, version int) string {
	return fmt.Sprintf("`%s` is already in Drive (v%d). Staging will create a new version.", fileName, version)
}

// ── Commit / Push Responses ──

func (r *Reyna) CommitFileResponse(fileName string) string {
	return fmt.Sprintf("Committed `%s` to Google Drive.", fileName)
}

func (r *Reyna) CommitAllResponse(count int) string {
	return fmt.Sprintf("%d file(s) committed to Google Drive.", count)
}

func (r *Reyna) CommitWithMessageResponse(count int, message string) string {
	return fmt.Sprintf("%d file(s) committed: \"%s\"", count, message)
}

func (r *Reyna) CommitEmptyResponse() string {
	return "Nothing to commit. Stage files first, then push."
}

func (r *Reyna) CommitError(fileName string) string {
	return fmt.Sprintf("Could not commit `%s`. Check if it's staged.", fileName)
}

// ── Remove Responses ──

func (r *Reyna) RmFileResponse(fileName string) string {
	return fmt.Sprintf("Removed `%s` from staging.", fileName)
}

func (r *Reyna) RmAllResponse(count int) string {
	return fmt.Sprintf("Cleared %d file(s) from staging.", count)
}

func (r *Reyna) RmEmptyResponse() string {
	return "Staging area is already empty."
}

func (r *Reyna) RmNotFoundResponse(fileName string) string {
	return fmt.Sprintf("`%s` not found in staging. It may already be committed.", fileName)
}

// ── Staged Response ──

func (r *Reyna) StagedResponse(files []model.File) string {
	if len(files) == 0 {
		return "No files staged. Share documents in the group, then stage them."
	}

	result := fmt.Sprintf("%d file(s) staged:\n", len(files))
	for i, f := range files {
		if i >= 10 {
			result += fmt.Sprintf("  ...and %d more\n", len(files)-10)
			break
		}
		folder := f.Subject
		if folder == "" {
			folder = "Unsorted"
		}
		result += fmt.Sprintf("  %s (%s) — by %s\n", f.FileName, folder, f.SharedByName)
	}
	result += "\nUse `push` to commit to Drive, or `remove [file]` to unstage."
	return result
}

// ── Find / Search Responses ──

func (r *Reyna) FindResponse(query string, files []model.File) string {
	if len(files) == 0 {
		return fmt.Sprintf("No files found for \"%s\".", query)
	}

	result := fmt.Sprintf("Found %d result(s) for \"%s\":\n", len(files), query)
	for i, f := range files {
		if i >= 5 {
			result += fmt.Sprintf("  ...and %d more\n", len(files)-5)
			break
		}
		result += fmt.Sprintf("  %s (v%d, by %s)\n", f.FileName, f.Version, f.SharedByName)
	}
	return result
}

func (r *Reyna) FindWithAuthorResponse(query string, files []model.File) string {
	if len(files) == 0 {
		return r.FindResponse(query, files)
	}

	result := fmt.Sprintf("Found %d result(s) for \"%s\":\n\n", len(files), query)
	for i, f := range files {
		if i >= 8 {
			result += fmt.Sprintf("...and %d more\n", len(files)-8)
			break
		}
		driveLink := ""
		if f.DriveFolderID != "" && !strings.HasPrefix(f.DriveFolderID, "local_") {
			driveLink = fmt.Sprintf("\n    Drive: https://drive.google.com/drive/folders/%s", f.DriveFolderID)
		}
		result += fmt.Sprintf("  *%s* (v%d)\n    By: %s | Folder: %s | Size: %s%s\n\n",
			f.FileName, f.Version, f.SharedByName, f.Subject, formatFileSize(f.FileSize), driveLink)
	}
	return result
}

// ── Log / History Responses ──

func (r *Reyna) LogResponse(files []model.File, total int) string {
	if total == 0 {
		return "No files in the repository yet."
	}

	result := fmt.Sprintf("Last %d of %d files:\n", min(len(files), 10), total)
	for i, f := range files {
		if i >= 10 {
			break
		}
		result += fmt.Sprintf("  %s — v%d by %s\n", f.FileName, f.Version, f.SharedByName)
	}
	return result
}

// ── Status Responses ──

func (r *Reyna) StatusResponse(newFiles []model.File, total int) string {
	if len(newFiles) == 0 {
		return fmt.Sprintf("No new files. %d file(s) total in the repository.", total)
	}

	subjects := make(map[string]int)
	for _, f := range newFiles {
		sub := f.Subject
		if sub == "" {
			sub = "Unsorted"
		}
		subjects[sub]++
	}

	result := fmt.Sprintf("%d new file(s):\n", len(newFiles))
	for sub, cnt := range subjects {
		result += fmt.Sprintf("  %s: %d\n", sub, cnt)
	}
	result += fmt.Sprintf("\n%d files total.", total)
	return result
}

func (r *Reyna) StatusResponseGit(staged []model.File, untracked []string, committed int, total int) string {
	if len(staged) == 0 && len(untracked) == 0 && total > 0 {
		return fmt.Sprintf("All clear. %d file(s) committed. Nothing pending.", total)
	}

	result := "Status:\n"
	if len(staged) > 0 {
		result += fmt.Sprintf("\nStaged: %d file(s)\n", len(staged))
		for i, f := range staged {
			if i >= 8 {
				result += fmt.Sprintf("  ...and %d more\n", len(staged)-8)
				break
			}
			result += fmt.Sprintf("  %s\n", f.FileName)
		}
		result += "\nUse `push` to commit to Drive.\n"
	}

	if len(untracked) > 0 {
		result += fmt.Sprintf("\nUntracked: %d file(s)\n", len(untracked))
		for i, name := range untracked {
			if i >= 8 {
				result += fmt.Sprintf("  ...and %d more\n", len(untracked)-8)
				break
			}
			result += fmt.Sprintf("  %s\n", name)
		}
		result += "\nReact with 📌 to stage, or say `reyna save` to stage all.\n"
	}

	if len(staged) == 0 && len(untracked) == 0 {
		result += "Nothing to commit. Share some files first."
	}

	return result
}

// ── Help Response ──

func (r *Reyna) HelpResponse() string {
	return `Reyna — your group's file manager.

Getting started:
  • /reyna init — activate Reyna in this group

How to save files:
  • Share a file in the group → React with 📌 to stage it
  • Or say: "reyna save" to stage recent files

How to search:
  • "reyna find DSA notes"
  • "reyna [topic]" — e.g. "reyna operating systems"

Other commands:
  • "reyna push" — commit staged files to Google Drive
  • "reyna status" — see what's new
  • "reyna history" — recent file log

Manage everything on your dashboard.`
}

// ── Generic / Unknown Responses ──

func (r *Reyna) GenericResponse() string {
	return "I didn't understand that. Say \"reyna help\" to see what I can do."
}

func (r *Reyna) InvalidCommandResponse(attempted string) string {
	valid := []string{"save", "push", "remove", "find", "history", "status", "staged", "help"}
	best := ""
	bestScore := 0
	attemptedLower := strings.ToLower(attempted)
	for _, v := range valid {
		score := 0
		for i := 0; i < len(attemptedLower) && i < len(v); i++ {
			if attemptedLower[i] == v[i] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = v
		}
	}
	if best != "" && bestScore > 1 {
		return fmt.Sprintf("Unknown command \"%s\". Did you mean \"%s\"?", attempted, best)
	}
	return fmt.Sprintf("Unknown command \"%s\". Say \"reyna help\" for available commands.", attempted)
}

// ── Natural Language Responses (for intents detected by NLP) ──

func (r *Reyna) NLPSearchResponse(query string, count int) string {
	if count == 0 {
		return fmt.Sprintf("No files found for \"%s\". Check your dashboard for the full archive.", query)
	}
	return fmt.Sprintf("Found %d file(s) matching \"%s\". Check your dashboard for details.", count, query)
}

func (r *Reyna) NLPSaveResponse(count int) string {
	if count == 0 {
		return "No new files to stage. Share documents first."
	}
	return fmt.Sprintf("Staged %d file(s). They'll auto-commit to Drive in 24 hours, or say \"reyna push\" now.", count)
}

func (r *Reyna) NLPPushResponse(count int, uploaded int) string {
	if count == 0 {
		return "Nothing staged to push. Stage files first."
	}
	msg := fmt.Sprintf("%d file(s) committed.", count)
	if uploaded > 0 {
		msg += fmt.Sprintf(" %d pushed to Google Drive.", uploaded)
	}
	return msg
}

func (r *Reyna) NLPStatusResponse(staged int, total int) string {
	return fmt.Sprintf("%d staged, %d total files. Check your dashboard for details.", staged, total)
}

func (r *Reyna) NLPHistoryResponse(count int) string {
	if count == 0 {
		return "No files in the repository yet."
	}
	return fmt.Sprintf("%d files in your repository. View the full history on your dashboard.", count)
}

// ── Reaction Tracking Response ──

func (r *Reyna) ReactionStagedResponse(fileName string) string {
	return fmt.Sprintf("Staged `%s`. It will auto-commit to Drive in 24 hours.", fileName)
}

// ── Helpers ──

func formatFileSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1048576 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/1048576)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
