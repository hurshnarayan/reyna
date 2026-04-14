package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/nlp"
	"github.com/hurshnarayan/reyna/internal/search"
)

// ── Reyna Live — Vapi voice tool webhooks ──
//
// Every endpoint here is called by the Vapi voice agent during a live call.
// Vapi POSTs a JSON body with a `message.functionCall.parameters` object
// containing the tool arguments. We also accept a shared secret in the
// `X-Reyna-Voice-Secret` header, matching VAPI_WEBHOOK_SECRET.
//
// Tool auth identifies the caller by phone number (Vapi passes the caller's
// phone, or the Reyna user phone we stash in assistant metadata for web
// calls). Phone → Reyna user → accessible groups.

// vapiParams extracts the tool parameters + toolCallId from Vapi's webhook
// body. Returns (params, toolName, toolCallId, err). The toolCallId is
// used to shape the response so Vapi can correlate it back to the pending
// function call — required by the modern format.
//
// Shapes supported:
//   A. message.toolCalls[0].function.arguments (current) — arguments is a
//      JSON-encoded STRING that we parse
//   B. message.toolCallList[0].function.arguments (variant naming)
//   C. message.functionCall.parameters (legacy, object)
//   D. flat body (used from curl during debugging)
func vapiParams(r *http.Request) (map[string]any, string, string, error) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, "", "", fmt.Errorf("invalid json: %w", err)
	}

	var params map[string]any
	toolName := ""
	toolCallID := ""
	var call map[string]any

	if msg, ok := raw["message"].(map[string]any); ok {
		if c, ok := msg["call"].(map[string]any); ok {
			call = c
		}

		pickFromCalls := func(arr []any) {
			if len(arr) == 0 || params != nil {
				return
			}
			first, _ := arr[0].(map[string]any)
			if first == nil {
				return
			}
			if id, ok := first["id"].(string); ok {
				toolCallID = id
			}
			if fn, ok := first["function"].(map[string]any); ok {
				toolName, _ = fn["name"].(string)
				switch a := fn["arguments"].(type) {
				case string:
					_ = json.Unmarshal([]byte(a), &params)
				case map[string]any:
					params = a
				}
			}
		}
		if tc, ok := msg["toolCalls"].([]any); ok {
			pickFromCalls(tc)
		}
		if params == nil {
			if tc, ok := msg["toolCallList"].([]any); ok {
				pickFromCalls(tc)
			}
		}
		if params == nil {
			if fc, ok := msg["functionCall"].(map[string]any); ok {
				if p, ok := fc["parameters"].(map[string]any); ok {
					params = p
				}
				if toolName == "" {
					toolName, _ = fc["name"].(string)
				}
			}
		}
	}
	if params == nil {
		params = raw
	}

	if call != nil {
		phone := phoneFromCall(call)
		if phone != "" {
			existing, _ := params["user_phone"].(string)
			if strings.TrimSpace(existing) == "" {
				params["user_phone"] = phone
			}
		}
	}
	if p, ok := params["user_phone"].(string); ok {
		params["user_phone"] = strings.TrimSpace(strings.ReplaceAll(p, " ", ""))
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	log.Printf("[VOICE-PARAMS] tool=%q toolCallId=%q paramKeys=%v", toolName, toolCallID, keys)

	return params, toolName, toolCallID, nil
}

