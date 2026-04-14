# Reyna Live — Setup Guide

Reyna Live is the voice surface built on [Vapi](https://vapi.ai) (voice agent) +
[Qdrant](https://qdrant.tech) (semantic search). This guide walks you through
wiring up everything so the "Call Reyna" button on the dashboard and the
WhatsApp voice-note handler both work end-to-end.

Time budget: **~15 minutes** if you already have a domain Reyna is reachable on.

---

## 1. Qdrant (5 min)

Qdrant is the vector database that powers both **Reyna's Recall** (semantic
file search) and **Reyna's Memory** (personal context retrieval).

1. Go to [cloud.qdrant.io](https://cloud.qdrant.io) → **Clusters** → **Create
   Free Cluster**. The free tier is enough for ~1M vectors — plenty for a
   single student's notes over a few semesters.
2. Copy the **cluster URL** (looks like `https://xxxxx.cloud.qdrant.io:6333`).
3. Under **API Keys** create a new key and copy it.
4. Add both to `.env`:
   ```
   QDRANT_URL=https://xxxxx.cloud.qdrant.io:6333
   QDRANT_API_KEY=...
   ```
5. Make sure `GEMINI_API_KEY` is also set — Reyna uses Gemini's
   `text-embedding-004` for the vectors. Free tier at
   [aistudio.google.com/apikey](https://aistudio.google.com/apikey).

Restart the backend. You should see:

```
Search: Qdrant (semantic) at https://xxxxx.cloud.qdrant.io:6333
[Qdrant] Created collection reyna_files (vector size 768)
[Qdrant] Created collection reyna_memories (vector size 768)
```

Every file committed from now on gets auto-embedded. (For back-filling
existing files, see "Backfill" at the bottom.)

---

## 2. Vapi (10 min)

### 2.1 Get credits

1. Sign up at [vapi.ai](https://vapi.ai).
2. Apply the hackathon code **`vapixhackblr`** during signup to get $30 in
   free credits.

### 2.2 Pick a webhook secret

Reyna verifies every incoming tool call using a shared secret header. Pick a
random string (32+ chars) and add it to `.env`:

```
VAPI_WEBHOOK_SECRET=your-long-random-string-here
```

Restart the backend.

### 2.3 Expose your backend

Vapi needs to reach your backend over HTTPS. For local dev:

```bash
# In another terminal
ngrok http 8080
# → copy the https URL, e.g. https://abc123.ngrok.io
```

For production: deploy to Render / Railway / Fly and use that domain.

### 2.4 Create the assistant

1. Log in to the [Vapi dashboard](https://dashboard.vapi.ai).
2. Open **Assistants** → **+ Create Assistant** → **Import JSON**.
3. Paste `configs/vapi-assistant.json`.
4. Before saving, **find and replace** in the JSON:
   - `YOUR_BACKEND_DOMAIN` → the ngrok / production domain (no trailing slash)
   - `REPLACE_WITH_VAPI_WEBHOOK_SECRET` → the same secret you put in `.env`
5. Save. Copy the assistant ID from the URL (it's the UUID in the address).

### 2.5 Configure the web button

1. Dashboard → **API Keys** → copy your **Public Key**.
2. Add to `.env`:
   ```
   VAPI_PUBLIC_KEY=pk_...
   VAPI_ASSISTANT_ID=the-uuid-you-copied
   ```
3. Restart the backend.

### 2.6 Test

1. Open the Reyna dashboard. You should see a **"Call Reyna"** button in the
   bottom-right.
2. Click it. Grant mic permission. Say: *"What are my recent files?"*
3. You should see a streaming transcript and hear Reyna's reply.

If nothing happens, check:
- Browser console for Vapi errors (usually "invalid public key" or "assistant
  not found")
- Backend logs for `[VOICE]` lines — every tool call is logged
- That `X-Reyna-Voice-Secret` header matches `VAPI_WEBHOOK_SECRET`

---

## 3. WhatsApp voice notes

The bot detects `audio/ogg` WhatsApp voice notes and forwards the transcript
to `/api/bot/voice-note`. You need a transcription service — Vapi's STT API
works, or you can use Whisper / Deepgram directly.

### 3.1 Transcription provider (pick one)

**Option A — Deepgram (recommended, cheap):**
```
# whatsapp-bot/.env
DEEPGRAM_API_KEY=...
```
Free tier includes $200 of credits.

**Option B — OpenAI Whisper:**
```
# whatsapp-bot/.env
OPENAI_API_KEY=...
```

### 3.2 Verify

Send a voice note in any WhatsApp group where Reyna is active. You'll see:

```
[BOT] voice note from Arjun (45KB) — transcribing…
[BOT] transcript: "what did mohit send about oscillators"
[BOT] reply: "Found one file — ..."
```

Reyna replies with the answer as text. (Voice-note replies are possible but
require extra TTS wiring; not included here.)

---

## Backfilling existing files

If you already have files in Reyna's DB before enabling Qdrant, they won't be
searchable semantically until you re-embed them. Trigger a backfill:

```bash
# TODO: ship a `reyna backfill` command — for now, re-saving files via the
# dashboard re-runs the extraction + embedding pipeline.
```

Or delete the file rows and re-share them in WhatsApp — the bot will capture
and index them automatically.

---

## What breaks if I skip any of this?

| Missing                    | Impact                                                             |
|----------------------------|--------------------------------------------------------------------|
| `QDRANT_URL`              | Recall and Memory fall back to keyword-only (still functional).   |
| `GEMINI_API_KEY`          | No embeddings — same as above.                                    |
| `VAPI_PUBLIC_KEY`         | "Call Reyna" button shows but can't connect.                      |
| `VAPI_WEBHOOK_SECRET`     | Vapi tool calls will 401. Call answers become generic (no files). |
| Deepgram/OpenAI STT key   | WhatsApp voice notes are ignored (text messages still work).      |

Every piece degrades gracefully — you can ship Recall + Memory without Vapi,
and Vapi without Qdrant (it just loses the semantic edge). No hard couplings.
