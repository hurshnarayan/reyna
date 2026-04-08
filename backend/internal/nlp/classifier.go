package nlp

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/reyna-bot/reyna-backend/internal/llm"
)

// geminiInlineMaxBytes is the safe ceiling for inline base64-encoded file data
// in a single Gemini generateContent request. Gemini's documented inline limit
// is ~20 MB total request size; we leave headroom for prompt + base64 overhead.
const geminiInlineMaxBytes = 14 * 1024 * 1024

// Classifier handles NLP-based file classification and intent detection
type Classifier struct {
	llm llm.Provider
}

// New creates a new NLP classifier with the given LLM provider.
// If provider is nil or disabled, falls back to keyword matching.
func New(provider llm.Provider) *Classifier {
	return &Classifier{llm: provider}
}

// IsEnabled returns true if an LLM provider is configured and active
func (c *Classifier) IsEnabled() bool {
	return c.llm != nil && c.llm.IsEnabled()
}

// ProviderName returns the active LLM provider name
func (c *Classifier) ProviderName() string {
	if c.llm == nil {
		return "none"
	}
	return c.llm.Name()
}

// ── Folder Classification ──

// ClassifyFile determines the best folder for a file based on its name.
// Folder Priority Logic (from PDF):
//   1st: User-created folders — your structure wins
//   2nd: Reyna-created folders — from past classifications
//   3rd: Create new folder — only when nothing fits
func (c *Classifier) ClassifyFile(fileName string, existingFolders []string) (folder string, isNew bool, confidence float64) {
	// Priority 1 & 2: Try keyword match against existing folders (user + reyna folders)
	if match, conf := c.keywordMatchFolder(fileName, existingFolders); match != "" {
		return match, false, conf
	}

	// Priority 1 & 2: Use LLM to match against existing folders (user + reyna)
	if c.IsEnabled() {
		if folder, isNew, conf := c.llmClassifyFile(fileName, existingFolders); folder != "" {
			// If LLM says use existing folder, that's priorities 1 or 2
			// If LLM says create new, that's priority 3
			return folder, isNew, conf
		}
	}

	// Priority 3: Keyword-based fallback for new folder creation
	if folder := c.keywordGuessSubject(fileName); folder != "" {
		return folder, true, 0.5
	}

	// Priority 4: never leave a file Unsorted — derive a folder from filename
	return guessFolderFromFilename(fileName), true, 0.3
}

// guessFolderFromFilename produces a sensible folder name from a filename when
// no other signal is available. Tries course-code prefixes, then content-type
// hints, then a generic "Notes" bucket.
func guessFolderFromFilename(fileName string) string {
	base := strings.TrimSuffix(fileName, filepathExt(fileName))
	lower := strings.ToLower(base)
	// Course code: leading letters+digits like BAI103, BESC104C, CSE201
	for i := 0; i < len(base); i++ {
		if base[i] == '_' || base[i] == '-' || base[i] == ' ' || base[i] == '.' {
			code := base[:i]
			if isCourseCode(code) {
				return strings.ToUpper(code) + " Notes"
			}
			break
		}
	}
	if isCourseCode(base) {
		return strings.ToUpper(base) + " Notes"
	}
	switch {
	case strings.Contains(lower, "slide") || strings.Contains(lower, "ppt"):
		return "Slides"
	case strings.Contains(lower, "assign") || strings.Contains(lower, "hw"):
		return "Assignments"
	case strings.Contains(lower, "lab"):
		return "Lab"
	case strings.Contains(lower, "syllabus"):
		return "Syllabus"
	}
	return "Notes"
}

