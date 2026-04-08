# Reyna

> **An autonomous multi-agent pipeline that turns WhatsApp group chats into a searchable knowledge base.**
> Zero commands. Zero training. Every file shared in your groups is read, classified, organized into Drive, and made retrievable through natural language.

---

## The problem

> *"Every group chat is a graveyard of lost files."*

2 billion WhatsApp users. Zero file management. PDFs scroll past, lab manuals vanish into the timeline, and *"send me that PYQ again"* becomes a daily ritual. Reyna fixes that.

---

## What Reyna does

Two user actions. Five autonomous agents. Zero commands to learn.

```
Someone shares a file in WhatsApp
            │
            ▼
   ┌────────────────┐    ┌────────────────┐
   │ Auto-track     │    │ Reaction mode  │
   │ every file     │ or │ 📌 to save     │
   └────────┬───────┘    └────────┬───────┘
            └──────────┬──────────┘
                       ▼
   ┌──────────────────────────────────────┐
   │ AGENT 1 — Content extraction         │
   │ Reads inside the file, not just the  │
   │ filename. PDFs/DOCXs → text + meta.  │
   └──────────────────┬───────────────────┘
                      ▼
   ┌──────────────────────────────────────┐
   │ AGENT 2 — AI classification          │
   │ Content + filename + sender + time   │
   │ → best-fit folder from user's Drive. │
   └──────────────────┬───────────────────┘
                      ▼
   ┌──────────────────────────────────────┐
   │ AGENT 3 — Staging engine             │
   │ 24h auto-commit. Push now or remove. │
   └──────────────────┬───────────────────┘
                      ▼
   ┌──────────────────────────────────────┐
   │ AGENT 4 — Drive sync                 │
   │ Pushed to Google Drive. v1/v2/v3     │
   │ version tracking on re-uploads.      │
   └──────────────────┬───────────────────┘
                      ▼
       File organized. Searchable.
       Version tracked. Zero effort.

   ─────────── LATER: RETRIEVAL ───────────

   ┌──────────────────────────────────────┐
   │ AGENT 5 — NLP retrieval              │
   │ "What did Rahul share about drones   │
   │  last week?"                         │
   │ → WHO=Rahul · WHAT=drones            │
   │ → WHEN=7 days · WHY=retrieve         │
   └──────────────────────────────────────┘
```

---

## Killer features

### ★ Conversational retrieval — not keyword search

Reyna decomposes any natural sentence into **WHO / WHAT / WHEN / WHY**.

| Query                                            | WHO   | WHAT         | WHEN      | WHY             |
| ------------------------------------------------ | ----- | ------------ | --------- | --------------- |
| *"What did Rahul share about drones last week?"* | Rahul | drones       | 7 days    | retrieve        |
| *"Do we have any OS notes?"*                     | —     | OS notes     | —         | check existence |
| *"What did Priya upload yesterday?"*             | Priya | —            | yesterday | retrieve        |
| *"Find the compiler lab manual"*                 | —     | compiler lab | —         | search          |
| *"What's new since Monday?"*                     | —     | —            | since Mon | activity check  |

No `/reyna` prefix. No trigger word. The system understands intent from any phrasing — even indirect questions like *"do we have…"* or *"has anyone shared…"*.

### ★ Autonomous classification pipeline

Drop a file. Five agents process it. No folder selection. No training.

**Folder priority logic:**

| 1st                                    | 2nd                              | 3rd                    |
| -------------------------------------- | -------------------------------- | ---------------------- |
| **User-created folders**               | **Reyna-created folders**        | **Create new folder**  |
| Always preferred — your structure wins | From past classifications        | Only when nothing fits |

**Example — `Module3_Compiler_Design_Unit2.pdf`:**

1. File shared in *"CSE 2026 — Section B"*. Auto-track is on. Captured instantly.
2. Content extracted: 42 pages about syntax analysis, parsing tables, LL(1) grammars.
3. Gemini receives extracted content + filename + sender + timestamp + folder list:
   ```
   📁 Reyna/
     📁 DSA/
     📁 Compiler Design/   ← best match
     📁 Operating Systems/
     📁 DBMS/
     📁 CN/
   ```
