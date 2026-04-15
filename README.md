# reyna

a whatsapp bot + web dashboard + voice assistant that captures files from your study groups, reads whats inside them, classifies them into your google drive, and lets you search, summarise, and talk to your notes in plain english.

**demo:** https://www.youtube.com/watch?v=XB-DCYWbtRQ

built for the vapi x qdrant hackathon, problem statement PS-1 (voice-first knowledge and workflow agent).

## the problem

two billion people use whatsapp. a huge chunk are students. every class group turns into a file graveyard. someone shares a pyq on monday, someone drops a syllabus on wednesday, and by friday its buried under five hundred messages of "bhai send that again."

devs solved this years back with git. students have nothing. reyna is that missing piece.

## current features

**reyna bot**
drops into any whatsapp group. `/reyna init` registers it. every file anyone shares after that gets caught, subject classified, and staged for your drive. works on pdfs, docx, pptx, xlsx, images.

**files dashboard**
web ui showing every staged and committed file with preview, download, delete, versioning, and one click push to google drive. files page also has a sync-from-drive button that pulls in existing notes reyna didnt capture via the bot.

**reyna's recall**
ask in plain english. "summarise the c programming notes mohit sent last week" returns the actual condensed explanation, grounded in the real file content, with sources cited. works on meaning, not keywords. built on qdrant.

**reyna's memory**
persistent user level context. pin your syllabus, exam dates, study style. reyna uses them to shape every answer. toggle off when you dont want them, delete when done.

**reyna live**
the voice layer. click call reyna on the dashboard or drop a voice note in whatsapp. reyna listens, finds files, summarises, saves memories, all by voice. built on vapi.

**background jobs**
push to drive and sync from drive run as background jobs. close the tab, switch pages, the work keeps going on the server. a progress pill tracks every live job across every page.

## how it works

### how qdrant helps

normal search needs the exact word. if your file says "pointers" and you search "ml memory allocation", keyword search misses it. qdrant is a vector database. every file's text gets converted into a list of 768 numbers that describe its meaning. files about similar things end up with similar numbers, sitting near each other in that number space. when you ask a question, reyna converts the question into the same kind of numbers and qdrant finds the nearest files. pure meaning match, no keyword overlap required.

the same trick powers memory. long memories like a full semester syllabus get chunked up, each chunk gets its own 768 numbers, and only the chunks relevant to the current question get pulled into the prompt. so a 40 page syllabus doesnt blow the context window.

### how vapi helps

vapi handles the voice. speech to text in real time, turn detection, interruption handling, text to speech, and function call routing. all reyna had to write was the backend functions (find files, answer from a file, add a memory, commit staged files) and vapi does everything between "user spoke" and "user heard the reply."

the web sdk puts a "call reyna" button on the dashboard. click it, talk, get answers from your own notes spoken back. on whatsapp, voice notes go through the same pipeline.

### the data flow

1. file lands via bot or drive sync
2. content extraction (zip + xml for office docs, gemini multimodal for pdf / images)
3. content embedded into qdrant for semantic search
4. search queries get embedded, qdrant returns nearest files, gemini writes the final answer grounded in retrieved text
5. voice tool calls hit the same search path, reply gets spoken back via vapi

## run it locally

requires go 1.22+, node 18+, an api key pair from qdrant cloud, gemini, and vapi.

```
git clone https://github.com/hurshnarayan/reyna
cd reyna
cp .env.example .env
# fill in GEMINI_API_KEY, QDRANT_URL, QDRANT_API_KEY,
#             VAPI_PUBLIC_KEY, VAPI_ASSISTANT_ID, etc.
make install
```

then in three terminals:

```
make backend     # go api on :8080
make frontend    # react on :5173
make bot         # whatsapp bot, scans qr on first run
```

`make fresh` wipes the local db and restarts the backend, handy for demos. full walkthrough for vapi and the voice tools is in `docs/reyna-live-setup.md`.

## privacy

reyna only sees messages in groups where it has been initialized. files get sent to gemini for classification and content extraction, then live in your own google drive. extracted text is cached locally in sqlite. no third party storage, no training on your data.

if a group has anything genuinely private (banking, medical, personal documents) just do not add reyna to it. same rule of thumb as chatgpt or notion ai.

## tech

go, react, sqlite, google drive api, gemini, qdrant, vapi, baileys (whatsapp).

## license

mit.
