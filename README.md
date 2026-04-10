# reyna

a whatsapp bot + web dashboard that captures files from your study groups, reads whats inside them, classifies them into your google drive, and lets you search and ask questions from those notes later -- in any language.

built this because every group chat i've been in has the same problem: someone shares notes, 200 messages later its gone, and then "bro send that pyq again" becomes everyones daily routine.

---

## privacy

reyna processes your files to classify and search them. content gets sent to gemini for ai analysis and the files end up in your own google drive -- not on any third party server. extracted text summaries are cached locally in sqlite for search. we dont sell or train on your data.

reyna can only see messages in groups where its been initialized. anything before that, or groups it hasnt been added to, it cant access. theres also a manual mode (pin reaction only) so if something sensitive gets shared just dont pin it.

for anything actually private (bank stuff, medical docs etc) just dont add those groups to reyna. same deal as chatgpt, notion ai, grammarly -- the ai needs to read the content to be useful about it.

---

## what it does

someone shares a file in your whatsapp study group. reyna picks it up, reads the actual content (not just the filename), figures out what subject it belongs to, and puts it in the right folder in your google drive. automatically. no commands.

later you dm reyna something like "that pdf rakesh shared tuesday about circuit diagrams" and it finds it. drops you a drive link. you can ask follow up questions about the file and it answers from the actual content, cites which page, and replies in whatever language you asked in.

you can also go to the web dashboard and do the same thing -- search, ask questions, browse files, manage which groups reyna tracks.

---

## the pipeline (its not just "save file + call gemini")

every file goes through 7 stages before hitting drive. only 1 of them is an llm call:

**1 -- hash dedup**
sha-256 of the file bytes. same file shared 5 times? saved once. different content same filename? auto v2. race condition safe with per-group mutex + unique index in sqlite.

**2 -- office doc parsing**
docx/pptx/xlsx files are actually zip archives with xml inside. reyna unzips them with go stdlib, walks the xml, pulls out the text. no external libraries. no api call. free.

**3 -- tokenization**
when you search "give me the exact oscillator definition mohit sent today" it strips the noise words (give, exact, definition, today) and keeps whats actually useful: `oscillator`. 80-word stopword filter covering english + hindi/hinglish.

**4 -- ranked sql search**
hand-built scoring in sqlite. `ORDER BY (CASE WHEN token1 THEN 1 ELSE 0 END + CASE WHEN token2 ...)`. the search ranking is sql, not llm. fast and free.

**5 -- fuzzy folder matching**
gemini suggests "C Programming Lab" but "C Programming Laboratory" already exists? jaccard similarity catches it and snaps to the existing folder. no duplicate folders.

**6 -- gemini api call** (this is the one llm stage)
gets the file content + filename + who sent it + when + which group + existing folder list. real context not just a blind prompt. strict json mode, retry on 5xx.

**7 -- drive sync**
uploads to google drive + sets permissions to "anyone with link can view" automatically so when reyna drops a link in chat it just works. no "request access" popups.

---

## analytics

the dashboard stats are real sql aggregation queries:
- subject distribution -- `GROUP BY subject`
- top contributors -- `ORDER BY count DESC LIMIT 5`
- storage used -- `SUM(file_size)`
- group count -- `COUNT(DISTINCT group_id)`

nothing llm generated. just sql on every page load.

---

## the features

**nlp retrieval**

you can ask things like:
- "what did rakesh share about data structures last week?"
- "that pdf from tuesday that had a circuit diagram"
- "mohit ne kal kya bheja?" (hindi)
- "the pptx about file handling in python"

it parses who/what/when/why from the query, searches across filenames + subjects + extracted content, and returns ranked results. works in english hindi hinglish bhojpuri tamil bengali kannada telugu marathi and mixed scripts.

**content-aware classification**

doesnt classify by filename. reads whats inside the document and decides. a file called `IMG_2847.pdf` that contains compiler design content goes into "Compiler Design" folder, not "Unsorted". prioritizes your existing drive folders before creating new ones.

**multi-turn q&a**

ask a question about any shared file. follow up with "in simpler words" or "give me an example" or "aur asaan shabdon mein batao" and it keeps the conversation going, remembers context, switches language to match yours.

```
you:   explain wien bridge oscillator from mohits notes
reyna: according to hii.pdf shared by mohit this morning, a wien bridge
       oscillator uses a lead-lag RC network to produce zero phase shift
       at the resonant frequency. formula: f = 1/(2*pi*R*C)

you:   simpler?
reyna: its basically a circuit that picks one specific frequency to
       vibrate at. like a tuning fork but electronic.

you:   aur formula bata do
reyna: f = 1/(2*pi*R*C) -- yahi formula hai, page 3 pe hai mohit ki pdf me
```

**whatsapp dm**

dm the bot number with `reyna find python notes` and it drops drive links right in chat. for 10 minutes after reyna replies you dont need the `reyna` prefix for follow ups. strangers who dm without the prefix get ignored.

---

## project structure

```
reyna/
├── cmd/server/main.go          -- go backend entry point
├── internal/
│   ├── api/                    -- http handlers
│   ├── repository/             -- sqlite + tokenizer + ranked search
│   ├── model/                  -- domain types
│   ├── nlp/                    -- classifier, folder snap, office text extractor
│   ├── integrations/
│   │   ├── gdrive/             -- drive api + oauth + auto-public links
│   │   └── llm/                -- provider-agnostic (gemini/claude/grok/openai)
│   ├── auth/                   -- jwt
│   ├── config/                 -- env loader
│   └── reyna/                  -- bot reply generation
├── web/                        -- react + vite dashboard
├── whatsapp-bot/bot.js         -- baileys whatsapp bot
├── go.mod
├── .env.example
└── Makefile
```

---

## running it

need: go 1.22+, node 20+, a gemini api key, google oauth credentials

```bash
git clone https://github.com/hurshnarayan/reyna.git
cd reyna
cp .env.example .env
# fill in GEMINI_API_KEY, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, JWT_SECRET
```

```bash
make install    # installs js deps
make backend    # starts go server on :8080
make frontend   # starts react on :5173 (separate terminal)
make bot        # starts whatsapp bot (separate terminal, scan qr)
```

`make fresh` wipes the db and starts the backend. thats probably what youll use day to day.

the makefile auto-loads your .env so you dont need to manually export anything. works in bash zsh and fish.

---

## cost

new google cloud accounts get $300 in free credits (90 days). you need to add a payment method to activate it (they do a small verification charge thats refunded) but they dont actually charge until the credits run out. $300 is enough for months. after that its like 1-5 rs per day for a normal study group.

get your api key at [aistudio.google.com/apikey](https://aistudio.google.com/apikey), enable billing on the linked project at [console.cloud.google.com/billing](https://console.cloud.google.com/billing).

---

## stack

- backend: go, sqlite, stdlib http, zero orm
- ai: gemini 2.5 flash (swappable to claude/grok/openai via env var)
- storage: google drive api v3
- frontend: react + vite, no component library
- bot: node.js + baileys
- auth: jwt

---

## license

mit. your data stays in your drive.
