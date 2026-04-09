<div align="center">

# 👑 Reyna

### Stop digging through WhatsApp for files. Stop re-uploading to Drive. Stop typing answers from notes you can't find.

**One bot. Every file your group ever shared. Searchable, classified, conversational — in any language.**

<br/>

![Go](https://img.shields.io/badge/Backend-Go_1.22-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![React](https://img.shields.io/badge/Frontend-React_+_Vite-61DAFB?style=for-the-badge&logo=react&logoColor=black)
![Node](https://img.shields.io/badge/Bot-Node_+_Baileys-339933?style=for-the-badge&logo=node.js&logoColor=white)
![Gemini](https://img.shields.io/badge/AI-Gemini_2.5_Flash-8E75B2?style=for-the-badge&logo=google&logoColor=white)
![SQLite](https://img.shields.io/badge/DB-SQLite-003B57?style=for-the-badge&logo=sqlite&logoColor=white)
![Drive](https://img.shields.io/badge/Storage-Google_Drive-4285F4?style=for-the-badge&logo=googledrive&logoColor=white)

<br/>

<!-- 🎬  DROP YOUR DEMO GIF/MP4 HERE 👇
     Save your screen recording as docs/demo.mp4 and it will autoplay + loop on github.
     If you only have a GIF, save as docs/demo.gif and uncomment the second line below. -->

<video src="docs/demo.mp4" autoplay loop muted playsinline width="720" poster="docs/demo-poster.png">
  Your browser doesn't render the video — see <a href="docs/demo.mp4">docs/demo.mp4</a>
</video>

<!-- ![Reyna in action](docs/demo.gif) -->

<sub><i>↑ A friend DMs Reyna in WhatsApp. Reyna finds the right file from a 200-message backlog, drops a Drive link, and answers a follow-up question — in Hinglish.</i></sub>

</div>

---

## 🛑 Read this first

> ### Reyna is **not** an LLM wrapper.
> Out of **7 processing stages** in the pipeline, **only ONE** involves an LLM call.
> The other six are real, hand-built systems: cryptographic deduplication, zip-archive parsing, ranked SQL retrieval, fuzzy folder normalization, multilingual stopword tokenization, and Drive permission management.
>
> If you've seen "AI projects" that are 50 lines of `openai.chat.completions.create()` — **this is the opposite of that**.

<br/>

## 🎯 The problem (in one image)

```
╭─────────────────────────────────────────────────────────────────╮
│                                                                 │
│   "Bro send that PYQ again?"                                    │
│   "Which Mod 5 notes are the latest?"                           │
│   "Anyone has the wien bridge oscillator pdf?"                  │
│   "Wait, who shared the lab manual on tuesday?"                 │
│                                                                 │
│   ↑ Every group chat in India, every single day.                │
│                                                                 │
╰─────────────────────────────────────────────────────────────────╯
```

2 billion WhatsApp users. Zero file management. PDFs scroll past, lab manuals vanish under 200 unread messages, and "send me that file again" becomes the daily ritual. Drive folders, if they exist at all, are messy and unmaintained because **nobody has time to file every PDF by hand.**

So nobody does.

---

## ✨ What Reyna actually does

```
╭─────────────────────────────────────────────────────────────────╮
│                                                                 │
│   Someone shares a file in your WhatsApp study group.           │
│                                                                 │
│      ↓  Reyna sees it instantly.                                │
│      ↓  Reyna reads what's inside (not just the filename).      │
│      ↓  Reyna figures out which subject it belongs to.          │
│      ↓  Reyna files it in your Google Drive — auto-versioned.   │
│      ↓  Reyna remembers WHO sent it and WHEN.                   │
│                                                                 │
│   2 hours later, you DM Reyna:                                  │
│      "what did mohit share about oscillators today?"            │
│                                                                 │
│   Reyna replies with the file link AND an answer from inside    │
│   the document, in the same language you asked.                 │
│                                                                 │
│   You ask "in simpler words?" → she refines.                    │
│   You ask "and the formula?" → she pulls it from the same PDF.  │
│                                                                 │
╰─────────────────────────────────────────────────────────────────╯
```

**That's the entire loop. Capture → understand → organise → retrieve → answer. Hands-off. Multilingual. In one bot.**

---

## 🧠 The 7-stage pipeline (the part that gets dismissed as "just an API call")

Every file that lands in Reyna goes through **seven** distinct processing stages before it hits your Drive. Six of them never touch an LLM. Here they are:

<table>
<tr>
<th width="60">#</th>
<th width="200">Stage</th>
<th>What it actually does</th>
<th>Where it lives</th>
</tr>
<tr>
<td align="center"><b>01</b></td>
<td><b>🔐 Cryptographic dedup</b></td>
<td>SHA-256 hash of every byte. Same file twice (even renamed) → instant rejection. Different bytes, same name → auto v2/v3 versioning. Race-condition-proof per-group <code>sync.Mutex</code> + partial UNIQUE index in SQLite. <b>Zero LLM, zero cost.</b></td>
<td><code>db/store.go:FindFileByHash</code><br/><code>handlers.go:uploadLockFor</code></td>
</tr>
<tr>
<td align="center"><b>02</b></td>
<td><b>📦 Office text extraction</b></td>
<td>DOCX/PPTX/XLSX are zip archives full of XML. Reyna unzips them with Go stdlib (<code>archive/zip</code>), walks the inner XML with the streaming token decoder, and pulls real text out. <b>No external libs. No API. No cents.</b></td>
<td><code>nlp/officeextract.go</code></td>
</tr>
<tr>
<td align="center"><b>03</b></td>
<td><b>🧹 Tokenization & cleaning</b></td>
<td>"Give me the exact oscillator definition mohit sent today" → throws away noise (give/exact/definition/today) and keeps the one keyword that matters: <code>oscillator</code>. 80-word stopword filter, English + Hindi/Hinglish, with min-length, dedup, and a graceful fallback.</td>
<td><code>db/store.go:TokenizeWhat</code></td>
</tr>
<tr>
<td align="center"><b>04</b></td>
<td><b>🎯 Ranked SQL retrieval</b></td>
<td>Hand-built information retrieval scoring inside SQLite. <code>ORDER BY (CASE WHEN tok1 THEN 1 ELSE 0 END + CASE WHEN tok2 THEN 1 ELSE 0 END + ...) DESC</code>. <b>The ranking happens in SQL — fast, deterministic, free.</b> The LLM never touches the search.</td>
<td><code>db/store.go:SearchFilesNLP</code></td>
</tr>
<tr>
<td align="center"><b>05</b></td>
<td><b>📂 Fuzzy folder snap</b></td>
<td>Gemini suggests "C Programming Lab" but you already have "C Programming Laboratory"? Reyna recognises they're the same and snaps to the existing one. Token-based <b>Jaccard similarity</b> + strict subset detection + case-insensitive exact match short-circuit. <b>No duplicate folders. Ever.</b></td>
<td><code>nlp/classifier.go:snapToExistingFolder</code></td>
</tr>
<tr>
<td align="center"><b>06</b></td>
<td><b>🤖 Multi-source LLM call</b><br/><sub>(the only LLM stage)</sub></td>
<td>Now Gemini gets involved. It receives <i>extracted content + filename + sender + timestamp + group + existing folders</i> all at once. Strict JSON mode, thinking-budget=0, retry on 5xx with exponential backoff. The LLM gets <b>real context</b>, not a naked prompt.</td>
<td><code>nlp/classifier.go:ClassifyFileWithContent</code><br/><code>llm/provider.go:doGeminiRequestWithRetry</code></td>
</tr>
<tr>
<td align="center"><b>07</b></td>
<td><b>☁️ Drive sync + auto-public</b></td>
<td>Multipart upload to Google Drive v3 + idempotent <code>permissions.create</code> with <code>role=reader, type=anyone</code>. So when Reyna drops a Drive link in WhatsApp, it just works — no "request access" popups. Eventual-consistency-aware classification (DB-first, Drive API-second).</td>
<td><code>gdrive/service.go:UploadFileToDrive</code><br/><code>gdrive/service.go:MakeFilePublic</code></td>
</tr>
</table>

<br/>

> 💡 **One LLM call. Six engineered systems. That's the difference between a wrapper and a product.**

---

## 📊 Plus a real analytics layer

Every stat on the Reyna dashboard is calculated from a real SQL aggregation query — not vibes, not LLM hallucination.

| Metric | Query |
|---|---|
| 📊 Subject distribution | `SELECT subject, COUNT(*) FROM files GROUP BY subject` |
| 🏆 Top 5 contributors | `SELECT shared_by_name, COUNT(*) FROM files GROUP BY shared_by_phone ORDER BY count DESC LIMIT 5` |
| 💾 Storage breakdown | `SELECT SUM(file_size) FROM files WHERE group_id IN (...)` |
| 📂 Total groups tracked | `SELECT COUNT(DISTINCT group_id) FROM files` |
| ⏱️ Activity over time | `SELECT date(created_at), COUNT(*) FROM files GROUP BY date(created_at)` |

All in `backend/internal/db/store.go:GetDashboardStats`.

---

## ⭐ The killer features

### ★ 1. Conversational retrieval — not keyword search

Reyna decomposes any natural sentence into **WHO / WHAT / WHEN / WHY**, in any language.

| Query                                            | WHO   | WHAT         | WHEN      | WHY             |
| ------------------------------------------------ | ----- | ------------ | --------- | --------------- |
| *"What did Rakesh share about drones last week?"* | Rakesh | drones      | 7 days    | retrieve        |
| *"Do we have any OS notes?"*                     | —     | OS          | —         | check existence |
| *"Mohit ne kal kya bheja?"* 🇮🇳                  | Mohit | —            | yesterday | retrieve        |
| *"क्या किसी ने कंप्यूटर नेटवर्क्स के नोट्स शेयर किए?"* 🇮🇳 | —     | computer networks | — | check existence |
| *"What's new since Monday?"*                     | —     | —            | since Mon | activity        |

No `/reyna` prefix. No trigger word. Works for English, Hindi, Hinglish, Bhojpuri, Tamil, Bengali, Marathi, Kannada, Telugu, Malayalam, mixed-script, and slang.

### ★ 2. Content-aware classification

Reyna doesn't classify by filename. It **reads the document** and decides where it belongs.

```
📥 Module3_Compiler_Design_Unit2.pdf

  ↓ extracted: 42 pages on syntax analysis, parsing tables, LL(1) grammars
  ↓ Gemini sees: content + filename + Mohit + 9:42am IST + CSE-2026 group +
                 existing folders [DSA, Compiler Design, OS, DBMS, CN]
  ↓ snaps to: "Compiler Design"  ← best match (existing folder, not invented)
  ↓ uploaded as: Compiler Design/Module3_Compiler_Design_Unit2.pdf
  ↓ marked public via Drive permissions API

✅ Done. No commands. No folder selection. No human in the loop.
```

**Folder priority**: User folders 1st → Reyna folders 2nd → Create new only when nothing fits. Course codes (CAED, ESC, BESC, BCS, BPLC) recognised as their own subjects. Vague umbrellas like "Engineering" / "Science" / "Misc" / "Programming Modules" forbidden.

### ★ 3. Multi-turn Notes Q&A (with follow-ups)

Ask anything from your notes. **Refine until it makes sense.**

```
You:    explain wien bridge oscillator from mohit's pdf

Reyna:  According to hii.pdf shared by Mohit Singh this morning, a Wien
        bridge oscillator uses a lead-lag RC network arranged in a Wien
        bridge layout to produce zero phase shift at one resonant
        frequency...

You:    in simpler words

Reyna:  Sure — it's a simple AC oscillator that picks one specific
        frequency to vibrate at. The formula is f = 1 / (2π·R·C),
        straight from the same PDF (page 3).

You:    aur asaan shabdon mein batao  ← (Hindi follow-up)

Reyna:  Bilkul — ye ek aisa circuit hai jo specifically ek hi frequency
        pe oscillate karta hai...
```

Threads conversation context across turns. Detects topic shifts. Available **on the dashboard** AND **in WhatsApp DMs**. With visible "thinking" indicator. With sender + time citation. With multilingual.

### ★ 4. WhatsApp DM personal assistant

Open a DM with the Reyna number, type `reyna find srikar's python notes`, and:
- Reyna replies with a clickable Drive link (auto-public — no access requests)
- For 10 minutes after, follow-ups work **without** the wake word
- Numbered list? Reply `"1"` or `"the second one"` → file drops
- Ask any question about the file → multi-turn Q&A kicks in
- Topic shift → fresh question, fresh search
- Strangers DMing your number without `reyna` prefix → silent ignore (anti-spam)

---

## 🏗️ Full architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  WhatsApp layer                                                 │
│  Group chats · Baileys (Node) · Auto-track · 📌 reactions       │
│  Wake-word DM gate · Active session continuation                │
└───────────────────────────────┬─────────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go backend — agentic pipeline (the 7 stages)                   │
│                                                                 │
│  ① Hash dedup     →  ② Office parse     →  ③ Tokenize           │
│  ④ Ranked SQL     →  ⑤ Fuzzy snap       →  ⑥ Gemini call        │
│  ⑦ Drive sync (auto-public)                                     │
│                                                                 │
└───────────────────────────────┬─────────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Data layer — SQLite                                            │
│  files (with content_hash UNIQUE index, extracted_content,      │
│         shared_by_name, shared_by_phone, status, version)       │
│  groups · group_settings · users · activity_log · waitlist      │
└───────────┬───────────────────────────────────────┬─────────────┘
            ▼                                       ▼
┌─────────────────────────────┐       ┌─────────────────────────────┐
│  LLM API (provider-agnostic)│       │  Google Drive API v3        │
│  • Gemini 2.5 Flash (default)│      │  • OAuth 2.0 + token refresh│
│  • Claude / Grok / OpenAI    │      │  • Folder CRUD              │
│    swappable via env var     │      │  • Multipart upload         │
│  • Inline doc blocks (PDFs)  │      │  • Auto-public permissions  │
│  • Retry on 5xx + 429        │      │  • Eventual-consistency aware│
└─────────────────────────────┘       └─────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  React + Vite dashboard                                         │
│  Files browser · Live search · NLP retrieval · Multi-turn Q&A  │
│  Drive tree (live) · Group settings · Subject + sender analytics│
└─────────────────────────────────────────────────────────────────┘
```

---

## 📁 Repository layout

```
reyna/
├── backend/                 # Go server — 7-stage pipeline
│   ├── cmd/server/         # Entry point
│   └── internal/
│       ├── api/            # HTTP handlers (bot upload, NLP retrieve, Q&A)
│       ├── auth/           # JWT
│       ├── config/         # Env loader
│       ├── db/             # SQLite store + tokenizer + ranked search SQL
│       ├── gdrive/         # Drive API v3 + OAuth + auto-public links
│       ├── llm/            # Provider-agnostic LLM iface (Gemini/Claude/Grok/OpenAI)
│       ├── models/         # Domain types
│       ├── nlp/            # Classifier + extractor + folder snap + multi-turn QA
│       └── reyna/          # Bot reply generation
├── frontend/                # React + Vite dashboard (1.2k LOC, no UI lib)
│   ├── src/
│   │   ├── pages/          # Landing · Dashboard · Files · Search · Settings
│   │   ├── components/     # Icons · notifications
│   │   └── lib/            # API client
│   └── package.json
├── whatsapp-bot/            # Baileys Node bot
│   ├── bot.js              # Wake-word DM gate · session continuation · follow-ups
│   └── package.json
├── docs/
│   ├── demo.mp4            # ← drop your screen recording here
│   └── demo.tape           # ← vhs script to regenerate the demo (optional)
├── .env.example             # Environment template
├── Makefile                 # Build / run helpers
└── README.md
```

---

## 🚀 Quick start

### Prerequisites

- Go 1.22+
- Node 20+
- A Google Cloud project with the Drive API enabled (OAuth client ID + secret)
- A Gemini API key (paid tier recommended — free tier hits 403/quota issues fast)

### 1. Clone + env

```bash
git clone https://github.com/<you>/reyna.git
cd reyna
cp .env.example .env
# fill in: GEMINI_API_KEY, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, JWT_SECRET
```

### 2. Backend

```bash
cd backend
go run ./cmd/server/
# → :8080
```

### 3. Frontend

```bash
cd frontend
npm install
npm run dev
# → :5173
```

### 4. WhatsApp bot

```bash
cd whatsapp-bot
npm install
node bot.js
# scan the QR with your WhatsApp → Linked Devices → Link a Device
```

### 5. Connect Drive + try it

1. Open <http://localhost:5173>, create an account, connect Google Drive (Settings → Change folder → pick a root)
2. In any WhatsApp group, type `/reyna init`
3. Share a PDF in that group → watch it auto-classify in the dashboard staging area
4. DM your own Reyna number: `reyna find that pdf` → file drops with Drive link
5. `reyna summarize it` → multi-turn Q&A kicks in

---

## 🛠️ Tech stack

| Layer | Stack | Why |
|---|---|---|
| **Backend** | Go 1.22 · SQLite · stdlib HTTP · zero ORM | Fast, single binary, no surprises |
| **AI** | Gemini 2.5 Flash (paid tier) · provider-agnostic interface | Inline document blocks, multilingual, swappable for Claude/Grok/OpenAI via `LLM_PROVIDER` env var |
| **Storage** | Google Drive API v3 · OAuth 2.0 · auto-public permissions | Files stay in YOUR drive, not ours |
| **Frontend** | React 19 + Vite 8 · no component library · 1.2k LOC | Hand-rolled markdown renderer, conversational chat thread, auto-grow textareas |
| **Bot** | Node.js · [Baileys](https://github.com/WhiskeySockets/Baileys) · in-memory session store | WhatsApp Web protocol, no Twilio fees, runs anywhere |
| **Auth** | JWT (HS256) | Stateless, simple, no session table |

---

## 🎬 Recording your own demo GIF

If you want to regenerate the demo asset, the easiest way is [vhs](https://github.com/charmbracelet/vhs) (terminal-recording-as-code from Charm). A starter `.tape` file lives at [`docs/demo.tape`](docs/demo.tape):

```bash
brew install vhs
cd reyna
vhs docs/demo.tape
# → outputs docs/demo.gif
```

For a real WhatsApp-side recording, use macOS QuickTime → screen recording → save as `docs/demo.mp4`. The video tag in this README will pick it up automatically.

---

## 🌍 Why Reyna exists

> *Your group chat is a knowledge base.
> Your files deserve better than vanishing into chat history.*

2 billion people share files in WhatsApp. None of them have a way to find those files five days later. The PDFs scroll past, the lab manuals vanish, the "send me that file again" loop runs forever. Drive folders, if they exist, are messy because filing-by-hand is a thankless chore nobody does.

Reyna fixes that. **The user does nothing differently** — they share files the way they always have. Reyna does the rest: capture, parse, classify, dedupe, file, version, retrieve, answer. In any language. Without you ever opening a Drive tab.

That's the vision: **one place, total recall, zero effort**.

---

## 📜 License

MIT. Your data stays in your Drive. Open source forever.

---

<div align="center">

### Built by one engineer in a few sleepless days.
### Because group chats should remember things for you.

<br/>

<sub>⭐ <b>If you read this far and didn't think "it's just an LLM call" — please star the repo.</b> It costs nothing and tells me the engineering was worth showing.</sub>

</div>