func filepathExt(name string) string {
	for i := len(name) - 1; i >= 0 && name[i] != '/'; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

func isCourseCode(s string) bool {
	if len(s) < 4 || len(s) > 10 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

// keywordMatchFolder tries to match filename against existing folder names
func (c *Classifier) keywordMatchFolder(fileName string, folders []string) (string, float64) {
	lower := strings.ToLower(fileName)
	for _, f := range folders {
		fl := strings.ToLower(f)
		// Direct substring match
		if strings.Contains(lower, fl) {
			return f, 0.9
		}
		// Check common abbreviations
		abbrevs := map[string][]string{
			"dsa":           {"data structure", "algorithm", "sorting", "linked list", "tree", "graph"},
			"os":            {"operating system", "process", "thread", "scheduling", "memory management"},
			"dbms":          {"database", "sql", "normalization", "er diagram", "relational"},
			"cn":            {"computer network", "networking", "tcp", "udp", "osi", "routing"},
			"daa":           {"design and analysis", "algorithm", "complexity", "dynamic programming"},
			"coa":           {"computer organization", "architecture", "pipeline", "cache"},
			"compiler":      {"compiler design", "lexical", "parsing", "syntax"},
			"maths":         {"math", "calculus", "linear algebra", "probability", "statistics"},
			"physics":       {"physics", "mechanics", "thermodynamics", "optics", "quantum"},
			"chemistry":     {"chemistry", "organic", "inorganic", "physical chemistry"},
			"pyq":           {"previous year", "past paper", "exam paper", "question paper"},
			"assignment":    {"assignment", "homework", "submission"},
			"lab":           {"lab", "practical", "experiment"},
			"notes":         {"notes", "lecture", "module", "unit"},
		}
		if expanded, ok := abbrevs[fl]; ok {
			for _, term := range expanded {
				if strings.Contains(lower, term) {
					return f, 0.75
				}
			}
		}
	}
	return "", 0
}

// FileMeta carries the social/temporal context of a captured file so the
// classifier can use sender + timestamp as classification signals (per the
// architecture: WHO/WHAT/WHEN/WHY are first-class).
type FileMeta struct {
	SenderName  string
	SenderPhone string
	GroupName   string
	SharedAt    time.Time
}

// isValidFolder rejects junk folder names the LLM sometimes echoes back from
// the prompt's empty-list placeholder. Anything matching is treated as "no
// classification" so the caller falls through to filename-based guessing.
func isValidFolder(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return false
	}
	switch strings.ToLower(n) {
	case "none", "null", "n/a", "na", "unsorted", "unknown", "other", "misc", "miscellaneous":
		return false
	}
	return true
}

func formatMetaForPrompt(meta FileMeta) string {
	var parts []string
	if meta.SenderName != "" {
		parts = append(parts, "Shared by: "+meta.SenderName)
	}
	if meta.SenderPhone != "" {
		parts = append(parts, "Sender phone: "+meta.SenderPhone)
	}
	if meta.GroupName != "" {
		parts = append(parts, "Group: "+meta.GroupName)
	}
	if !meta.SharedAt.IsZero() {
		parts = append(parts, "Shared at: "+meta.SharedAt.Format("Mon 2006-01-02 15:04 MST"))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// ClassifyFileWithContent combines extraction + classification in a single LLM call.
// Per PDF: "Collapses Agents 1 & 2 into a single Claude call — more reliable, less code, better classification accuracy."
// meta carries sender/time context — Gemini uses these as additional signals.
func (c *Classifier) ClassifyFileWithContent(fileName, mimeType string, fileData []byte, existingFolders []string, meta FileMeta) (folder string, isNew bool, confidence float64, content string, summary string) {
	// Tier 1: keyword match against existing folders (free, instant)
	if match, conf := c.keywordMatchFolder(fileName, existingFolders); match != "" {
		return match, false, conf, "", ""
	}

	// Tier 2: Combined LLM call — extract + classify in one shot
	if c.IsEnabled() && len(fileData) > 0 && strings.Contains(mimeType, "pdf") {
		foldersStr := "(no existing folders — pick a descriptive new one)"
		if len(existingFolders) > 0 {
			foldersStr = strings.Join(existingFolders, ", ")
		}

		metaBlock := formatMetaForPrompt(meta)
		if metaBlock != "" {
			metaBlock = "\nContext:\n" + metaBlock + "\n"
		}

		prompt := fmt.Sprintf(`You are a document analysis and classification agent for a university study group's shared file system. Analyze the attached document AND its sharing context, then:
1. "content": detailed description of topics, concepts, chapters, key terms inside the document (max 800 chars). Read the actual document — do not guess from the filename.
2. "summary": one-line summary (max 100 chars).
3. "folder": classify into the best folder from: [%s]
   - Strongly prefer an existing folder if one fits.
   - If none fit, invent a clean 2–3 word Title Case folder name based on the document's actual subject.
   - NEVER return "None", "Unsorted", "Unknown", "Misc", "Other" or any placeholder. Always pick or invent a real subject folder.
4. "is_new": true if you invented the folder, false if it already exists.
5. "confidence": 0.0–1.0.

Use the sender, group, and time context as supporting signals (e.g. who tends to share which subject, recent exam season, etc.) but the document content is the primary signal.

Filename: "%s"%s

Respond ONLY with JSON:
{"content": "...", "summary": "...", "folder": "FolderName", "is_new": true/false, "confidence": 0.0-1.0}`, foldersStr, fileName, metaBlock)

		// Only send the doc inline if it fits Gemini's request size budget.
		// NEVER slice raw PDF bytes — that corrupts the file and Gemini returns 400.
		if len(fileData) <= geminiInlineMaxBytes {
			result, err := c.llm.CompleteWithDoc(prompt, fileData, mimeType, 1500)
			if err == nil {
				var resp struct {
					Content    string  `json:"content"`
					Summary    string  `json:"summary"`
					Folder     string  `json:"folder"`
					IsNew      bool    `json:"is_new"`
					Confidence float64 `json:"confidence"`
				}
				result = llm.CleanJSON(result)
				if jerr := json.Unmarshal([]byte(result), &resp); jerr == nil && isValidFolder(resp.Folder) {
					log.Printf("[NLP] Combined extract+classify: %s → %s (%.0f%%) [sender=%s]", fileName, resp.Folder, resp.Confidence*100, meta.SenderName)
					return resp.Folder, resp.IsNew, resp.Confidence, resp.Content, resp.Summary
				} else if jerr != nil {
					log.Printf("[NLP] Combined parse error: %v (raw: %.200s)", jerr, result)
				} else {
					log.Printf("[NLP] LLM returned invalid folder %q for %s — falling back", resp.Folder, fileName)
				}
			} else {
				log.Printf("[NLP] CompleteWithDoc failed for %s: %v", fileName, err)
			}
		} else {
			log.Printf("[NLP] %s is %d bytes — exceeds inline limit, skipping doc API", fileName, len(fileData))
		}
	}

	// Fallback to separate classification (filename-based)
	folder, isNew, confidence = c.ClassifyFile(fileName, existingFolders)
	return folder, isNew, confidence, "", ""
}

// llmClassifyFile uses the configured LLM to classify a file into a folder
func (c *Classifier) llmClassifyFile(fileName string, existingFolders []string) (string, bool, float64) {
	foldersStr := "None"
	if len(existingFolders) > 0 {
		foldersStr = strings.Join(existingFolders, ", ")
	}

	prompt := fmt.Sprintf(`You are a file classification system for a university study group. Given a filename and a list of existing folders, determine the best folder for this file.

Existing folders: [%s]

Filename: "%s"

Rules:
1. If the file clearly belongs in an existing folder, return that folder name exactly as written.
2. If no existing folder fits but you can infer a clear academic subject, suggest a new folder name (2-3 words max, title case).
3. Always pick or invent a descriptive folder. NEVER return "Unsorted" — if unsure, infer from course code, filename keywords, or document type.

Respond ONLY with a JSON object, no other text:
{"folder": "FolderName", "is_new": true/false, "confidence": 0.0-1.0}`, foldersStr, fileName)

	result, err := c.llm.Complete(prompt, 600)
	if err != nil {
		log.Printf("[NLP] Classification error (%s): %v", c.llm.Name(), err)
		return "", false, 0
	}

	var resp struct {
		Folder     string  `json:"folder"`
		IsNew      bool    `json:"is_new"`
		Confidence float64 `json:"confidence"`
	}

	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[NLP] Parse error: %v (raw: %s)", err, result)
		return "", false, 0
	}

	if !isValidFolder(resp.Folder) {
		return guessFolderFromFilename(fileName), true, 0.3
	}
	return resp.Folder, resp.IsNew, resp.Confidence
}

// keywordGuessSubject is the free fallback — improved version of the original guessSubject
func (c *Classifier) keywordGuessSubject(fileName string) string {
	lower := strings.ToLower(fileName)
	patterns := map[string][]string{
		"DSA":               {"dsa", "data structure", "algorithm", "sorting", "linked list", "binary tree", "graph algorithm"},
		"Operating Systems": {"os ", "operating system", "process scheduling", "memory management", "deadlock"},
		"DBMS":              {"dbms", "database", "sql", "normalization", "er diagram", "relational"},
		"Computer Networks": {"cn ", "computer network", "networking", "tcp", "udp", "osi model", "routing"},
		"DAA":               {"daa", "design and analysis", "complexity", "dynamic programming", "greedy"},
		"COA":               {"coa", "computer organization", "architecture", "pipeline", "cache memory"},
		"Compiler Design":   {"compiler", "lexical analysis", "parsing", "syntax tree"},
		"Mathematics":       {"math", "calculus", "linear algebra", "probability", "statistics", "discrete"},
		"Physics":           {"physics", "mechanics", "thermodynamics", "optics", "quantum", "electromagnetic"},
		"Chemistry":         {"chemistry", "organic", "inorganic", "physical chemistry", "periodic"},
		"PYQ":               {"pyq", "previous year", "past paper", "exam paper", "question paper"},
		"Assignments":       {"assignment", "homework", "submission", "task"},
		"Lab":               {"lab", "practical", "experiment", "lab manual"},
		"Circulars":         {"circular", "notice", "notification", "holiday", "schedule", "time table", "timetable"},
		"Admit Cards":       {"admit card", "hall ticket", "exam admit"},
		"Syllabus":          {"syllabus", "curriculum", "course outline", "course plan"},
		"Projects":          {"project", "proposal", "report", "presentation", "ppt"},
		"Research":          {"research", "paper", "journal", "ieee", "survey"},
		"CAD & Engineering": {"caed", "cad", "autocad", "engineering drawing", "engineering graphics"},
	}
	for folder, keywords := range patterns {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return folder
			}
		}
	}
	return ""
}