// phoneFromCall digs through the Vapi call object for the caller's phone.
// Checks variableValues and metadata on assistantOverrides (set by the web
// SDK's v.start() call), then falls back to the customer.number field
// used for real PSTN calls.
func phoneFromCall(call map[string]any) string {
	if ao, ok := call["assistantOverrides"].(map[string]any); ok {
		if vv, ok := ao["variableValues"].(map[string]any); ok {
			if p, ok := vv["user_phone"].(string); ok && p != "" {
				return p
			}
		}
		if md, ok := ao["metadata"].(map[string]any); ok {
			if p, ok := md["user_phone"].(string); ok && p != "" {
				return p
			}
		}
	}
	if cust, ok := call["customer"].(map[string]any); ok {
		if p, ok := cust["number"].(string); ok && p != "" {
			return p
		}
	}
	return ""
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
func getInt64(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// voiceAuth verifies the shared webhook secret and resolves the caller to
// a Reyna user by phone. Returns the user or writes a 401 and returns nil.
func (s *Server) voiceAuth(w http.ResponseWriter, r *http.Request, params map[string]any) *model.User {
	if s.cfg.VapiWebhookSecret != "" {
		got := r.Header.Get("X-Reyna-Voice-Secret")
		if got != s.cfg.VapiWebhookSecret {
			http.Error(w, `{"error":"invalid voice secret"}`, 401)
			return nil
		}
	}
	phone := strings.TrimSpace(getString(params, "user_phone"))
	log.Printf("[VOICE-AUTH] incoming user_phone=%q (len=%d)", phone, len(phone))
	if phone == "" {
		// Dev fallback: if only ONE user exists in the DB and no phone came
		// through, use them. Covers the single-user dev/demo case where the
		// web SDK didn't stash a phone. Prints a warning so we notice.
		if only := s.store.OnlyUserIfSingle(); only != nil {
			log.Printf("[VOICE-AUTH] no user_phone — falling back to the only user in DB (id=%d phone=%q)", only.ID, only.Phone)
			s.store.AutoLinkUserToGroups(only.ID, only.Phone)
			return only
		}
		http.Error(w, `{"error":"user_phone required"}`, 400)
		return nil
	}
	user, err := s.store.GetUserByPhone(phone)
	if err != nil {
		// Try tolerant match — strip + / spaces, compare suffix.
		if u := s.store.FindUserByPhoneLoose(phone); u != nil {
			log.Printf("[VOICE-AUTH] exact phone miss, loose match → user_id=%d phone=%q", u.ID, u.Phone)
			s.store.AutoLinkUserToGroups(u.ID, u.Phone)
			return u
		}
		log.Printf("[VOICE-AUTH] no user for phone %q (exact + loose both missed)", phone)
		http.Error(w, `{"error":"user not found"}`, 404)
		return nil
	}
	s.store.AutoLinkUserToGroups(user.ID, user.Phone)
	log.Printf("[VOICE-AUTH] resolved user_id=%d phone=%q groups=%v", user.ID, user.Phone, s.store.GetUserGroupIDs(user.ID))
	return user
}

// writeVoiceResult sends back a Vapi-compatible tool result.
// Modern Vapi expects `{ "results": [{ "toolCallId": "...", "result": ... }] }`
// so the platform can correlate the reply to the pending function call.
// We also include the legacy flat `result` field at the top so older
// assistant schema versions still work.
func writeVoiceResult(w http.ResponseWriter, toolCallID string, payload any) {
	// Vapi prefers the result body to be a string; stringify structured
	// payloads so the assistant LLM can consume them. We keep the raw
	// object too for clients that can read it.
	resultStr := ""
	if b, err := json.Marshal(payload); err == nil {
		resultStr = string(b)
	}
	if toolCallID == "" {
		// Legacy flat shape — kept for curl debugging.
		json.NewEncoder(w).Encode(map[string]any{"result": payload})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"results": []map[string]any{
			{"toolCallId": toolCallID, "result": resultStr},
		},
		// Legacy field for any assistant still on the old schema.
		"result": payload,
	})
}

// GET /api/voice/config — used by the dashboard "Call Reyna" button to
// bootstrap the Vapi web SDK. Returns the public key + assistant ID. The
// webhook secret is NEVER returned here (server-side only).
func (s *Server) handleVoiceConfig(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"public_key":   s.cfg.VapiPublicKey,
		"assistant_id": s.cfg.VapiAssistantID,
		"enabled":      s.cfg.VapiPublicKey != "" && s.cfg.VapiAssistantID != "",
	})
}