4. Staged as **Compiler Design / Module3_Compiler_Design_Unit2.pdf**. Auto-commit countdown: 24 h.
5. Synced to Google Drive. If re-uploaded, becomes **v2** automatically.

**Edge case — no matching folder:**
`UAV_Autonomous_Navigation_Research.pdf` → no match for "UAV" → creates **Robotics & UAV/** (clean, descriptive — not a keyword dump). Future UAV files auto-route here. The system gets smarter over time.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  WhatsApp layer                                                 │
│  Group chats · Baileys (Node) · Auto-track / 📌 · Intent detect │
└───────────────────────────────┬─────────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go backend — agentic pipeline                                  │
│  Content extraction · AI classification · Staging · NLP query   │
│      (PDF parser)      (Gemini/Claude)   (24h timer)  (W/W/W/W) │
└───────────────────────────────┬─────────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Data layer                                                     │
│  SQLite · file metadata + index · group settings · JWT auth     │
└───────────┬───────────────────────────────────────┬─────────────┘
            ▼                                       ▼
┌─────────────────────────────┐       ┌─────────────────────────────┐
│  LLM API (Gemini / Claude)  │       │  Google Drive API           │
│  • Extract + classify       │       │  • Folder tree CRUD         │
│  • NLQ → structured search  │       │  • Upload, sync, versioning │
└─────────────────────────────┘       └─────────────────────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  React dashboard                                                │
│  Files · Search · Staging · Drive tree · Analytics · Groups     │
└─────────────────────────────────────────────────────────────────┘
```

**AI parsing note:** PDFs are base64-encoded and sent as inline document blocks. The LLM reads inside the file directly — collapsing extraction + classification into a single API call. More reliable, less code, better accuracy than a raw Go PDF parser.

---

## Repository layout

```
reyna/
├── backend/         Go server — API, agents, LLM, Drive, SQLite
├── frontend/        React + Vite dashboard
├── whatsapp-bot/    Baileys-based WhatsApp listener (Node)
├── .env.example     Environment template
├── Makefile         Build / run helpers
└── README.md
```

---

## Quick start

```bash
# 1. environment
cp .env.example .env
# fill in: GEMINI_API_KEY, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, JWT_SECRET

# 2. backend
cd backend && go run ./cmd/server/        # :8080

# 3. frontend
cd frontend && npm install && npm run dev # :5173

# 4. whatsapp bot
cd whatsapp-bot && npm install && node bot.js
```

The dashboard lives at <http://localhost:5173>. Connect Google Drive on the **Settings** tab, then point the bot at any WhatsApp group — Reyna takes it from there.

---

## Implementation status

| ✅ Built                              | ★ Killer features                                          |
| ------------------------------------- | ---------------------------------------------------------- |
| WhatsApp bot (Baileys)                | Conversational NLP retrieval (WHO/WHAT/WHEN/WHY)           |
| Two tracking modes — auto + reaction  | Content extraction agent (PDF → LLM document blocks)       |
| Google Drive OAuth + folder CRUD      | AI classification with sender + time signals               |
| Staging engine + 24 h auto-commit     | Folder priority logic (user → reyna → new)                 |
| Keyword + LLM hybrid classification   | Smart folder naming — never "Unsorted"                     |
| Live search + autocomplete            | Notes Q&A — *"What did the teacher say about integrals?"*  |
| Subject analytics + top contributors  |                                                            |
| Re-upload detection (v1 / v2 / v3)    |                                                            |
| React dashboard                       |                                                            |
| JWT auth + waitlist                   |                                                            |

---

## Tech stack

- **Backend** — Go · SQLite · Google Drive API · Gemini 2.5 Flash (Claude drop-in)
- **Frontend** — React · Vite
- **Bot** — Node.js · [`@whiskeysockets/baileys`](https://github.com/WhiskeySockets/Baileys)
- **LLM** — provider-agnostic interface — `gemini`, `claude`, `grok`, `openai` switchable via `LLM_PROVIDER` env var

---

## Philosophy

> Your files deserve better than vanishing into chat history.

Reyna is built around one rule: **the user does nothing differently**. They share files the way they always have. The agents do the rest — read, understand, organize, version, and surface on demand. No commands, no buttons, no folders to pick.