// ── Intent Detection ──

// DetectIntent classifies a natural language message into a Reyna intent
func (c *Classifier) DetectIntent(message string) (intent string, query string) {
	lower := strings.ToLower(strings.TrimSpace(message))

	// Strip wake word prefix
	for _, prefix := range []string{"reyna ", "reyna, ", "hey reyna ", "@reyna "} {
		lower = strings.TrimPrefix(lower, prefix)
	}

	// Keyword-based intent detection (free, instant, handles 80%+ of cases)
	if intent, query := c.keywordDetectIntent(lower); intent != "unknown" {
		return intent, query
	}

	// LLM fallback for ambiguous messages
	if c.IsEnabled() {
		if intent, query := c.llmDetectIntent(lower); intent != "unknown" {
			return intent, query
		}
	}

	return "unknown", ""
}

// keywordDetectIntent uses pattern matching for common phrases
func (c *Classifier) keywordDetectIntent(msg string) (string, string) {
	// SAVE intent
	savePatterns := []string{"save", "add", "stage", "track", "store", "keep", "backup"}
	for _, p := range savePatterns {
		if strings.Contains(msg, p) {
			return "save", ""
		}
	}

	// PUSH / COMMIT intent
	pushPatterns := []string{"push", "commit", "upload", "sync", "send to drive", "backup to drive"}
	for _, p := range pushPatterns {
		if strings.Contains(msg, p) {
			return "push", ""
		}
	}

	// SEARCH / FIND intent
	searchPatterns := []string{"find", "search", "look for", "where is", "get me", "show me", "do you have", "need", "send me"}
	for _, p := range searchPatterns {
		if strings.Contains(msg, p) {
			// Extract query: everything after the pattern
			idx := strings.Index(msg, p)
			query := strings.TrimSpace(msg[idx+len(p):])
			query = strings.Trim(query, "\"'?.,!")
			if query == "" {
				query = msg // Fallback to full message
			}
			return "search", query
		}
	}

	// Bare noun phrases (likely search) — "dsa notes", "os pyq", "unit 3 notes"
	academicTerms := []string{"notes", "pyq", "paper", "assignment", "module", "unit", "lab", "slides", "pdf"}
	for _, t := range academicTerms {
		if strings.Contains(msg, t) {
			return "search", msg
		}
	}

	// HISTORY / LOG intent
	historyPatterns := []string{"history", "log", "recent", "latest", "last", "what was shared", "all files"}
	for _, p := range historyPatterns {
		if strings.Contains(msg, p) {
			return "history", ""
		}
	}

	// STATUS intent
	statusPatterns := []string{"status", "what's new", "whats new", "update", "how many", "count", "overview"}
	for _, p := range statusPatterns {
		if strings.Contains(msg, p) {
			return "status", ""
		}
	}

	// HELP intent
	helpPatterns := []string{"help", "how to", "how do", "what can you", "commands", "guide", "tutorial"}
	for _, p := range helpPatterns {
		if strings.Contains(msg, p) {
			return "help", ""
		}
	}

	return "unknown", ""
}