// POST /api/voice/tools/recall-search  — { user_phone, query, limit? }
func (s *Server) handleVoiceRecallSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	rawQuery := strings.TrimSpace(getString(params, "query"))
	if rawQuery == "" {
		http.Error(w, `{"error":"query required"}`, 400)
		return
	}
	// Undo common speech-recognition mishears before searching.
	query := normalizeVoiceQuery(rawQuery)
	if query != rawQuery {
		log.Printf("[VOICE] normalized %q → %q", rawQuery, query)
	}
	limit := int(getInt64(params, "limit"))
	if limit <= 0 {
		limit = 5
	}

	groupIDs := s.store.GetUserGroupIDs(user.ID)
	results := []map[string]any{}
	seen := map[int64]bool{}
	addFile := func(f *model.File, score float64) {
		if f == nil || seen[f.ID] {
			return
		}
		seen[f.ID] = true
		results = append(results, formatFileForVoice(f, score))
	}

	who, what, when, _ := s.classifier.ParseNLPQuery(query)
	if what == "" {
		what = query
	}
	var sinceTime *time.Time
	switch when {
	case "today":
		t := time.Now(); t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()); sinceTime = &t
	case "yesterday":
		t := time.Now().AddDate(0, 0, -1); t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()); sinceTime = &t
	case "last_week", "this_week":
		t := time.Now().AddDate(0, 0, -7); sinceTime = &t
	case "last_month":
		t := time.Now().AddDate(0, -1, 0); sinceTime = &t
	}

	// Fuzzy-map WHO onto an actually-known sender. Handles "Krish"/"Harsh",
	// "Mohit"/"Rohit", etc. — ASR confuses phonetically similar names often.
	knownSenders := s.store.DistinctSenderNames(groupIDs)
	who = strings.TrimSpace(who)
	if who != "" {
		if matched := closestSender(who, knownSenders); matched != "" && !strings.EqualFold(matched, who) {
			log.Printf("[VOICE] who=%q fuzzy→%q (from senders %v)", who, matched, knownSenders)
			who = matched
		}
	}

	if who != "" {
		nlpFiles, _ := s.store.SearchFilesNLP(groupIDs, who, what, sinceTime, limit)
		for _, f := range nlpFiles {
			addFile(&f, 0)
		}
		if len(results) == 0 {
			looseFiles, _ := s.store.SearchFilesNLP(groupIDs, who, "", nil, limit)
			for _, f := range looseFiles {
				addFile(&f, 0)
			}
		}
	}

	if len(results) < limit && s.search.IsEnabled() {
		hits, err := s.search.SearchFiles(query, groupIDs, limit)
		if err != nil {
			log.Printf("[VOICE] recall-search qdrant: %v", err)
		}
		for _, h := range hits {
			f, err := s.store.GetFileByID(h.FileID)
			if err != nil || f == nil {
				continue
			}
			addFile(f, h.Score)
		}
	}

	// Content keyword — only when the user DIDN'T ask for a specific
	// person. Showing random content hits for "files by Krish" led to the
	// assistant claiming unrelated files were "related to Krish".
	if len(results) == 0 && who == "" {
		kw, _ := s.store.SearchFilesContent(groupIDs, what, limit)
		for _, f := range kw {
			addFile(&f, 0)
		}
	}

	// No silent "recent files" fallback. An empty result set stays empty
	// so the LLM honestly says "I didn't find anything from Krish" instead
	// of listing random files. knownSenders lets it suggest alternatives.
	writeVoiceResult(w, toolCallID, map[string]any{
		"query":             query,
		"parsed":            map[string]any{"who": who, "what": what, "when": when},
		"count":             len(results),
		"results":           results,
		"no_matches":        len(results) == 0,
		"senders_available": knownSenders,
	})
}

