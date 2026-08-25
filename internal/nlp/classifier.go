package nlp

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/docs"
	"github.com/hurshnarayan/reyna/internal/integrations/llm"
	"github.com/hurshnarayan/reyna/internal/model"
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
//
//	1st: User-created folders — your structure wins
//	2nd: Reyna-created folders — from past classifications
//	3rd: Create new folder — only when nothing fits
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
// no other signal is available.
//
// Deliberately vague. This runs only when the model is unavailable and the
// filename is the only clue, and a wrong specific answer is worse than a
// right vague one: a bank statement filed under "Assignments" is harder to
// find than one left in a general bucket. It used to read leading course
// codes like CSE201 and produce "CSE201 Notes", which was correct for a
// class group and wrong for everyone else.
func guessFolderFromFilename(fileName string) string {
	lower := strings.ToLower(strings.TrimSuffix(fileName, filepathExt(fileName)))
	switch {
	case containsAny(lower, "invoice", "receipt", "bill", "payment"):
		return "Receipts"
	case containsAny(lower, "contract", "agreement", "lease", "policy", "nda"):
		return "Contracts"
	case containsAny(lower, "ticket", "boarding", "itinerary", "booking"):
		return "Travel"
	case containsAny(lower, "slide", "ppt", "deck", "presentation"):
		return "Presentations"
	}
	return "Documents"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func filepathExt(name string) string {
	for i := len(name) - 1; i >= 0 && name[i] != '/'; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
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
			"dsa":        {"data structure", "algorithm", "sorting", "linked list", "tree", "graph"},
			"os":         {"operating system", "process", "thread", "scheduling", "memory management"},
			"dbms":       {"database", "sql", "normalization", "er diagram", "relational"},
			"cn":         {"computer network", "networking", "tcp", "udp", "osi", "routing"},
			"daa":        {"design and analysis", "algorithm", "complexity", "dynamic programming"},
			"coa":        {"computer organization", "architecture", "pipeline", "cache"},
			"compiler":   {"compiler design", "lexical", "parsing", "syntax"},
			"maths":      {"math", "calculus", "linear algebra", "probability", "statistics"},
			"physics":    {"physics", "mechanics", "thermodynamics", "optics", "quantum"},
			"chemistry":  {"chemistry", "organic", "inorganic", "physical chemistry"},
			"pyq":        {"previous year", "past paper", "exam paper", "question paper"},
			"assignment": {"assignment", "homework", "submission"},
			"lab":        {"lab", "practical", "experiment"},
			"notes":      {"notes", "lecture", "module", "unit"},
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

// snapToExistingFolder takes the folder name the LLM produced and the list of
// folders that already exist, and returns the existing folder if it's a near-
// match (90% token overlap, or one is a strict subset of the other). Stops
// the LLM from inventing "C Programming Lab" when "C Programming Laboratory"
// already exists, "Python Programming" vs "Python Programming Lab", etc.
func snapToExistingFolder(suggestion string, existing []string) string {
	if suggestion == "" || len(existing) == 0 {
		return suggestion
	}
	suggLower := strings.ToLower(strings.TrimSpace(suggestion))
	suggTokens := folderTokens(suggLower)
	if len(suggTokens) == 0 {
		return suggestion
	}

	bestExisting := ""
	bestScore := 0.0
	for _, ex := range existing {
		exLower := strings.ToLower(strings.TrimSpace(ex))
		if exLower == suggLower {
			return ex // exact match (case-insensitive)
		}
		exTokens := folderTokens(exLower)
		if len(exTokens) == 0 {
			continue
		}
		// Strict subset check: every token of one is in the other → very strong signal
		if isSubset(suggTokens, exTokens) || isSubset(exTokens, suggTokens) {
			return ex
		}
		// Jaccard similarity over significant tokens
		score := jaccard(suggTokens, exTokens)
		if score > bestScore {
			bestScore = score
			bestExisting = ex
		}
	}
	// Snap if the best match has >= 60% token overlap. This catches cases like
	// "Python Programming" vs "Python Programming Modules" (Jaccard ~0.66) or
	// "DBMS Notes" vs "DBMS" (Jaccard 0.5 → not snapped, but the subset check
	// above would catch it first).
	if bestScore >= 0.6 && bestExisting != "" {
		log.Printf("[FOLDER-SNAP] %q → %q (jaccard=%.2f)", suggestion, bestExisting, bestScore)
		return bestExisting
	}
	return suggestion
}

// folderTokens splits a folder name into significant lowercase tokens, dropping
// noise words that don't carry subject identity (notes, notes, lab, etc. are
// kept because they DO matter — but pure connectives like "the", "and", "of"
// are dropped).
func folderTokens(name string) []string {
	noise := map[string]bool{
		"the": true, "and": true, "of": true, "for": true, "in": true,
		"a": true, "an": true, "to": true, "with": true, "&": true,
	}
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, name)
	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(cleaned) {
		if len(tok) < 2 || noise[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// isSubset returns true if every token of `a` appears in `b`.
func isSubset(a, b []string) bool {
	if len(a) == 0 || len(a) > len(b) {
		return false
	}
	bset := map[string]bool{}
	for _, t := range b {
		bset[t] = true
	}
	for _, t := range a {
		if !bset[t] {
			return false
		}
	}
	return true
}

// jaccard returns the Jaccard similarity (|intersection| / |union|) between
// two token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	aset := map[string]bool{}
	for _, t := range a {
		aset[t] = true
	}
	bset := map[string]bool{}
	for _, t := range b {
		bset[t] = true
	}
	intersection := 0
	for t := range aset {
		if bset[t] {
			intersection++
		}
	}
	union := len(aset) + len(bset) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
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

	// Tier 2a: Office docs (DOCX/PPTX/XLSX) — extract text from the zipped XML
	// payload, then send the extracted text to Gemini for classification. This
	// is the fix for "PPTX files all get scattered into different folders"
	// because filename-only guessing can't tell that Py_Module_3, Py_Module_4
	// and Py_Module_5 are all the same subject.
	if c.IsEnabled() && len(fileData) > 0 && IsOfficeDoc(mimeType) {
		extracted, err := ExtractOfficeText(fileData, mimeType, 30*1024)
		if err == nil && len(extracted) > 100 {
			folder, isNew, conf, summary := c.classifyFromExtractedText(fileName, mimeType, extracted, existingFolders, meta)
			if folder != "" {
				log.Printf("[NLP] Office content classify: %s → %s (%.0f%%) [sender=%s]", fileName, folder, conf*100, meta.SenderName)
				return folder, isNew, conf, extracted, summary
			}
		} else if err != nil {
			log.Printf("[NLP] Office text extract failed for %s: %v", fileName, err)
		}
	}

	// Tier 2b: PDFs — combined LLM call with the file as inline doc block
	if c.IsEnabled() && len(fileData) > 0 && strings.Contains(mimeType, "pdf") {
		foldersStr := "(no existing folders — pick a descriptive new one)"
		if len(existingFolders) > 0 {
			foldersStr = strings.Join(existingFolders, ", ")
		}

		metaBlock := formatMetaForPrompt(meta)
		if metaBlock != "" {
			metaBlock = "\nContext:\n" + metaBlock + "\n"
		}

		prompt := fmt.Sprintf(`You are a document analysis and classification agent for a personal file archive. The files can be anything a person keeps: invoices, contracts, tickets, receipts, medical records, scanned paperwork, manuals, photos of documents, work files, study material. Analyze the attached document AND its sharing context, then:
1. "content": the document's actual readable text, not a description of it. Transcribe what it says.
   This is the only record kept of what is inside the file, and every later question is answered from it alone.
   Copy every fact verbatim: names, dates, times, amounts, reference numbers, room and seat codes, table rows, deadlines, contact details, terms.
   A table becomes one line per row with its columns separated by " | ".
   Do NOT write "contains a timetable with room assignments". Write the rows, with the rooms in them.
   Skip decoration and repeated headers. Up to 6000 characters; if the document is longer, keep the parts carrying specific facts and drop the prose.
   Begin each page with a marker on its own line, exactly [[page 1]], [[page 2]] and so on, so a later answer can point at the page it came from. Number them as the document does if it prints page numbers, otherwise count from one.
2. "summary": one-line summary (max 100 chars).
3. "folder": classify into the best folder from: [%s]

   STRICT folder rules:
   - Use an existing folder ONLY if the document is unambiguously about that exact thing. "Close enough" is not a match.
   - Wrong: a phone bill under "Contracts"; a tax return under "Receipts"; a database exam paper under "Computer Science". These are different things, do not lump them.
   - Right: an electricity bill → an existing "Electricity Bills" folder; a compiler design manual → an existing "Compiler Design" folder.
   - If nothing matches exactly, invent a clean two or three word Title Case folder named after what the document actually is, for example "Electricity Bills", "Rental Agreement", "Flight Tickets", "Compiler Design".
   - NEVER return "None", "Unsorted", "Unknown", "Misc", "Other" or "General". Always something specific.

4. "is_new": true if you invented the folder, false if it already exists in the list above.
5. "confidence": 0.0–1.0. Lower confidence (≤0.6) if you had to invent the folder or if the subject is ambiguous.

Use the sender, group, and time context as supporting signals (e.g. who tends to share which subject, recent exam season) but the document content is the primary signal.

Filename: "%s"%s

Respond ONLY with JSON:
{"content": "...", "summary": "...", "folder": "FolderName", "is_new": true/false, "confidence": 0.0-1.0}`, foldersStr, fileName, metaBlock)

		// Only send the doc inline if it fits Gemini's request size budget.
		// NEVER slice raw PDF bytes — that corrupts the file and Gemini returns 400.
		if len(fileData) <= geminiInlineMaxBytes {
			// Room for the transcription asked for above. A tight budget here
			// silently truncates the JSON and loses exactly the tail of the
			// document where deadlines and totals tend to live.
			result, err := c.llm.CompleteWithDoc(prompt, fileData, mimeType, 8000)
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
					// Snap to a near-matching existing folder if one exists
					// — stops the LLM from inventing "C Programming Lab"
					// when "C Programming Laboratory" already exists.
					snapped := snapToExistingFolder(resp.Folder, existingFolders)
					if snapped != resp.Folder {
						resp.Folder = snapped
						resp.IsNew = false
					}
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

// classifyFromExtractedText runs the same content-based classification as the
// PDF inline-doc path but uses pre-extracted text (e.g. from a DOCX/PPTX/XLSX
// that we unzipped ourselves). Returns ("", false, 0, "") on failure so the
// caller can fall through to filename-only classification.
func (c *Classifier) classifyFromExtractedText(fileName, mimeType, extractedText string, existingFolders []string, meta FileMeta) (folder string, isNew bool, confidence float64, summary string) {
	if !c.IsEnabled() || extractedText == "" {
		return "", false, 0, ""
	}
	foldersStr := "(no existing folders — pick a descriptive new one)"
	if len(existingFolders) > 0 {
		foldersStr = strings.Join(existingFolders, ", ")
	}
	metaBlock := formatMetaForPrompt(meta)
	if metaBlock != "" {
		metaBlock = "\nContext:\n" + metaBlock + "\n"
	}
	// Cap text fed to the LLM
	if len(extractedText) > 12000 {
		extractedText = extractedText[:12000] + "..."
	}

	prompt := fmt.Sprintf(`You are a document analysis and classification agent for a personal file archive. The files can be anything a person keeps: invoices, contracts, tickets, receipts, medical records, scanned paperwork, manuals, photos of documents, work files, study material. The document is a %s — its full text content (extracted from the file) is provided below. Analyze it and return:
1. "summary": one-line summary of what the document is actually about (max 100 chars). Use the CONTENT, not the filename.
2. "folder": classify into the best folder from: [%s]

   STRICT folder rules:
   - Use an existing folder ONLY if the document is unambiguously about that exact subject. "Close enough" is NOT a match.
   - Multiple files about the same subject MUST end up in the same folder. If you previously created "Python Programming" and a similar file arrives, use "Python Programming" again — do NOT invent "Python Modules" or "Programming Modules" as a separate folder.
   - Examples of WRONG matches: putting CAED under "Engineering Science"; putting DBMS under "Computer Science"; splitting "Python Module 3" and "Python Module 4" into different folders. These belong together.
   - If no existing folder is an exact subject match, INVENT a clean 2-3 word Title Case folder named after the actual subject. Recognise Indian engineering course codes (CAED, ESC, BESC, BCS, BEC, BCSL, BPLC) as their own subjects.
   - NEVER return "None", "Unsorted", "Unknown", "Misc", "Other", "General", "Engineering", "Science", "Programming Modules", "Modules" or any vague umbrella. Always pick a SPECIFIC subject folder.

3. "is_new": true if you invented the folder, false if it already exists.
4. "confidence": 0.0-1.0.

Use sender/group/time context as supporting signals but the document content is the primary signal.

Filename: "%s"%s

DOCUMENT CONTENT:
%s

Respond ONLY with JSON:
{"summary": "...", "folder": "FolderName", "is_new": true/false, "confidence": 0.0-1.0}`, mimeTypeLabel(mimeType), foldersStr, fileName, metaBlock, extractedText)

	result, err := c.llm.Complete(prompt, 800)
	if err != nil {
		log.Printf("[NLP] classifyFromExtractedText error for %s: %v", fileName, err)
		return "", false, 0, ""
	}
	var resp struct {
		Summary    string  `json:"summary"`
		Folder     string  `json:"folder"`
		IsNew      bool    `json:"is_new"`
		Confidence float64 `json:"confidence"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[NLP] classifyFromExtractedText parse error for %s: %v (raw: %.150s)", fileName, err, result)
		return "", false, 0, ""
	}
	if !isValidFolder(resp.Folder) {
		return "", false, 0, ""
	}
	// Snap to a near-matching existing folder if one exists.
	snapped := snapToExistingFolder(resp.Folder, existingFolders)
	if snapped != resp.Folder {
		resp.Folder = snapped
		resp.IsNew = false
	}
	return resp.Folder, resp.IsNew, resp.Confidence, resp.Summary
}

func mimeTypeLabel(mimeType string) string {
	switch {
	case strings.Contains(mimeType, "wordprocessingml"), strings.HasSuffix(mimeType, "docx"):
		return "Microsoft Word document (.docx)"
	case strings.Contains(mimeType, "presentationml"), strings.HasSuffix(mimeType, "pptx"):
		return "Microsoft PowerPoint presentation (.pptx)"
	case strings.Contains(mimeType, "spreadsheetml"), strings.HasSuffix(mimeType, "xlsx"):
		return "Microsoft Excel spreadsheet (.xlsx)"
	case strings.Contains(mimeType, "pdf"):
		return "PDF document"
	default:
		return "document"
	}
}

// llmClassifyFile uses the configured LLM to classify a file into a folder
func (c *Classifier) llmClassifyFile(fileName string, existingFolders []string) (string, bool, float64) {
	foldersStr := "None"
	if len(existingFolders) > 0 {
		foldersStr = strings.Join(existingFolders, ", ")
	}

	prompt := fmt.Sprintf(`You are a file classification system for a personal file archive covering any kind of document. Given a filename and a list of existing folders, determine the best folder for this file.

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
	// Snap to a near-matching existing folder if one exists.
	snapped := snapToExistingFolder(resp.Folder, existingFolders)
	if snapped != resp.Folder {
		return snapped, false, resp.Confidence
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
	prompt := fmt.Sprintf(`You are an intent classifier for Reyna, which files and finds documents shared in a person's chats. Classify this message into one of these intents:

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
	// Read it here when the format allows it.
	//
	// Free, instant, exact, and it does not touch a daily allowance measured
	// in tens of calls. Only PDFs genuinely need the model, because nothing in
	// the standard library turns a PDF into text.
	if docs.CanExtract(fileName) {
		if text, err := docs.Text(fileName, fileData); err == nil && strings.TrimSpace(text) != "" {
			return text, c.summarise(fileName, text)
		}
	}

	if !c.IsEnabled() {
		return "", ""
	}

	// Only PDFs and images reach the model. Everything else either was read
	// above or cannot be read at all, and guessing a document's contents from
	// its filename is worse than admitting it has not been read: it produces a
	// confident paragraph about a document nobody has opened, and the answer
	// generator has no way to tell that apart from a real reading.
	canSend := len(fileData) > 0 && len(fileData) <= geminiInlineMaxBytes &&
		(strings.Contains(mimeType, "pdf") || strings.Contains(mimeType, "image"))
	if !canSend {
		return "", ""
	}

	prompt := fmt.Sprintf(`Read this document and return what it actually says.

1. "content": the document's readable text, not a description of it. Transcribe it.
   Copy every fact verbatim: names, dates, times, amounts, reference numbers, room and seat codes, table rows, deadlines, contact details, terms, definitions, formulas.
   A table becomes one line per row with its columns separated by " | ".
   Do NOT write "contains a timetable with room assignments". Write the rows, with the rooms in them.
   Skip decoration and repeated headers. Up to 6000 characters.
   Begin each page with a marker on its own line, exactly [[page 1]], [[page 2]] and so on.
2. "summary": one line, under 100 characters, saying what the document is.

Filename: "%s"

Respond ONLY with JSON, no other text:
{"content": "...", "summary": "..."}`, fileName)

	result, err := c.llm.CompleteWithDoc(prompt, fileData, mimeType, 8000)
	if err != nil {
		log.Printf("[EXTRACT] %s: %v", fileName, err)
		return "", ""
	}

	var resp struct {
		Content string `json:"content"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(llm.CleanJSON(result)), &resp); err != nil {
		log.Printf("[EXTRACT] %s: parse error: %v", fileName, err)
		return "", ""
	}
	return resp.Content, resp.Summary
}

// summarise writes the one line shown beside a file, from text already in hand.
//
// Falls back to the opening line rather than spending a model call: the
// summary is a label, and a label is not worth one of the day's few calls when
// the full text is already stored and searchable.
func (c *Classifier) summarise(fileName, text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[[page") {
			continue
		}
		if len(line) > 100 {
			line = line[:100]
		}
		return line
	}
	return fileName
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
	return c.ParseNLPQueryWithHistory(query, nil)
}

// ParseNLPQueryWithHistory parses a query with recent conversation history for pronoun/context resolution.
func (c *Classifier) ParseNLPQueryWithHistory(query string, history []model.ChatMessageContext) (who, what, when, why string) {
	// Primary: Use LLM for accurate parsing of any natural language with context
	if c.IsEnabled() {
		who, what, when, why = c.llmParseQueryWithHistory(query, history)
		if who != "" || what != "" {
			if strings.EqualFold(strings.TrimSpace(who), "reyna") {
				if what == "" {
					what = "reyna"
				} else if !strings.Contains(strings.ToLower(what), "reyna") {
					what = "reyna " + what
				}
				who = ""
			}
			// Drop generic filler words from WHAT that would over-filter results
			what = c.cleanGenericWhat(what)
			return
		}
	}

	// Fallback: keyword parsing (free, instant)
	who, what, when, why = c.keywordParseQuery(query)
	if strings.EqualFold(strings.TrimSpace(who), "reyna") {
		if what == "" {
			what = "reyna"
		} else if !strings.Contains(strings.ToLower(what), "reyna") {
			what = "reyna " + what
		}
		who = ""
	}
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
		"reyna": true,
	}

	// Action verbs that come AFTER a person's name: "X sent me", "X shared", "X uploaded"
	actionVerbs := []string{" sent ", " shared ", " uploaded ", " gave ", " posted ", " forwarded "}
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
		for _, w := range []string{
			"can you find me", "can you show me", "can you get me", "can you find", "can you",
			"find me", "show me", "get me", "search for", "find", "search",
			"has anyone shared", "do we have", "share", "shared", "upload", "uploaded",
			"sent", "send", "received", "receive", "about", "any", "the", "some",
			"latest", "recently", "recent", "me",
		} {
			what = strings.Replace(what, w, "", -1)
		}
		what = strings.Trim(strings.TrimSpace(what), "?.,! ")
	}

	return who, what, when, why
}

func (c *Classifier) llmParseQuery(query string) (who, what, when, why string) {
	return c.llmParseQueryWithHistory(query, nil)
}

func (c *Classifier) llmParseQueryWithHistory(query string, history []model.ChatMessageContext) (who, what, when, why string) {
	var histSection string
	if len(history) > 0 {
		var histBuf strings.Builder
		histBuf.WriteString("Recent Conversation History:\n")
		for _, h := range history {
			role := "User"
			if h.Role == "assistant" {
				role = "Assistant"
			}
			fmt.Fprintf(&histBuf, "%s: %s\n", role, h.Text)
			if len(h.FileNames) > 0 {
				fmt.Fprintf(&histBuf, "  (Files cited: %s)\n", strings.Join(h.FileNames, ", "))
			}
		}
		histBuf.WriteString("\n")
		histSection = histBuf.String()
	}

	prompt := fmt.Sprintf(`You are a query parser for a file retrieval system covering documents shared in a person's chats.
Parse this natural language query into structured search filters.

%sCurrent Query: "%s"

Rules:
- "who": Extract the PERSON'S NAME if the user is asking about files from a specific sender person. Leave empty if no person mentioned.
  IMPORTANT: The assistant/app itself is named "Reyna". "Reyna" is NEVER a sender person. If the user mentions "Reyna" (e.g. "Reyna script", "Reyna document", "hey Reyna find X"), "Reyna" belongs in "what" if it is part of the topic/document name, or ignored if used as a greeting. NEVER set "who" to "Reyna".
- "what": Extract the SPECIFIC TOPIC, KEYWORD, or SUBJECT being searched.
  CONTEXT RESOLUTION RULE: If the query uses pronouns or follow-up phrases (e.g. "can you find it?", "what does it say?", "explain module 1 from that", "open it", "summarize it", "who sent it?", "send that to me", "where is that exam?"), RESOLVE the referred topic or file from the Conversation History and output that specific topic/filename in "what". If there is no previous context and the user uses generic words like "notes", "files", "stuff", leave this empty.
- "when": Extract time reference as one of: today, yesterday, last_week, this_week, last_month. ONLY extract when an explicit calendar period is specified. Words like "latest", "recent", "newest", "last" indicate sorting order, NOT a time filter; leave "when" empty for them.
- "why": One of: retrieve, search, check_existence, activity_check, qa

Examples:
- "can you find me the latest Reyna script received" → {"who":"","what":"Reyna script","when":"","why":"search"}
- "mohit sent some notes" → {"who":"mohit","what":"","when":"","why":"retrieve"}
- "do we have OS notes?" → {"who":"","what":"OS","when":"","why":"check_existence"}
- "what did priya upload yesterday?" → {"who":"priya","what":"","when":"yesterday","why":"retrieve"}
- "find compiler lab manual" → {"who":"","what":"compiler lab manual","when":"","why":"search"}
- "rakesh shared quantum mechanics pdf" → {"who":"rakesh","what":"quantum mechanics","when":"","why":"retrieve"}

Respond ONLY with JSON, no other text:
{"who":"","what":"","when":"","why":"retrieve"}`, histSection, query)

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

// QASource describes one piece of context (a file) for Notes Q&A — content
// plus social/temporal metadata so the LLM can attribute its answer to a
// specific person and time.
type QASource struct {
	FileName   string
	Content    string
	SenderName string
	Subject    string
	SharedAt   time.Time
}

// QAFollowup carries the previous turn of a multi-turn Q&A conversation.
// Used to thread refinement questions ("tell me more", "explain that part",
// "in simpler words") through to Gemini with the prior context attached.
type QAFollowup struct {
	PrevQuestion string
	PrevAnswer   string
	PrevSources  []string
}

// AnswerFromNotes takes a question + structured QA sources and asks the LLM
// for an attributed answer. Sources include sender name, subject folder, and
// shared-at timestamp so the answer can say "Mohit shared this PDF this
// morning — the Wien bridge oscillator works as follows…".
func (c *Classifier) AnswerFromNotes(question string, sources []QASource) string {
	return c.AnswerFromNotesWithContext(question, sources, nil)
}

// AnswerFromNotesWithContext is the multi-turn variant. If `prev` is non-nil
// the prompt includes the previous question/answer so Gemini can build on it.
func (c *Classifier) AnswerFromNotesWithContext(question string, sources []QASource, prev *QAFollowup) string {
	if !c.IsEnabled() || len(sources) == 0 {
		return "I don't have enough content from your notes to answer that. Make sure files have been shared and extracted."
	}

	var context strings.Builder
	for i, src := range sources {
		content := src.Content
		// Allow much more content per source than before — modern Gemini handles 10k+ tokens easily.
		if len(content) > 8000 {
			content = content[:8000] + "..."
		}
		context.WriteString(fmt.Sprintf("=== SOURCE %d: %s ===\n", i+1, src.FileName))
		if src.SenderName != "" {
			context.WriteString(fmt.Sprintf("Shared by: %s\n", src.SenderName))
		}
		if src.Subject != "" {
			context.WriteString(fmt.Sprintf("Folder: %s\n", src.Subject))
		}
		if !src.SharedAt.IsZero() {
			context.WriteString(fmt.Sprintf("Shared at: %s\n", src.SharedAt.Format("Mon 2006-01-02 15:04 MST")))
		}
		context.WriteString("Content:\n")
		context.WriteString(content)
		context.WriteString("\n\n")
	}

	prompt := fmt.Sprintf(`You are Reyna. You help someone find and understand documents that were shared in their chats, whatever those documents are: invoices, contracts, tickets, records, manuals, notes, anything. You answer like a smart friend, not like a dry assistant. Never assume the person is a student or that the files are course material.

CRITICAL LANGUAGE RULE:
- Detect the language of the QUESTION ITSELF, not the sender names. "rakesh" / "mohit" / "priya" are proper nouns and DO NOT indicate Hindi.
- English question → English answer ONLY.
- Hindi (Devanagari) → Hindi answer.
- Hinglish (Hindi in Roman script like "kya", "hai", "kal", "bheja") → Hinglish answer.
- Bhojpuri / Tamil / Bengali / Marathi / Kannada / Telugu / Malayalam → reply in that language.
- "explain oscillators sent by mohit" is ENGLISH. Reply in English.
- "mohit ne oscillators ke baare me kya bheja" is HINGLISH. Reply in Hinglish.
- Match tone: casual query → casual reply; formal query → formal reply.

CRITICAL TIME RULE:
- Each source has a "Shared at:" line with the exact pre-computed time. Use it verbatim. NEVER compute relative time yourself, NEVER hallucinate "yesterday" or "2 days ago".

CONVERSATION CONTEXT:
- If a "PREVIOUS TURN" block appears below, this is a follow-up to an earlier question. Build on the previous answer — don't repeat its full content. Refine, expand, simplify, or add detail as the new question asks.
- Pronouns like "it", "that", "this", "the formula" refer to things from the previous turn — resolve them from there.
- If the new question clearly changes topic, treat it as fresh and ignore the previous turn.

How to read the question — figure out what they actually want:
- If they ask to "explain / samjhao / batao" — explain in your own words, structured and clear.
- If they ask for "exact / verbatim / hubahu / actual definition / quote / drop kar do" — quote the relevant lines from the source word-for-word, in a code block or blockquote.
- If they ask for a "summary / saar / short me batao" — give a tight bullet summary.
- If they ask "kisne / who / kaun" or "kab / when" — answer with names/dates from the source metadata.
- If they ask "kya bheja / what did X share" — list what the person shared with file names + dates.
- If they're casual ("yo what was that thing about oscillators?") — be casual back.

Source material rules:
- Use ONLY the source content below. NEVER invent facts not in the sources.
- ALWAYS cite which source you used, e.g. "From notes.pdf, shared by Mohit on Mon 2026-08-18 21:14 IST, ..." — take the filename, sender and time verbatim from the SOURCE blocks.
- ATTRIBUTION: a source block only has a "Shared by:" line when we actually know who shared it. If a source has no "Shared by:" line, do NOT name anyone for it. Say "shared in <folder>" or just cite the filename and date. NEVER guess a sender, and never carry a name over from a different source.
- If multiple sources are relevant, weave them together with citations.
- If the sources truly don't contain the answer, say so honestly and suggest a follow-up they could try (e.g. "I don't see that in Mohit's PDF — try asking about [something close that IS in there]").

Formatting:
- Use plain text with markdown — short paragraphs, bullets where helpful, **bold** for key terms, > blockquotes for direct quotes from a source.
- Keep it under ~400 words unless they explicitly asked for full text.
- Do NOT wrap your reply in any envelope or curly braces. Just write the answer directly.

SOURCE MATERIAL:
%s
%s
STUDENT QUESTION: %s

Your answer:`, context.String(), formatQAPrev(prev), question)

	result, err := c.llm.Complete(prompt, 1200)
	if err != nil {
		return "Sorry, I couldn't process that question right now. Try again in a moment."
	}
	return cleanLLMReply(result)
}

// formatQAPrev renders the previous Q&A turn as a context block, or empty
// string if there's no prior turn. The block is wedged between the source
// material and the person's question in the prompt.
func formatQAPrev(prev *QAFollowup) string {
	if prev == nil || prev.PrevQuestion == "" || prev.PrevAnswer == "" {
		return ""
	}
	prevAns := prev.PrevAnswer
	if len(prevAns) > 1500 {
		prevAns = prevAns[:1500] + "..."
	}
	srcLine := ""
	if len(prev.PrevSources) > 0 {
		srcLine = "\nPrevious answer cited: " + strings.Join(prev.PrevSources, ", ")
	}
	return fmt.Sprintf("\n\nPREVIOUS TURN (this is a follow-up — build on it, don't restart):\nQ: %s\nA: %s%s\n", prev.PrevQuestion, prevAns, srcLine)
}

// MatchesQuery sends a PDF (or image) to Gemini along with the user's
// natural-language query and asks "does this document match what they're
// looking for?". Returns (matched, confidence). Used for deep content
// retrieval — when metadata search fails or the user gives content cues
// like "the PDF with the wien bridge diagram".
func (c *Classifier) MatchesQuery(query, fileName, mimeType string, fileData []byte) (bool, float64) {
	if !c.IsEnabled() || len(fileData) == 0 {
		return false, 0
	}
	// Skip files too big for inline doc API
	if len(fileData) > 14*1024*1024 {
		return false, 0
	}
	prompt := fmt.Sprintf(`You are Reyna's content retrieval agent. Someone is searching their own documents with this natural-language query:

QUERY: "%s"

The attached document's filename is: "%s"

Read the document and determine: does this document satisfy what the person is looking for? Consider:
- Specific topics, concepts, or terms mentioned in the query
- Visual cues ("diagram of...", "the figure showing...", "the chart with...")
- Document type ("the PYQ paper", "the lab manual", "the assignment")
- Vague but real recall ("the one about Coulomb's law", "had R1 R2 R3 in a circuit")
- Multi-language queries — interpret intent regardless of language

Respond ONLY with JSON:
{"matches": true/false, "confidence": 0.0-1.0, "snippet": "1-2 sentence reason / quoted excerpt"}`, query, fileName)

	result, err := c.llm.CompleteWithDoc(prompt, fileData, mimeType, 400)
	if err != nil {
		log.Printf("[MATCH] CompleteWithDoc error for %s: %v", fileName, err)
		return false, 0
	}
	var resp struct {
		Matches    bool    `json:"matches"`
		Confidence float64 `json:"confidence"`
		Snippet    string  `json:"snippet"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("[MATCH] parse error for %s: %v (raw: %.150s)", fileName, err, result)
		return false, 0
	}
	return resp.Matches, resp.Confidence
}

// MatchesQueryText is the text-only fallback when we can't send the file
// inline (DOCX, large files). Operates on the cached extracted_content.
func (c *Classifier) MatchesQueryText(query, fileName, content string) (bool, float64) {
	if !c.IsEnabled() || content == "" {
		return false, 0
	}
	if len(content) > 8000 {
		content = content[:8000]
	}
	prompt := fmt.Sprintf(`You are Reyna's content retrieval agent. Someone is searching their own documents with this natural-language query:

QUERY: "%s"

Filename: "%s"
Document summary/content (cached):
%s

Does this document satisfy what the person is asking for? Consider topics, concepts, recall hints in any language.

Respond ONLY with JSON:
{"matches": true/false, "confidence": 0.0-1.0, "snippet": "1-2 sentence reason"}`, query, fileName, content)

	result, err := c.llm.Complete(prompt, 400)
	if err != nil {
		return false, 0
	}
	var resp struct {
		Matches    bool    `json:"matches"`
		Confidence float64 `json:"confidence"`
	}
	result = llm.CleanJSON(result)
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return false, 0
	}
	return resp.Matches, resp.Confidence
}

// cleanLLMReply strips any accidental JSON envelope and code-fence wrappers
// the model may have produced, leaving just the user-facing text.
func cleanLLMReply(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` or ``` ... ``` fences
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	// If the whole thing is a JSON object with an "answer"/"reply"/"text" field,
	// pull that field out. This is defensive — the prompt asks for plain text but
	// the model occasionally still wraps.
	if strings.HasPrefix(s, "{") {
		var obj map[string]interface{}
		cleanJsonStr := s
		if !strings.HasSuffix(cleanJsonStr, "}") {
			cleanJsonStr = cleanJsonStr + "\"}"
		}
		if err := json.Unmarshal([]byte(cleanJsonStr), &obj); err == nil {
			for _, key := range []string{"answer", "reply", "response", "text", "output", "result", "message"} {
				if v, ok := obj[key]; ok {
					if str, ok := v.(string); ok && str != "" {
						return strings.TrimSpace(str)
					}
				}
			}
		}
		// Regex fallback for `"answer": "..."`
		re := regexp.MustCompile(`"(?:answer|reply|response|text|message)"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)`)
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			val := matches[1]
			val = strings.ReplaceAll(val, `\"`, `"`)
			val = strings.ReplaceAll(val, `\n`, "\n")
			return strings.TrimSpace(val)
		}
	}
	return s
}

// GenerateRetrievalReply asks Gemini to write a natural-language summary of
// retrieval results. This replaces the old template-string buildNLPReply with
// a conversational, multi-language, intent-aware response. Falls back to a
// simple template if the LLM call fails.
func (c *Classifier) GenerateRetrievalReply(rawQuery, who, what, when, why string, files []RetrievalFile, driveMatches []RetrievalFile, history []model.ChatMessageContext) SourcedReply {
	if !c.IsEnabled() {
		return SourcedReply{Answer: fallbackRetrievalReply(rawQuery, files, driveMatches, who, what, when)}
	}

	var histSection string
	if len(history) > 0 {
		var histBuf strings.Builder
		histBuf.WriteString("RECENT CONVERSATION HISTORY:\n")
		for _, h := range history {
			role := "User"
			if h.Role == "assistant" {
				role = "Assistant (Reyna)"
			}
			fmt.Fprintf(&histBuf, "%s: %s\n", role, h.Text)
			if len(h.FileNames) > 0 {
				fmt.Fprintf(&histBuf, "  (Files cited: %s)\n", strings.Join(h.FileNames, ", "))
			}
		}
		histBuf.WriteString("\n")
		histSection = histBuf.String()
	}

	var ctx strings.Builder
	if len(files) > 0 {
		ctx.WriteString("FILES FOUND IN REYNA'S DATABASE (captured from WhatsApp groups):\n")
		for i, f := range files {
			if i >= 8 {
				ctx.WriteString(fmt.Sprintf("...and %d more\n", len(files)-8))
				break
			}
			ctx.WriteString(fmt.Sprintf("- %s | folder: %s | sender: %s | shared: %s | summary: %s\n",
				f.Name, f.Folder, f.Sender, f.SharedAt, f.Summary))
		}
		ctx.WriteString("\n")
	}
	if len(driveMatches) > 0 {
		ctx.WriteString("FILES ALREADY ORGANIZED IN GOOGLE DRIVE (not captured by bot):\n")
		for i, m := range driveMatches {
			if i >= 8 {
				ctx.WriteString(fmt.Sprintf("...and %d more\n", len(driveMatches)-8))
				break
			}
			ctx.WriteString(fmt.Sprintf("- %s | drive folder: %s\n", m.Name, m.Folder))
		}
		ctx.WriteString("\n")
	}
	if len(files) == 0 && len(driveMatches) == 0 {
		ctx.WriteString("(no matching files found in database or Drive)\n")
	}

	prompt := fmt.Sprintf(`You are Reyna. You are a personal document assistant helping someone find and understand files in their chats.
Write a natural, conversational reply describing what was found or directly answering their question.

%sCRITICAL LANGUAGE RULE — read this twice:
- Detect the language of the QUERY ITSELF (not the sender names — "rakesh" or "mohit" are proper nouns and do NOT indicate Hindi).
- If the query is written in English, reply ONLY in English.
- If the query is written in Hindi (Devanagari), reply in Hindi.
- If the query is written in Hinglish (Hindi words in Roman script like "kya bheja", "kal", "hain"), reply in Hinglish.
- If the query is in Bhojpuri / Tamil / Bengali / Marathi / Kannada / Telugu / Malayalam, reply in that language.
- Match the tone too — casual query → casual reply; formal query → formal reply.

CONVERSATION CONTEXT & FOLLOW-UP QUESTIONS:
- If recent conversation history is present above, this may be a follow-up (e.g. "can you find it?", "what does it say?", "explain module 1 from that", "who sent it?").
- Connect pronouns ("it", "that", "this file") to the file or topic from recent conversation turns.
- Answer naturally without asking the user to re-specify the file if it was already discussed!

RESPOND ONLY WITH JSON, in exactly this shape and nothing around it:
{"answer": "...", "quotes": [{"file": "exact filename", "quote": "verbatim text copied from that file's summary"}]}

- "answer" is what the user asked for and nothing else. No raw filenames list. Write one or two clean, natural conversational sentences, under 50 words, plain text with no markdown and no bullets.
- "quotes" is the evidence. Copy the lines from the summary that contain the answer, character for character. Do not paraphrase, do not tidy, do not translate. If the answer came from a table row, the quote is that row.
- Every quote must appear word for word in a summary above. A quote that is only a filename is not evidence and will be discarded.
- If nothing above answers the question, say so plainly in "answer" and return an empty "quotes" list. Never invent either one.

ANSWER THE QUESTION FIRST:
- Each file carries a "summary:" holding text taken from inside the document.
- If the query asks something factual and the answer is in there, SAY THE ANSWER directly in plain sentences.
  Example: "which room is the operating systems exam in" → "The Operating Systems exam is scheduled to be held in room B-207."
  Example: "can you find me the latest Reyna script received" → "The latest Reyna script covers the meeting and presentation deck for SIH."
  Example: "can you find it?" (after asking about C programming) → "I found the C programming lab manual in your Lab folder."
- Do not dump lists of filenames in "answer". Put the evidence in "quotes" instead.
- Never invent a fact that is not in a summary.

CRITICAL TIME RULE:
- The "shared:" line in each file's metadata below is the GROUND TRUTH. Use it verbatim if mentioning time.

CRITICAL ATTRIBUTION RULE:
- "sender:" is empty for files where we do not know who shared them. For those, NEVER invent a sender. If the user asked about a specific person and some results have no sender, say you found the file but the original sender is unconfirmed.

ORIGINAL QUERY: %s
PARSED — who:%s what:%s when:%s why:%s

%s
Your reply:`, histSection, rawQuery, who, what, when, why, ctx.String())

	result, err := c.llm.Complete(prompt, 900)
	if err != nil || result == "" {
		return SourcedReply{Answer: fallbackRetrievalReply(rawQuery, files, driveMatches, who, what, when)}
	}

	var parsed struct {
		Answer string `json:"answer"`
		Quotes []struct {
			File  string `json:"file"`
			Quote string `json:"quote"`
		} `json:"quotes"`
	}
	if jerr := json.Unmarshal([]byte(llm.CleanJSON(result)), &parsed); jerr != nil || parsed.Answer == "" {
		// The model wrote prose instead of JSON. Its answer is still worth
		// showing; only the evidence is lost, and an answer with no sources
		// button is better than an error.
		return SourcedReply{Answer: cleanLLMReply(result)}
	}

	out := SourcedReply{Answer: cleanLLMReply(parsed.Answer)}
	for _, q := range parsed.Quotes {
		if strings.TrimSpace(q.Quote) == "" {
			continue
		}
		out.Quotes = append(out.Quotes, QuotedSource{FileName: q.File, Quote: q.Quote})
	}
	return out
}

// RetrievalFile is a flattened view of either a DB file or a Drive match for
// passing into GenerateRetrievalReply without coupling to model.File.
type RetrievalFile struct {
	ID       int64
	Name     string
	Folder   string
	Sender   string
	SharedAt string
	Summary  string
}

// SourcedReply is an answer and the passages it rests on.
type SourcedReply struct {
	Answer string
	Quotes []QuotedSource
}

// QuotedSource is one passage the model says it used.
//
// Unverified at this point. The caller checks each quote actually appears in
// the file's stored text before it is shown, because a model asked to produce
// evidence is being invited to invent some.
type QuotedSource struct {
	FileName string
	Quote    string
}

func fallbackRetrievalReply(rawQuery string, files, driveMatches []RetrievalFile, who, what, when string) string {
	if len(files) == 0 && len(driveMatches) == 0 {
		msg := "Couldn't find anything matching that"
		if who != "" {
			msg += " from " + who
		}
		if what != "" {
			msg += " about \"" + what + "\""
		}
		return msg + ". Try rephrasing or being more specific."
	}
	var b strings.Builder
	if len(files) > 0 {
		b.WriteString(fmt.Sprintf("Found %d file(s) shared in your groups:\n", len(files)))
		for i, f := range files {
			if i >= 5 {
				b.WriteString(fmt.Sprintf("...and %d more\n", len(files)-5))
				break
			}
			b.WriteString(fmt.Sprintf("• **%s** — %s", f.Name, f.Folder))
			if f.Sender != "" {
				b.WriteString(" (by " + f.Sender + ")")
			}
			b.WriteString("\n")
		}
	}
	if len(driveMatches) > 0 {
		b.WriteString(fmt.Sprintf("\nPlus %d already in your Drive:\n", len(driveMatches)))
		for i, m := range driveMatches {
			if i >= 5 {
				b.WriteString(fmt.Sprintf("...and %d more\n", len(driveMatches)-5))
				break
			}
			b.WriteString(fmt.Sprintf("• **%s** — in %s/\n", m.Name, m.Folder))
		}
	}
	return b.String()
}
