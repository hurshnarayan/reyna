# Reyna

A whatsapp bot + web dashboard + voice assistant that captures files from your study groups, reads whats inside them, classifies them into your google drive, and lets you search, summarise, and talk to your notes in plain english.

**Demo:** https://www.youtube.com/watch?v=XB-DCYWbtRQ

Built for the vapi x qdrant hackathon, problem statement PS-1 (voice-first knowledge and workflow agent).

## The Problem

Two billion people use whatsapp. A huge chunk are students. Every class group turns into a file graveyard. Someone shares a pyq on monday, someone drops a syllabus on wednesday, and by friday its buried under five hundred messages of "bhai send that again."

Devs solved this years back with git. Students have nothing. Reyna is that missing piece.

## Current Features

**Reyna Bot**
Drops into any whatsapp group. `/reyna init` registers it. Every file anyone shares after that gets caught, subject classified, and staged for your drive. Works on pdfs, docx, pptx, xlsx, images.

**Files Dashboard**
Web ui showing every staged and committed file with preview, download, delete, versioning, and one click push to google drive. Files page also has a sync-from-drive button that pulls in existing notes reyna didnt capture via the bot.

**Reyna's Recall**
Ask in plain english. "summarise the c programming notes mohit sent last week" returns the actual condensed explanation, grounded in the real file content, with sources cited. Works on meaning, not keywords. Built on qdrant.

**Reyna's Memory**
Persistent user level context. Pin your syllabus, exam dates, study style. Reyna uses them to shape every answer. Toggle off when you dont want them, delete when done.

**Reyna Live**
The voice layer. Click call reyna on the dashboard or drop a voice note in whatsapp. Reyna listens, finds files, summarises, saves memories, all by voice. Built on vapi.

**Background Jobs**
Push to drive and sync from drive run as background jobs. Close the tab, switch pages, the work keeps going on the server. A progress pill tracks every live job across every page.

## How It Works

### How Qdrant Helps

Normal search needs the exact word. If your file says "pointers" and you search "ml memory allocation", keyword search misses it. Qdrant is a vector database. Every file's text gets converted into a list of 768 numbers that describe its meaning. Files about similar things end up with similar numbers, sitting near each other in that number space. When you ask a question, reyna converts the question into the same kind of numbers and qdrant finds the nearest files. Pure meaning match, no keyword overlap required.

The same trick powers memory. Long memories like a full semester syllabus get chunked up, each chunk gets its own 768 numbers, and only the chunks relevant to the current question get pulled into the prompt. So a 40 page syllabus doesnt blow the context window.

### How Vapi Helps

Vapi handles the voice. Speech to text in real time, turn detection, interruption handling, text to speech, and function call routing. All reyna had to write was the backend functions (find files, answer from a file, add a memory, commit staged files) and vapi does everything between "user spoke" and "user heard the reply."

The web sdk puts a "call reyna" button on the dashboard. Click it, talk, get answers from your own notes spoken back. On whatsapp, voice notes go through the same pipeline.

### The Data Flow

1. File lands via bot or drive sync
2. Content extraction (zip + xml for office docs, gemini multimodal for pdf / images)
3. Content embedded into qdrant for semantic search
4. Search queries get embedded, qdrant returns nearest files, gemini writes the final answer grounded in retrieved text
5. Voice tool calls hit the same search path, reply gets spoken back via vapi

## Run It Locally

Requires go 1.22+, node 18+, an api key pair from qdrant cloud, gemini, and vapi.

```
git clone https://github.com/hurshnarayan/reyna
cd reyna
cp .env.example .env
# fill in GEMINI_API_KEY, QDRANT_URL, QDRANT_API_KEY,
#             VAPI_PUBLIC_KEY, VAPI_ASSISTANT_ID, etc.
make install
```

Then in three terminals:

```
make backend     # go api on :8080
make frontend    # react on :5173
make bot         # whatsapp bot, scans qr on first run
```

`make fresh` wipes the local db and restarts the backend, handy for demos. Full walkthrough for vapi and the voice tools is in `docs/reyna-live-setup.md`.

## Privacy

Reyna only sees messages in groups where it has been initialized. Files get sent to gemini for classification and content extraction, then live in your own google drive. Extracted text is cached locally in sqlite. No third party storage, no training on your data.

If a group has anything genuinely private (banking, medical, personal documents) just do not add reyna to it. Same rule of thumb as chatgpt or notion ai.

## Tech

Go, react, sqlite, google drive api, gemini, qdrant, vapi, baileys (whatsapp).

## License

Mit.