// POST /api/voice/tools/recall-ask  — { user_phone, question, file_id? }
// Reuses the full QA pipeline (including memory injection).
func (s *Server) handleVoiceRecallAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	rawQuestion := strings.TrimSpace(getString(params, "question"))
	if rawQuestion == "" {
		http.Error(w, `{"error":"question required"}`, 400)
		return
	}
	question := normalizeVoiceQuery(rawQuestion)
	if question != rawQuestion {
		log.Printf("[VOICE] normalized question %q → %q", rawQuestion, question)
	}
	fileID := getInt64(params, "file_id")
	groupIDs := s.store.GetUserGroupIDs(user.ID)

	var qaSources []nlp.QASource
	if fileID > 0 {
		// Direct file — the assistant remembered a prior search result.
		f, err := s.store.GetFileByID(fileID)
		if err == nil && f != nil {
			content := s.store.GetFileExtractedContent([]int64{f.ID})[f.ID]
			if content == "" {
				// Lazy-extract if cache is empty.
				if data, derr := s.drive.GetLocalFileData(f.ID); derr == nil && len(data) > 0 {
					content, _ = s.classifier.ExtractContent(f.FileName, f.MimeType, f.FileSize, data)
				}
			}
			if content != "" {
				qaSources = append(qaSources, nlp.QASource{
					FileName: f.FileName, Content: content,
					SenderName: f.SharedByName, Subject: f.Subject,
					SharedAt: f.CreatedAt,
				})
			}
		}
	} else {
		// No specific file — search then QA.
		relevant, _ := s.store.SearchFilesContent(groupIDs, question, 3)
		if s.search.IsEnabled() {
			if hits, _ := s.search.SearchFiles(question, groupIDs, 3); len(hits) > 0 {
				seen := map[int64]bool{}
				for _, f := range relevant {
					seen[f.ID] = true
				}
				for _, h := range hits {
					if seen[h.FileID] {
						continue
					}
					if f, err := s.store.GetFileByID(h.FileID); err == nil && f != nil {
						relevant = append(relevant, *f)
					}
				}
			}
		}
		for _, f := range relevant {
			content := s.store.GetFileExtractedContent([]int64{f.ID})[f.ID]
			if content == "" {
				continue
			}
			qaSources = append(qaSources, nlp.QASource{
				FileName: f.FileName, Content: content,
				SenderName: f.SharedByName, Subject: f.Subject,
				SharedAt: f.CreatedAt,
			})
			if len(qaSources) >= 3 {
				break
			}
		}
	}

	// Always inject Memory.
	memSources := s.collectMemorySources(user.ID, question)
	qaSources = append(memSources, qaSources...)

	if len(qaSources) == 0 {
		writeVoiceResult(w, toolCallID, map[string]any{
			"answer": "I couldn't find anything relevant in your notes or memory. Try sharing the file first, or asking differently.",
		})
		return
	}
	answer := s.classifier.AnswerFromNotesWithContext(question, qaSources, nil)
	sourceNames := []string{}
	for _, q := range qaSources {
		sourceNames = append(sourceNames, q.FileName)
	}
	writeVoiceResult(w, toolCallID, map[string]any{
		"answer":  answer,
		"sources": sourceNames,
	})
}

// POST /api/voice/tools/list-recent  — { user_phone, days?, limit? }
func (s *Server) handleVoiceListRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	limit := int(getInt64(params, "limit"))
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	groupIDs := s.store.GetUserGroupIDs(user.ID)
	files, _ := s.store.GetGroupsFiles(groupIDs, limit)

	results := []map[string]any{}
	for _, f := range files {
		results = append(results, formatFileForVoice(&f, 0))
	}
	writeVoiceResult(w, toolCallID, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

// POST /api/voice/tools/list-memories  — { user_phone }
func (s *Server) handleVoiceListMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	memories, _ := s.store.ListMemories(user.ID)
	out := make([]map[string]any, 0, len(memories))
	for _, m := range memories {
		out = append(out, map[string]any{
			"id":        m.ID,
			"title":     m.Title,
			"is_active": m.IsActive,
			"preview":   firstNRunes(m.Content, 120),
		})
	}
	writeVoiceResult(w, toolCallID, map[string]any{
		"count":    len(out),
		"memories": out,
	})
}

// POST /api/voice/tools/add-memory  — { user_phone, title, content, always_include? }
// The killer demo moment: create memories by voice.
func (s *Server) handleVoiceAddMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	title := strings.TrimSpace(getString(params, "title"))
	content := strings.TrimSpace(getString(params, "content"))
	if title == "" || content == "" {
		http.Error(w, `{"error":"title and content required"}`, 400)
		return
	}
	alwaysInc, _ := params["always_include"].(bool)
	m := &model.UserMemory{
		UserID: user.ID, Title: title, Content: content,
		Source: "voice", IsActive: true, AlwaysInclude: alwaysInc,
	}
	created, err := s.store.CreateMemory(m)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	if s.search != nil && s.search.IsEnabled() {
		go func() {
			meta := search.MemoryMetadata{MemoryID: created.ID, UserID: created.UserID, Title: created.Title}
			if err := s.search.IndexMemory(meta, created.Content); err != nil {
				log.Printf("[VOICE] index new memory: %v", err)
			}
		}()
	}
	writeVoiceResult(w, toolCallID, map[string]any{
		"id":      created.ID,
		"title":   created.Title,
		"message": "Saved. I'll remember this.",
	})
}

