# REYNA AI SYSTEM HANDOFF PROMPT

> **Copy and paste everything below into your AI coding assistant (Gemini CLI / Claude Code / Cursor / Windsurf / ChatGPT) at the start of any new session.**

```markdown
You are working on Reyna, an intelligent zero-upload document and attribution system built for Smart India Hackathon 2026 (Problem Statement ID: SIH260150, Theme: Smart Education).

================================================================================
1. CRITICAL WORKSPACE & SCOPE DIRECTIVE (READ THIS FIRST):
================================================================================
1. ACTIVE CODEBASE LOCATION:
   All active work takes place in:
   `/Users/harsh/code/reyna-app` (or your local clone of `reyna-app`)
   There is a legacy folder at `/Users/harsh/code/reyna` (v0.1 prototype). DO NOT USE IT.

2. THE WEB BOT IS OBSOLETE AND NOT PART OF OUR APP:
   - The Baileys WhatsApp bot and the React web dashboard are DEAD/OBSOLETE.
   - Our production app consists of:
     (A) A native Android client (Kotlin + Jetpack Compose + Room SQLite) in `android/`
     (B) An ultra-lightweight Go backend server (Go 1.22+ with only 2 external dependencies) in `cmd/server/` and `internal/`
   - Do NOT suggest, write, or touch any WhatsApp bot or web bot code.

================================================================================
2. WHAT REYNA IS IN PLAIN ENGLISH:
================================================================================
In 46,000+ colleges across India, semester study materials, project reports, and PYQs are shared in WhatsApp groups and immediately buried under chat.
Traditional college portals ask students to manually upload files. Nobody uploads, and platforms die.

Reyna eliminates the upload step entirely:
1. NO UPLOAD: An Android background service (FileObserver) watches the folder WhatsApp already writes files to (/Android/media/com.whatsapp/...).
2. NO BOTS IN GROUPS: No bot phone number joins chats. WhatsApp cannot ban anyone.
3. SOVEREIGN DRIVE BACKUP: As files land, Reyna files them directly into the student's personal Google Drive (/Reyna/<Subject>/...). Documents NEVER live on central servers.
4. 4-SIGNAL ATTRIBUTION: A file on disk carries no sender tag. Reyna reconstructs who shared it using 4 signals (timestamps, naming formats, /Sent/ folder, chat exports) with mathematical confidence.
5. CONVERSATIONAL RETRIEVAL: Students can ask in any language: "Where are the notes for Module 1 ODE?" Reyna answers cleanly, cites the file, names the verified sender, and renders a clickable "Sources" button.

================================================================================
3. RECENT SESSION SUMMARY & ACCOMPLISHMENTS:
================================================================================
During our most recent engineering session, we resolved critical chat retrieval, context retention, and tokenization bugs:

A. Multi-Turn Conversational Memory & Anaphora Resolution:
   - Problem: Reyna had zero conversational memory. Asking follow-up queries like "can you find it?", "what does it say?", or "who sent it?" failed because the backend received only the isolated query string and asked the user to re-specify the file.
   - Solution:
     * Android Client: `ReynaDao.recentMessages(limit)` fetches recent dialogue turns and decoded citations. `ReynaApi.ask()` and `Repo.ask()` serialize `List<ChatContext>` into `NLPRetrievalRequest.History`.
     * Backend: `internal/nlp/classifier.go` implements `ParseNLPQueryWithHistory()` and `llmParseQueryWithHistory()`. The prompt explicitly resolves anaphora/pronouns ("it", "that", "that file") to previous topics/filenames.
     * `GenerateRetrievalReply()` consumes `history` so Gemini generates natural multi-turn answers and quotes source evidence.

B. Reliable Sources Button & Citation Verification:
   - Problem: The `[ 1 source ]` button was missing on metadata-only matches, or overwritten by a noisy client-side fallback ("Found on your phone: file1, file2...").
   - Solution: Removed the destructive fallback in `Repo.kt`. Added positive-reply citation fallback in `handleNLPRetrieve` (`internal/api/handlers.go`) ensuring positive matches always attach citations and render the `[ 1 source ]` button. Tapping it opens the "Where this came from" bottom sheet with exact metadata and "Open" action.

C. Whole-Word Tokenization & Domain Acronym Preservation:
   - Problem: Short programming and domain acronyms (e.g., "c", "os", "ai", "db") were either stripped as noise or matched as arbitrary substrings (`LIKE '%c%'`), matching unrelated files.
   - Solution: `SearchFilesNLP` in `internal/repository/store.go` now grants exact phrase boosts (+60), protects 1-2 character acronyms in `TokenizeWhat()`, and enforces word-boundary checks (`tokenMatchesWord`) across DB and Drive search.

D. Fresh APK Builds:
   - Freshly generated APKs are placed in:
     `/Users/harsh/Downloads/Reyna_APKs/`
     * `reyna.apk` / `reyna-release.apk` (42 MB)
     * `reyna-debug.apk` (56 MB)

================================================================================
4. GIT BRANCH TOPOLOGY & ACTIVE BRANCHES:
================================================================================
- `fix/search-and-chat-retrieval` (contains commit `4c82c62`):
  Contains the multi-turn context parsing, anaphora resolution, citation bottom sheet fixes, tokenization acronym preservation, and Drive matching.
- `main` (contains commit `348712f`):
  Contains UI polish (Reyna mark, amber notice cards, quota wall handling, fixed backend address, and local instructions).
- Handoff Action: Merge `fix/search-and-chat-retrieval` into `main` cleanly when ready.

================================================================================
5. KEY DIRECTORY & CODE MAP (Inside `reyna-app`):
================================================================================
- `cmd/server/main.go`            → Entrypoint, background queue reader
- `internal/api/handlers.go`      → HTTP endpoints (/api/v1/stage, /api/nlp/retrieve, /api/device/drive/*)
- `internal/attribution/`         → 4-Signal attribution scoring engine
- `internal/nlp/`                 → Query parsing (strip.go), multi-turn history parsing (classifier.go), prompts
- `internal/docs/`                → Zero-cost pure Go local Office parser (.docx, .pptx, .xlsx)
- `internal/relevance/`           → Whole-word BM25-style document relevance ranking
- `internal/repository/`          → Pure Go SQLite store (no ORM, mattn/go-sqlite3)
- `android/app/src/main/java/app/reyna/`
    ├── `data/Repo.kt`            → File capture, upload queue, ask() multi-turn retrieval
    ├── `data/Db.kt`              → Room Database schema, migrations, recentMessages query
    ├── `data/Words.kt`           → Client-side whole-word tokenizer (mirrors server)
    ├── `net/ReynaApi.kt`         → Retrofit/OkHttp client with ChatContext history serialization
    └── `ui/`                     → Jetpack Compose UI (Chat, Sources sheet, Notice cards, ReynaViewModel)
- `justfile`                      → Single source of truth for all build/run/test commands

================================================================================
6. DEVELOPMENT PLAYBOOK & JUST COMMANDS:
================================================================================
All workflow is automated via `just`:
- `just backend-bg`     → Build & start Go backend detached on :8080 (survives shell)
- `just backend-stop`   → Stop backend safely using `-sTCP:LISTEN` (never kills tunnels)
- `just tunnel-up`      → Expose backend via permanent ngrok domain (`superformally-reckonable-etha.ngrok-free.dev`)
- `just check`          → Run `go build ./... && go vet ./... && go test ./...`
- `just build`          → Assemble Android debug APK
- `just apk-release`    → Build signed production APK to ~/Desktop/reyna.apk
- `just logs`           → Stream Android logcat filtered for Reyna
- `just ask "<query>"`  → Test retrieval from the terminal

Emulator Setup:
- AVD `reyna_pixel` (arm64, API 35) is pre-configured.
- SDK dir: `/opt/homebrew/share/android-commandlinetools`
- Launch headless: `emulator -avd reyna_pixel -no-snapshot-load -gpu swiftshader_indirect &`
- Point app at emulator: `just point-at-emulator && just build`
- Point app at tunnel: `just point-at <tunnel-url>` before building release

================================================================================
7. THE TEN INVARIANTS (NEVER BREAK OR REGRESS THESE):
================================================================================
1. STRICT 0.70 CONFIDENCE FLOOR:
   Never allow any classifier or prompt change to name an individual sender below 0.70 confidence.
   - >= 0.70: 🟢 Name person ("Priya · 18 August")
   - 0.30 - 0.69: 🟠 Name chat ("Sem 5 CS · 18 August")
   - < 0.30: ⚪ Local phone ("Found on your phone · 18 August")
   Rule: A wrong name is infinitely worse than no name.

2. WHOLE-WORD MATCHING ONLY:
   Substring matching is forbidden. "ode" must NOT match "diode", and "c" must not match random files.
   Filename matching in `internal/relevance` and `Words.kt` splits at non-alphanumeric and case/number boundaries.

3. DEVICE IDENTITY IS NOT A SENDER:
   Uploads from Android carry the internal identity `device`. The device identity must NEVER be promoted to a human sender or stamped as "(by device)".

4. ZERO API COST FOR OFFICE FILES:
   `.docx`, `.pptx`, and `.xlsx` files are parsed locally on the machine via pure Go (`archive/zip` + `encoding/xml` in `internal/docs`). NEVER route Office docs to an LLM.

5. 0.03s FAST-FAIL QUOTA WALL:
   Gemini free tier has 20 calls/day/model (`GEMINI_MODEL` list). When daily quota runs out, `Classifier.OutOfAllowanceUntil` immediately returns an amber Notice card naming the reset time (midnight Pacific, ~12:30 IST). Do NOT let queries hang in rate-limiting queues.

6. NOTICES ARE CARDS, NOT TEXT:
   System notices (like quota exhaustion) are returned in the `Notice` field and rendered as amber cards. Never synthesize a fake directory listing as conversational prose.

7. EXPLICIT ROOM MIGRATIONS ONLY:
   Never use `fallbackToDestructiveMigration` alone in `Db.kt`. Every schema update requires an explicit migration (e.g., `MIGRATION_2_3`), or it wipes the user's entire local document library.

8. DISTINGUISH ErrNotAttempted FROM UnreadableSentinel:
   If a document read fails due to quota or network, return `ErrNotAttempted`. Only write `UnreadableSentinel` if the file was genuinely opened and contained zero text.

9. NON-DESTRUCTIVE PHRASE STRIPPING:
   `stripPhrases` in `internal/nlp/strip.go` must match whole phrases longest-first. Naive replacement that turns "meant" into "ant" or "theory" into "ory" is banned.

10. SMALLTALK PRECEDES RETRIEVAL:
    `nlp.IsSmallTalk` must be evaluated before document search and before ambiguity branching so greetings like "hi" or "thanks" never consume document search quota.
```