// llmDetectIntent uses the configured LLM for ambiguous messages
func (c *Classifier) llmDetectIntent(msg string) (string, string) {
	prompt := fmt.Sprintf(`You are an intent classifier for Reyna, a file management bot in a WhatsApp study group. Classify this message into one of these intents:

- "save" — user wants to save/stage/track a file
- "push" — user wants to commit/upload staged files to Google Drive
- "search" — user is looking for a specific file or topic (extract the search query)
- "history" — user wants to see recent files or activity log
- "status" — user wants a summary of what's new or current state
- "help" — user wants to know what the bot can do
- "unknown" — cannot determine intent

Message: "%s"

Respond ONLY with JSON, no other text:
{"intent": "search", "query": "DSA notes"}`, msg)

	result, err := c.llm.Complete(prompt, 600)
	if err != nil {
		log.Printf("[NLP] Intent detection error (%s): %v", c.llm.Name(), err)
		return "unknown", ""
	}

	var resp struct {
		Intent string `json:"intent"`
		Query  string `json:"query"`
	}

	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[NLP] Intent parse error: %v (raw: %s)", err, result)
		return "unknown", ""
	}

	return resp.Intent, resp.Query
}

// ── Content Extraction Agent ──
// Per PDF: "Files sent directly to Claude API as document blocks (base64).
// Returns extracted metadata: topic, subject area, key concepts.
// Collapses extraction + classification into a single API call."