// POST /api/voice/tools/toggle-memory  — { user_phone, memory_id, is_active }
func (s *Server) handleVoiceToggleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	memID := getInt64(params, "memory_id")
	active, _ := params["is_active"].(bool)
	if err := s.store.ToggleMemory(memID, user.ID, active); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	// Sync Qdrant with the new state.
	if m, _ := s.store.GetMemory(memID, user.ID); m != nil {
		go s.indexMemoryAsync(m)
	}
	msg := "Okay, I'll forget that for now."
	if active {
		msg = "Okay, I'll remember that again."
	}
	writeVoiceResult(w, toolCallID, map[string]any{"ok": true, "is_active": active, "message": msg})
}

// POST /api/voice/tools/commit-staged  — { user_phone, group_wa_id? }
// Voice shortcut for "commit the staged files". Delegates to the existing
// commit path. If group_wa_id is omitted, commits across all the user's
// groups (handy for a global "commit everything" voice command).
func (s *Server) handleVoiceCommitStaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	params, _, toolCallID, err := vapiParams(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	user := s.voiceAuth(w, r, params)
	if user == nil {
		return
	}
	groupWAID := strings.TrimSpace(getString(params, "group_wa_id"))
	var groupIDs []int64
	if groupWAID != "" {
		if g, err := s.store.GetGroupByWAID(groupWAID); err == nil {
			groupIDs = []int64{g.ID}
		}
	} else {
		groupIDs = s.store.GetUserGroupIDs(user.ID)
	}
	totalCommitted := int64(0)
	for _, gid := range groupIDs {
		n, err := s.store.CommitFiles(gid)
		if err == nil {
			totalCommitted += n
		}
	}
	writeVoiceResult(w, toolCallID, map[string]any{
		"committed": totalCommitted,
		"message":   fmt.Sprintf("Committed %d staged file(s).", totalCommitted),
	})
}

