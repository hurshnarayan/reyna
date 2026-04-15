package nlp

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/integrations/llm"
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

		prompt := fmt.Sprintf(`You are a document analysis and classification agent for a university study group's shared file system. Analyze the attached document AND its sharing context, then:
1. "content": detailed description of topics, concepts, chapters, key terms inside the document (max 800 chars). Read the actual document — do not guess from the filename.
2. "summary": one-line summary (max 100 chars).
3. "folder": classify into the best folder from: [%s]

   STRICT folder rules:
   - Use an existing folder ONLY if the document is unambiguously about that exact subject. "Close enough" or "topically adjacent" is NOT a match. Each university subject is its own folder.
   - Examples of WRONG matches: putting CAED (Computer Aided Engineering Drawing) under "Engineering Science"; putting DBMS under "Computer Science"; putting Operating Systems under "Computer Networks"; putting a Compiler Design PDF under "Programming". These are DIFFERENT subjects — never lump them.
   - Examples of CORRECT matches: a Compiler Design lab manual → existing "Compiler Design" folder; a DBMS PYQ → existing "DBMS" folder.
   - If no existing folder is an exact subject match, INVENT a new clean 2–3 word Title Case folder named after the document's actual subject (e.g. "CAED", "Engineering Drawing", "Compiler Design", "Operating Systems"). Recognise common Indian engineering course codes as their own subject: CAED, ESC, BESC, BCS, BEC, BCSL, etc. — these are distinct subjects, not generic "Engineering".
   - NEVER return "None", "Unsorted", "Unknown", "Misc", "Other", "General", "Engineering", "Science" or any vague umbrella. Always pick or invent a SPECIFIC subject folder.

4. "is_new": true if you invented the folder, false if it already exists in the list above.
5. "confidence": 0.0–1.0. Lower confidence (≤0.6) if you had to invent the folder or if the subject is ambiguous.

Use the sender, group, and time context as supporting signals (e.g. who tends to share which subject, recent exam season) but the document content is the primary signal.

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

	prompt := fmt.Sprintf(`You are a document analysis and classification agent for a university study group's shared file system. The document is a %s — its full text content (extracted from the file) is provided below. Analyze it and return:
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

	// 1. Office docs (DOCX / PPTX / XLSX) — read the actual text via the
	//    stdlib zip+xml extractor. Gemini doesn't accept these formats
	//    natively; without this step DOCX summaries are filename-guesses
	//    that look like a textbook outline and aren't grounded in the
	//    real content.
	if len(fileData) > 0 && IsOfficeDoc(mimeType) {
		if rawText, err := ExtractOfficeText(fileData, mimeType, 8000); err == nil && len(rawText) > 40 {
			return rawText, ""
		}
		// Couldn't read the docx (corrupt / unusual variant) — fall through
		// to filename-only inference rather than fabricating from the name.
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

	// 2. PDFs / images — Gemini multimodal can read these inline.
	//    Never slice raw bytes — that corrupts the file. Only send if it
	//    fits inline.
	if len(fileData) > 0 && len(fileData) <= geminiInlineMaxBytes && (strings.Contains(mimeType, "pdf") || strings.Contains(mimeType, "image")) {
		result, err = c.llm.CompleteWithDoc(prompt, fileData, mimeType, 1500)
	} else {
		// 3. Other types (or no file bytes available) — there's literally
		//    nothing to ground on, drop straight to filename inference
		//    (which marks itself as such so QA stays honest).
		return c.extractFromFilename(fileName, mimeType, fileSize)
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
// FilenameOnlyMarker prefixes content that was inferred from the filename
// alone (not extracted from the actual file body). The QA prompt looks for
// this so the answering LLM can refuse to fabricate a detailed summary
// from a stub. Without this marker the QA model treats the inferred
// description as if it were real document content and synthesises
// confident-sounding bullet lists from nothing.
const FilenameOnlyMarker = "[FILENAME-ONLY-INFERENCE] "

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
	// Tag the content so downstream Q&A knows this is a filename-based
	// guess, not real document text. Summary is left clean — it's a
	// one-liner used in card UIs where the distinction doesn't matter.
	if resp.Content != "" {
		resp.Content = FilenameOnlyMarker + resp.Content
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

	prompt := fmt.Sprintf(`You are Reyna — a friendly study assistant living inside a WhatsApp study group. You help students find and understand things from notes their groupmates shared. You answer like a smart friend, not like a dry assistant.

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
- If they ask for a "summary / summarize / summarise / saar / short me batao" — write a REAL content summary, not a description of the document. That means:
    • Pull out the actual topics, definitions, formulas, theorems, examples, and key points taught in the source.
    • Use 4–8 tight bullets organised by sub-topic. Each bullet should teach the reader something concrete (a definition, a formula, a result), not describe what the document "covers".
    • BAD summary: "This document discusses transactions and normalization in databases." (meta-description, useless)
    • GOOD summary: "• Normalization progressively removes redundancy: 1NF (atomic), 2NF (no partial deps), 3NF (no transitive deps), BCNF (every determinant is a key). • A transaction is ACID: atomicity via logging, consistency via constraints, isolation via locks, durability via WAL. • 2PL: grow-phase acquires locks, shrink-phase releases them; prevents cascading aborts when strict." (actual teaching)
    • If the source is long, prioritise the most important 5–7 ideas, not every section heading.
- If they ask "kisne / who / kaun" or "kab / when" — answer with names/dates from the source metadata.
- If they ask "kya bheja / what did X share" — list what the person shared with file names + dates.
- If they're casual ("yo what was that thing about oscillators?") — be casual back.

Source material rules:
- Use ONLY the source content below. NEVER invent facts not in the sources.
- If a SOURCE's Content begins with "[FILENAME-ONLY-INFERENCE]", we DO NOT have the actual document text — only an inference from its filename. Do NOT pretend to summarise or quote from it. Instead say honestly: "I don't have the contents of <filename> extracted yet, only its title. From the title, it looks like it's about <one short clause>. Try again later, or share it again so I can re-extract." Treat ALL [FILENAME-ONLY-INFERENCE] sources this way.
- ALWAYS cite which source you used. Format: "From %sshared by %s on %s, …" — use the real filename, sender, and shared-at time from the SOURCE blocks.
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

Your answer:`, "", "", "", context.String(), formatQAPrev(prev), question)

	result, err := c.llm.Complete(prompt, 1200)
	if err != nil {
		return "Sorry, I couldn't process that question right now. Try again in a moment."
	}
	return cleanLLMReply(result)
}

// formatQAPrev renders the previous Q&A turn as a context block, or empty
// string if there's no prior turn. The block is wedged between the source
// material and the student question in the prompt.
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
	prompt := fmt.Sprintf(`You are Reyna's content retrieval agent. The student is searching their study notes with this natural-language query:

QUERY: "%s"

The attached document's filename is: "%s"

Read the document and determine: does this document satisfy what the student is looking for? Consider:
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
	prompt := fmt.Sprintf(`You are Reyna's content retrieval agent. The student is searching their study notes with this natural-language query:

QUERY: "%s"

Filename: "%s"
Document summary/content (cached):
%s

Does this document satisfy what the student is asking for? Consider topics, concepts, recall hints in any language.

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
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(s), &obj); err == nil {
			for _, key := range []string{"answer", "reply", "response", "text", "output", "result", "message"} {
				if v, ok := obj[key]; ok {
					if str, ok := v.(string); ok && str != "" {
						return strings.TrimSpace(str)
					}
				}
			}
		}
	}
	return s
}

// GenerateRetrievalReply asks Gemini to write a natural-language summary of
// retrieval results. This replaces the old template-string buildNLPReply with
// a conversational, multi-language, intent-aware response. Falls back to a
// simple template if the LLM call fails.
func (c *Classifier) GenerateRetrievalReply(rawQuery, who, what, when, why string, files []RetrievalFile, driveMatches []RetrievalFile) string {
	if !c.IsEnabled() {
		return fallbackRetrievalReply(rawQuery, files, driveMatches, who, what, when)
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

	prompt := fmt.Sprintf(`You are Reyna — a smart, friendly study assistant inside a WhatsApp study group. The student just searched their notes. Write a natural, conversational reply describing what was found.

CRITICAL LANGUAGE RULE — read this twice:
- Detect the language of the QUERY ITSELF (not the sender names — "rakesh" or "mohit" are proper nouns and do NOT indicate Hindi).
- If the query is written in English, reply ONLY in English.
- If the query is written in Hindi (Devanagari), reply in Hindi.
- If the query is written in Hinglish (Hindi words in Roman script like "kya bheja", "kal", "hain"), reply in Hinglish.
- If the query is in Bhojpuri / Tamil / Bengali / Marathi / Kannada / Telugu / Malayalam, reply in that language.
- Match the tone too — casual query → casual reply; formal query → formal reply.
- A query like "rakesh shared notes or what?" is ENGLISH. Reply in English.
- A query like "rakesh ne kya bheja?" is HINGLISH. Reply in Hinglish.

Read the original query to understand intent:
- "find / dhundo / dikhao" → list the files clearly
- "kya bheja / what did X share" → list with sender + time
- "do we have / hai kya" → confirm yes/no, then list
- "what's new / kuch naya" → recency-focused list
- Activity-check questions → confirm with names + counts
If results are empty, say so honestly and suggest a different phrasing or topic.

CRITICAL TIME RULE:
- The "shared:" line in each file's metadata below is the GROUND TRUTH — it already contains the relative time AND the absolute time. Use it VERBATIM.
- NEVER compute or guess relative times yourself ("2 days ago", "this morning"). NEVER override what's written.
- If the metadata says "5 minute(s) ago", say "5 minutes ago" — not "today" or "earlier".

Formatting:
- Plain text with light markdown — bullets, **bold** for filenames, short paragraphs.
- Mention if a file is from "Drive" vs "shared in WhatsApp".
- Under 200 words. No envelopes, no curly braces — just write the reply.

ORIGINAL QUERY: %s
PARSED — who:%s what:%s when:%s why:%s

%s
Your reply:`, rawQuery, who, what, when, why, ctx.String())

	result, err := c.llm.Complete(prompt, 600)
	if err != nil || result == "" {
		return fallbackRetrievalReply(rawQuery, files, driveMatches, who, what, when)
	}
	return cleanLLMReply(result)
}

// RetrievalFile is a flattened view of either a DB file or a Drive match for
// passing into GenerateRetrievalReply without coupling to model.File.
type RetrievalFile struct {
	Name     string
	Folder   string
	Sender   string
	SharedAt string
	Summary  string
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