// ExtractContent sends the actual file data to the LLM for deep content extraction.
// PDFs are base64-encoded and sent as document blocks to Claude/Gemini.
// For providers that don't support doc blocks (OpenAI/Grok), falls back to filename analysis.
func (c *Classifier) ExtractContent(fileName, mimeType string, fileSize int64, fileData []byte) (content string, summary string) {
	if !c.IsEnabled() {
		return "", ""
	}

	prompt := fmt.Sprintf(`You are a document analysis agent for a university study group file system.
Analyze this document and extract:
1. "content": detailed description of the topics, concepts, chapters, key terms it covers (max 800 chars)
2. "summary": one-line summary (max 100 chars)

Filename: "%s"

Respond ONLY with JSON, no other text:
{"content": "...", "summary": "..."}`, fileName)

	var result string
	var err error

	// Try sending actual file data for deep extraction (PDF, images).
	// Never slice raw bytes — that corrupts the file. Only send if it fits inline.
	if len(fileData) > 0 && len(fileData) <= geminiInlineMaxBytes && (strings.Contains(mimeType, "pdf") || strings.Contains(mimeType, "image")) {
		result, err = c.llm.CompleteWithDoc(prompt, fileData, mimeType, 1500)
	} else {
		// For DOCXs and other types, use filename-based analysis
		result, err = c.llm.Complete(prompt, 800)
	}

	if err != nil {
		log.Printf("[EXTRACT] Error: %v, falling back to filename analysis", err)
		return c.extractFromFilename(fileName, mimeType, fileSize)
	}

	var resp struct {
		Content string `json:"content"`
		Summary string `json:"summary"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[EXTRACT] Parse error: %v", err)
		return c.extractFromFilename(fileName, mimeType, fileSize)
	}
	return resp.Content, resp.Summary
}

// extractFromFilename is the fallback when file data can't be sent to the LLM
func (c *Classifier) extractFromFilename(fileName, mimeType string, fileSize int64) (string, string) {
	prompt := fmt.Sprintf(`You are a document analysis agent. Given this filename and metadata, infer what the document likely contains:
1. "content": detailed description of likely topics and concepts (max 500 chars)
2. "summary": one-line summary (max 100 chars)

Filename: "%s", Type: %s, Size: %d bytes

Respond ONLY with JSON: {"content": "...", "summary": "..."}`, fileName, mimeType, fileSize)

	result, err := c.llm.Complete(prompt, 800)
	if err != nil {
		return "", ""
	}
	var resp struct {
		Content string `json:"content"`
		Summary string `json:"summary"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return "", ""
	}
	return resp.Content, resp.Summary
}