// POST /api/bot/voice-note — WhatsApp bot forwards a transcribed voice note.
// Body: { group_wa_id, user_phone, user_name, transcript }.
// Runs the transcript through Recall (QA path) and returns a spoken answer
// the bot can either TTS back or reply with as text.
func (s *Server) handleBotVoiceNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req struct {
		GroupWAID string `json:"group_wa_id"`
		UserPhone string `json:"user_phone"`
		UserName  string `json:"user_name"`
		Transcript string `json:"transcript"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	transcript := strings.TrimSpace(req.Transcript)
	if transcript == "" {
		http.Error(w, `{"error":"transcript required"}`, 400)
		return
	}
	if n := normalizeVoiceQuery(transcript); n != transcript {
		log.Printf("[VOICE-NOTE] normalized %q → %q", transcript, n)
		transcript = n
	}

	// Reuse the handleNotesQA machinery by forging an internal request body.
	qaReq := model.NotesQARequest{
		Question:  transcript,
		GroupWAID: req.GroupWAID,
		UserPhone: req.UserPhone,
	}
	qaBody, _ := json.Marshal(qaReq)

	// Route through the QA handler so memory + semantic search kick in.
	// We do it by function call rather than HTTP to avoid re-entering the
	// middleware stack.
	answer, sources := s.answerTranscript(qaReq)
	log.Printf("[VOICE-NOTE] %s asked %q → %s (sources: %v)", req.UserName, transcript, firstNRunes(answer, 80), sources)

	json.NewEncoder(w).Encode(map[string]any{
		"transcript": transcript,
		"answer":     answer,
		"sources":    sources,
		"raw_qa":     string(qaBody), // handy for bot-side debugging; remove in prod
	})
}

// answerTranscript is an internal helper that reuses the QA pipeline from a
// transcript. It's cheaper to call a helper than to synthesize an HTTP round
// trip through our own server.
func (s *Server) answerTranscript(req model.NotesQARequest) (string, []string) {
	// Resolve user + groups.
	var user *model.User
	if req.UserPhone != "" {
		user, _ = s.store.GetUserByPhone(req.UserPhone)
	}
	var groupIDs []int64
	if req.GroupWAID != "" {
		if g, err := s.store.GetGroupByWAID(req.GroupWAID); err == nil {
			groupIDs = []int64{g.ID}
		}
	}
	if len(groupIDs) == 0 && user != nil {
		groupIDs = s.store.GetUserGroupIDs(user.ID)
	}

	// Find candidate files.
	files, _ := s.store.SearchFilesContent(groupIDs, req.Question, 3)
	if s.search.IsEnabled() {
		if hits, _ := s.search.SearchFiles(req.Question, groupIDs, 3); len(hits) > 0 {
			seen := map[int64]bool{}
			for _, f := range files {
				seen[f.ID] = true
			}
			for _, h := range hits {
				if seen[h.FileID] {
					continue
				}
				if f, err := s.store.GetFileByID(h.FileID); err == nil && f != nil {
					files = append(files, *f)
				}
			}
		}
	}

	var qaSources []nlp.QASource
	sourceNames := []string{}
	for _, f := range files {
		content := s.store.GetFileExtractedContent([]int64{f.ID})[f.ID]
		if content == "" {
			continue
		}
		qaSources = append(qaSources, nlp.QASource{
			FileName: f.FileName, Content: content,
			SenderName: f.SharedByName, Subject: f.Subject,
			SharedAt: f.CreatedAt,
		})
		sourceNames = append(sourceNames, f.FileName)
		if len(qaSources) >= 3 {
			break
		}
	}
	// Memory context.
	if user != nil {
		mem := s.collectMemorySources(user.ID, req.Question)
		qaSources = append(mem, qaSources...)
	}
	if len(qaSources) == 0 {
		return "I couldn't find anything relevant in your notes or memory. Try sharing the file first.", sourceNames
	}
	return s.classifier.AnswerFromNotesWithContext(req.Question, qaSources, nil), sourceNames
}

// normalizeVoiceQuery undoes common speech-recognition mishears before we
// hit the search layer. Deepgram's ASR consistently turns single-letter
// subject codes into English words ("C" → "see"/"sea"/"deep", "DSA" →
// "dis ah", etc.). We patch the obvious ones with word-boundary regexes
// so a user saying "C programming notes" still resolves when the mic
// returns "see programming notes".
//
// Conservative — only rewrites when followed by a programming/academic
// context word (programming, notes, assignment, ...) so we don't mangle
// sentences where "see" or "mel" are legitimate.
func normalizeVoiceQuery(q string) string {
	if strings.TrimSpace(q) == "" {
		return q
	}
	type rule struct{ pat, repl string }
	rules := []rule{
		// "C programming" family — most common mishear for students.
		{`\b(?i)(see|sea|the|deep|cee)\s+(programming|program|prog|language|lang|notes|assignment|pyq|lab|code)\b`, "C $2"},
		{`\b(?i)(see|sea)\s*\+\s*\+`, "C++"},
		{`\b(?i)(see|sea)\s*plus\s*plus\b`, "C++"},
		// DSA
		{`\b(?i)(dis\s*ah|dis\s*a|desa|disa)\b`, "DSA"},
		// DBMS
		{`\b(?i)d\s+b\s+m\s+s\b`, "DBMS"},
		// OS
		{`\b(?i)(oh\s+ess|o\s+s)\b\s+(?i:(programming|notes|concepts|scheduling|memory|process|syllabus|assignment|lab|pyq))`, "OS $2"},
		// ML — only when paired with an ML context word.
		{`\b(?i)(mel|them\s+l|m\s+l)\s+(?i:(notes|model|assignment|syllabus|lab|pyq|algorithm|algorithms))`, "ML $2"},
		// PYQ spelt out.
		{`\b(?i)p\s+y\s+q\b`, "PYQ"},
		// "AI" when spelt out in tech context.
		{`\b(?i)(a\s+i)\s+(?i:(notes|model|assignment|syllabus|lab|pyq))`, "AI $2"},
	}
	out := q
	for _, r := range rules {
		out = regexpReplace(out, r.pat, r.repl)
	}
	return out
}

// regexpReplace is a small wrapper that caches the compiled regex per
// pattern so we're not re-compiling on every voice call.
var _voiceRxCache = map[string]*regexpCompiled{}
var _voiceRxMu sync.Mutex

type regexpCompiled = regexp.Regexp

func regexpReplace(s, pattern, repl string) string {
	_voiceRxMu.Lock()
	rx, ok := _voiceRxCache[pattern]
	if !ok {
		var err error
		rx, err = regexp.Compile(pattern)
		if err != nil {
			_voiceRxMu.Unlock()
			return s
		}
		_voiceRxCache[pattern] = rx
	}
	_voiceRxMu.Unlock()
	return rx.ReplaceAllString(s, repl)
}

// formatFileForVoice shapes a file row into a voice-friendly map with the
// WHO/WHEN metadata the Vapi assistant needs to narrate results naturally.
// Adds a `speakable_name` derived from the filename — strips extensions,
// dots, hyphens, underscores, and version-like date strings so the TTS
// doesn't read them verbatim ("advertisement-bilingual-06.09.2024.pdf"
// becomes "advertisement bilingual").
func formatFileForVoice(f *model.File, score float64) map[string]any {
	out := map[string]any{
		"file_id":        f.ID,
		"file_name":      f.FileName,
		"speakable_name": speakableFilename(f.FileName),
		"subject":        f.Subject,
		"sender":         f.SharedByName,
		"shared_at":      f.CreatedAt.Format(time.RFC3339),
		"shared_when":    humanTime(f.CreatedAt),
	}
	if score > 0 {
		out["score"] = score
	}
	return out
}

// speakableFilename cleans a raw filename into something TTS can read
// naturally. Drops extension, converts separators to spaces, strips
// ISO-like date fragments, collapses whitespace.
var _speakableRxExt = regexp.MustCompile(`\.[a-zA-Z0-9]{1,5}$`)
var _speakableRxDate = regexp.MustCompile(`\b(19|20)\d{2}([.\-_/]?\d{1,2}){0,2}\b|\b\d{1,2}[.\-_/]\d{1,2}([.\-_/]\d{2,4})?\b`)
var _speakableRxVer = regexp.MustCompile(`\bv\d+(\.\d+)*\b|\(\d+\)`)
var _speakableRxPunct = regexp.MustCompile(`[._\-]+`)
var _speakableRxWS = regexp.MustCompile(`\s+`)

func speakableFilename(name string) string {
	s := name
	s = _speakableRxExt.ReplaceAllString(s, "")
	s = _speakableRxDate.ReplaceAllString(s, "")
	s = _speakableRxVer.ReplaceAllString(s, "")
	s = _speakableRxPunct.ReplaceAllString(s, " ")
	s = _speakableRxWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// closestSender does a simple case-insensitive Levenshtein match between
// the heard name and the list of known senders. Returns "" if no sender
// is within edit-distance 2 (tight threshold to avoid false positives on
// short names). Used to recover from ASR mishears like Harsh→Krish.
func closestSender(heard string, candidates []string) string {
	heard = strings.ToLower(strings.TrimSpace(heard))
	if heard == "" {
		return ""
	}
	best := ""
	bestDist := 99
	for _, c := range candidates {
		first := strings.ToLower(strings.Fields(c)[0]) // compare on first token of sender name
		d := levenshtein(heard, first)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	// Adaptive threshold: allow dist 1 for short names (≤4 chars),
	// dist 2 for longer. Short names are too easy to collide otherwise.
	maxDist := 2
	if len(heard) <= 4 {
		maxDist = 1
	}
	if bestDist <= maxDist {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	ar := []rune(a)
	br := []rune(b)
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			a1 := curr[j-1] + 1
			a2 := prev[j] + 1
			a3 := prev[j-1] + cost
			m := a1
			if a2 < m {
				m = a2
			}
			if a3 < m {
				m = a3
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// humanTime renders a timestamp as something a voice assistant can speak
// naturally: "2 days ago", "yesterday", "3 hours ago", "last week".
func humanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/(24*7)))
	default:
		return t.Format("Jan 2, 2006")
	}
}

func firstNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