// ── NLP Query Parsing (WHO/WHAT/WHEN/WHY) ──

// ParseNLPQuery parses a natural language query into structured components.
// Uses AI as primary parser, keyword as fallback (per PDF: "the main killer feature").
func (c *Classifier) ParseNLPQuery(query string) (who, what, when, why string) {
	// Primary: Use LLM for accurate parsing of any natural language
	if c.IsEnabled() {
		who, what, when, why = c.llmParseQuery(query)
		if who != "" || what != "" {
			// Drop generic filler words from WHAT that would over-filter results
			what = c.cleanGenericWhat(what)
			return
		}
	}

	// Fallback: keyword parsing (free, instant)
	who, what, when, why = c.keywordParseQuery(query)
	what = c.cleanGenericWhat(what)
	return
}

// cleanGenericWhat removes filler words from WHAT that would incorrectly filter results.
// "notes", "files", "stuff", "things" etc. are too generic to be useful search terms.
func (c *Classifier) cleanGenericWhat(what string) string {
	generic := map[string]bool{
		"notes": true, "files": true, "stuff": true, "things": true,
		"documents": true, "docs": true, "material": true, "content": true,
		"some notes": true, "some files": true, "something": true,
	}
	if generic[strings.ToLower(strings.TrimSpace(what))] {
		return "" // drop it — too generic to filter on
	}
	return what
}

func (c *Classifier) keywordParseQuery(query string) (who, what, when, why string) {
	lower := strings.ToLower(strings.TrimSpace(query))
	lower = strings.Trim(lower, "?.,! ")

	// Time patterns — extract and remove from query
	timePatterns := map[string]string{
		"today":           "today",
		"yesterday":       "yesterday",
		"last week":       "last_week",
		"this week":       "this_week",
		"last month":      "last_month",
		"since monday":    "this_week",
		"since tuesday":   "this_week",
		"since wednesday": "this_week",
		"since thursday":  "this_week",
		"since friday":    "this_week",
	}
	for pattern, value := range timePatterns {
		if strings.Contains(lower, pattern) {
			when = value
			lower = strings.Replace(lower, pattern, "", 1)
			break
		}
	}

	// Common noise words to skip when detecting WHO
	skipWords := map[string]bool{
		"the": true, "any": true, "anyone": true, "someone": true, "we": true,
		"i": true, "you": true, "me": true, "my": true, "some": true,
		"a": true, "an": true, "all": true, "those": true, "these": true,
	}

	// Action verbs that come AFTER a person's name: "X sent me", "X shared", "X uploaded"
	actionVerbs := []string{" sent ", " shared ", " uploaded ", " gave ", " posted "}
	for _, verb := range actionVerbs {
		if idx := strings.Index(lower, verb); idx > 0 {
			// Everything before the verb is potentially the WHO
			beforeVerb := strings.TrimSpace(lower[:idx])
			// Take the last word before the verb as the name
			parts := strings.Fields(beforeVerb)
			if len(parts) > 0 {
				candidate := parts[len(parts)-1]
				if !skipWords[candidate] && len(candidate) > 1 {
					who = candidate
					// Everything after the verb is the WHAT
					afterVerb := strings.TrimSpace(lower[idx+len(verb):])
					// Clean up noise from what
					for _, noise := range []string{"me ", "us ", "some ", "any ", "the "} {
						afterVerb = strings.TrimPrefix(afterVerb, noise)
					}
					what = strings.Trim(afterVerb, "?.,! ")
					if what == "" {
						what = ""
					}
					why = "retrieve"
					return who, what, when, why
				}
			}
		}
	}

	// Pattern: "what did X share/send/upload"
	prefixPatterns := []string{"what did ", "did ", "has ", "from ", "files from ", "notes from ", "shared by "}
	for _, p := range prefixPatterns {
		if idx := strings.Index(lower, p); idx >= 0 {
			rest := lower[idx+len(p):]
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				candidate := parts[0]
				if !skipWords[candidate] && len(candidate) > 1 {
					who = candidate
					// Rest after the name is the WHAT context
					remaining := strings.Join(parts[1:], " ")
					for _, noise := range []string{"share", "shared", "upload", "uploaded", "send", "sent", "about", "any", "the", "me"} {
						remaining = strings.Replace(remaining, noise, "", -1)
					}
					remaining = strings.Trim(strings.TrimSpace(remaining), "?.,! ")
					if remaining != "" {
						what = remaining
					}
					lower = "" // consumed
					break
				}
			}
		}
	}

	// WHY patterns
	if strings.Contains(lower, "find") || strings.Contains(lower, "search") || strings.Contains(lower, "get") {
		why = "search"
	} else if strings.Contains(lower, "do we have") || strings.Contains(lower, "has anyone") || strings.Contains(lower, "is there") {
		why = "check_existence"
	} else if strings.Contains(lower, "what's new") || strings.Contains(lower, "what is new") {
		why = "activity_check"
	} else {
		why = "retrieve"
	}

	// WHAT — if not already set, clean up remaining text
	if what == "" && lower != "" {
		what = lower
		for _, w := range []string{"share", "shared", "upload", "uploaded", "sent", "send", "about", "any", "the",
			"do we have", "has anyone shared", "find", "search for", "get me", "show me", "me", "some"} {
			what = strings.Replace(what, w, "", -1)
		}
		what = strings.Trim(strings.TrimSpace(what), "?.,! ")
	}

	return who, what, when, why
}

func (c *Classifier) llmParseQuery(query string) (who, what, when, why string) {
	prompt := fmt.Sprintf(`You are a query parser for a file retrieval system in a WhatsApp study group.
Parse this natural language query into structured search filters.

Query: "%s"

Rules:
- "who": Extract the PERSON'S NAME if the user is asking about files from a specific person. Leave empty if no person mentioned.
- "what": Extract the SPECIFIC TOPIC or SUBJECT being searched. If the user just says generic words like "notes", "files", "stuff", "documents" with no specific topic, leave this empty.
- "when": Extract time reference as one of: today, yesterday, last_week, this_week, last_month. Leave empty if no time mentioned.
- "why": One of: retrieve, search, check_existence, activity_check

Examples:
- "mohit sent some notes" → {"who":"mohit","what":"","when":"","why":"retrieve"}
- "do we have OS notes?" → {"who":"","what":"OS","when":"","why":"check_existence"}
- "what did priya upload yesterday?" → {"who":"priya","what":"","when":"yesterday","why":"retrieve"}
- "find compiler lab manual" → {"who":"","what":"compiler lab manual","when":"","why":"search"}
- "rakesh shared quantum mechanics pdf" → {"who":"rakesh","what":"quantum mechanics","when":"","why":"retrieve"}

Respond ONLY with JSON, no other text:
{"who":"","what":"","when":"","why":"retrieve"}`, query)

	result, err := c.llm.Complete(prompt, 600)
	if err != nil {
		log.Printf("[NLP] LLM parse failed: %v, falling back to keyword parser", err)
		// Fall back to keyword parser instead of returning raw query
		return c.keywordParseQuery(query)
	}

	var resp struct {
		Who  string `json:"who"`
		What string `json:"what"`
		When string `json:"when"`
		Why  string `json:"why"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[NLP] LLM parse JSON error: %v, falling back to keyword parser", err)
		return c.keywordParseQuery(query)
	}
	return resp.Who, resp.What, resp.When, resp.Why
}

// ── Notes Q&A ──

// AnswerFromNotes takes a question and relevant file content, asks the LLM for an answer
func (c *Classifier) AnswerFromNotes(question string, fileContents map[string]string) string {
	if !c.IsEnabled() || len(fileContents) == 0 {
		return "I don't have enough content from your notes to answer that. Make sure files have been shared and extracted."
	}

	// Build context from file contents
	var context strings.Builder
	for fileName, content := range fileContents {
		// Truncate each file's content to avoid token limits
		if len(content) > 2000 {
			content = content[:2000] + "..."
		}
		context.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", fileName, content))
	}

	prompt := fmt.Sprintf(`You are Reyna, a helpful study assistant. Answer the student's question based ONLY on the provided notes content. Be concise and helpful. If the notes don't contain enough information, say so.

NOTES CONTENT:
%s

QUESTION: %s

Answer concisely (max 300 words):`, context.String(), question)

	result, err := c.llm.Complete(prompt, 500)
	if err != nil {
		return "Sorry, I couldn't process that question right now. Try again."
	}
	return strings.TrimSpace(result)
}
