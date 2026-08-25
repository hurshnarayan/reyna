# Handoff

Read this first if you are picking up Reyna without the previous conversation.
Last updated 25 August 2026.

## What Reyna is

An Android app plus a Go backend. It captures files shared in the user's own
WhatsApp chats, has an LLM read them, files them into the user's own Google
Drive, and answers plain-language questions about them months later.

The distinguishing technical claim is **attribution**. A file on disk carries
no sender, so Reyna rebuilds "who shared this" from four independent signals
and refuses to name anyone below 0.70 confidence. Any prompt change that drops
that rule silently breaks the core promise, and has done once already.

It is for any files for anyone. Student and college framing was deliberately
removed. Keep the lines "no bot joins your groups, nothing to ban" and "files
never touch Reyna's servers".

## Where things are

Work happens in `/Users/harsh/code/reyna-app`. There is a second, older
directory at `/Users/harsh/code/reyna` that sessions often open in by default;
its history stops at v0.1 and it is not the project. Check the working
directory before doing anything.

```
cmd/server/main.go        entrypoint, background reader
internal/api/handlers.go  routes, retrieval, on-demand reading
internal/nlp/             prompts, query parsing, extraction
internal/docs/            local text extraction, no model, no OCR
internal/repository/      SQLite, no ORM
android/app/src/main/java/app/reyna/
  data/Repo.kt            capture, upload queue, ask()
  net/ReynaApi.kt         HTTP, Rejected exception
  ui/                     Compose screens, ReynaViewModel
justfile                  every build and run command
```

Two Go dependencies in total: `golang-jwt/jwt/v5` and `mattn/go-sqlite3`. Keep
it that way; `internal/docs` reads Office files with `archive/zip` and
`encoding/xml` rather than adding one.

## Running it without a phone

The Android command line tools are installed at
`/opt/homebrew/share/android-commandlinetools`, which is also `sdk.dir` in
`android/local.properties`. Nothing is on the PATH by default, so:

```
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export PATH=$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH
emulator -avd reyna_pixel -no-snapshot-load -gpu swiftshader_indirect &
```

An AVD named `reyna_pixel` already exists (arm64, API 35). Then
`just point-at-emulator`, `just build`, `adb install -r -g`, and
`adb exec-out screencap -p > shot.png` to see what it looks like. Put the URL
back with `just point-at <tunnel url>` before building a release.

There is no ffmpeg on this machine, so an animation cannot be checked by
recording it and splitting the frames. Rendering the same maths in Python with
Pillow and looking at a contact sheet works, and is how the splash was checked.

## Getting running

```
just backend-bg      # build and start on :8080
just tunnel-up       # public URL, rewires .env, server, app build
just apk-release     # signed APK to ~/Desktop/reyna.apk
just check           # go vet and tests
just build           # debug APK
just logs            # device logcat
```

`just tunnel-up` prints an OAuth redirect URI. It must be registered in Google
Cloud Console under the OAuth client or Drive will not connect from a phone.
A free cloudflared tunnel gets a **new hostname on every restart**, which
breaks both the URL baked into the APK and the registered redirect URI.
`just tunnel-keep` supervises it.

For emulator work, `just point-at-emulator` then `just build`. Put it back with
`just point-at <tunnel url>` before building a release.

## The constraint that shapes everything

**Gemini free tier is 20 requests per day per model.** `GEMINI_MODEL` in `.env`
is a comma separated fallback list, so three models gives about 60 calls a day.
Google Cloud billing cannot currently be enabled: it fails with `OR_BACR2_44`
even paying by UPI. Billing support is the only route left.

Consequences that are already designed in, and should not be undone:

- Only PDFs cost a model call. Word, PowerPoint, Excel and text formats are
  read locally by `internal/docs`, free and instant.
- Images and video are never read and never uploaded.
- Files are read **when someone asks about them**, up to three per question,
  and the text is kept. See `ensureContent` in `handlers.go`.
- The background reader takes at most 8 calls a day, stays quiet for 10 minutes
  after any question, and retires files that hold no text.

## Recently fixed, worth not regressing

- **Retrieval matched letters, not words.** Every filename test was a
  substring test, on both sides. "ode" is inside "diodes", so a question about
  ordinary differential equations was answered from a semiconductor lecture and
  cited it. "1" is inside "part1", so a question about module 1 returned
  Module4_part1_Arrays. Matching now happens on whole words in
  `internal/relevance` and `android/.../data/Words.kt`, which split a filename
  at every non-alphanumeric character and at every letter/digit boundary. The
  two must stay in agreement: the phone uses its copy to decide which unsent
  files to push ahead of a question. Adjacent query words score far higher than
  scattered ones, which is what tells "Module 1" from "Module4_part1".
- **Content search stopped after the first 4000 characters.** Retrieval does
  look inside documents, not only at filenames, and it always did: the SQL
  matches `extracted_content`. But the Go ranker that replaced the SQL ranking
  initially scored only the opening of each file, so a lecture naming
  Bernoulli's equation at character 7891 was fetched as a candidate and then
  dropped, making it findable by nothing except its filename, which did not
  mention it either. `relevance.ContainsWord` now scans the whole document in
  place rather than building a word list, because a question can pull a few
  hundred candidates and some are thirty thousand characters. A name match
  still scores 25 against a body match's 4, so the title stays the strongest
  signal; the body decides whether a file is a candidate at all.
- **The search was an OR with no relevance floor.** One matching token
  qualified a file, so "module 1 ode" returned everything containing "module"
  and the top four were cited. The SQL is now only a recall net that
  deliberately over-fetches; `rankByRelevance` makes the real decision, and the
  cut is relative to the best match rather than an absolute threshold.
- **`TokenizeWhat` dropped bare digits** as too short, so "module 1" searched
  for "module". The phone side had been fixed for this and the server had not.
- **A failed reading was recorded as a document containing nothing.**
  `ExtractContent` returned empty on any error, including quota, and the caller
  wrote `UnreadableSentinel`, which is permanent by design. On twenty calls a
  day one refusal retired a file for good, and it did: "2CSE Module 1 ODE of
  first order.pdf" could never answer a question about module 1 ODE again.
  Extraction now returns `ErrNotAttempted` for a reading that never happened,
  and only a document that was genuinely opened and found empty gets the
  sentinel. A one-time migration released the files already lost this way.
- **Extraction threw away transcriptions that ran past the token ceiling.**
  A dense PDF of worked mathematics transcribes to more text than the reply
  budget holds, so the JSON came back cut off mid-string, failed to parse, and
  the document was recorded as unreadable. `salvageJSONString` keeps everything
  up to the cut. This is what actually made the ODE file readable again.
- **Citations could show `[unreadable]` as the passage an answer rested on.**
  The fallback cited the top file whatever was stored against it. A file with
  no text is not evidence and is no longer offered as any.
- **The app invented answers when the backend was unreachable.** It matched the
  question against its own filenames, wrote "I found A, B, C on your phone",
  and attached those files as sources with "File on your phone: A" standing in
  for a quotation. Nothing had been read or searched, but it was laid out
  exactly like an answer. It now says plainly that it could not reach the
  backend, and any filename matches are labelled as such.
- **`ask()` uploaded the whole backlog before sending the question.** It ran
  `syncPending()`, which walks up to four thousand queued files. That is what
  "Looking through your files" was doing for minutes, and why the count of
  documents waiting for Drive climbed while the user watched a spinner. Only
  files matching the question are sent ahead of it now. The backlog still goes
  via `CaptureService` and `ReconcileWorker`.
- **One refused file blocked the whole upload queue.** The loop stopped on any
  failure and drained oldest first, so a file over the 50 MB server limit was
  retried first forever and 82 files behind it never went. A 4xx is now a
  verdict on that file: retire it and carry on. Anything else stops the pass.
- **`.docx` and `.pptx` were never read.** The code asked the model to infer
  contents from the filename and stored the guess as a reading. It described a
  C programming document as being about organisational communication. Filename
  inference is gone; `internal/docs` reads the real text.
- **Extraction stored descriptions, not text.** Both extractors have now been
  rewritten to transcribe verbatim with `[[page N]]` markers, which the
  citation code counts to jump to the right page.
- **Deep retrieval ignored the topic** and only searched sender and date, so on
  a large library the candidates were just the most recent files.
- **Chat had no memory.** Query rewriting resolves follow-ups ("what does it
  say") against the last six turns. This is the standard technique and does not
  need a memory library; mem0, Zep and Letta solve a different problem and are
  Python.
- `just backend-bg` killed every process *connected* to :8080, including the
  emulator. Now `-sTCP:LISTEN`.

## New behaviour worth knowing about

- **Reyna asks which document you meant** instead of guessing, when several
  match the question equally well. A library with six files called "Module 1"
  cannot answer "what is module 1 about" from any one of them. The backend
  decides: `ambiguousCandidates` in `handlers.go` returns
  `status: "needs_choice"` with the candidates, and this happens before
  anything is read, so an ambiguous question costs no model calls. A single
  clear match still answers straight away with no extra tap. The choice comes
  back as `file_ids` on the next request, which skips the search entirely.
- **The sheet opens each candidate.** Choosing between documents by filename
  alone is the same guess Reyna just declined to make, so the file is one tap
  away and the sheet stays open behind the preview.
- **The pending choice is held in memory, not on the message row.** Room is
  configured with `fallbackToDestructiveMigration`, so adding a column would
  wipe the phone's index of every captured file. A choice lost when the app is
  killed only means asking again; the index is not replaceable. If the choice
  ever needs to survive a restart, write a real `Migration(2, 3)` first.
- **Progress is streamed.** `/api/nlp/retrieve` speaks newline-delimited JSON
  when the client sends `Accept: application/x-ndjson`: a line per stage, then
  the answer, always last. Clients that do not ask get the single JSON object
  they always got, so the web app is unaffected. The stage naming the document
  being read is the useful one, because that is where nearly all the time goes.
- **Gemini 3 thinking tokens come out of the output budget.** `CompleteWithDoc`
  now sets `thinkingBudget: 0` and reports `finishReason`, so an empty
  candidate is diagnosable rather than looking like an empty document.

## The mark

Reyna's logo is three rising drops forming a crown, specified in the "Reyna
Mark" artifact and reproduced from its geometry rather than eyeballed. Every
drop is the same curve placed three times, rotated about a pivot below the
mark, the outer pair splayed thirty degrees. The tails stop level and short of
that pivot so the shapes never touch, and the centre drop is longer and heavier
than the outer pair, which is what makes it read as a crown rather than a fan.

It lives in three places, all carrying the same numbers on the same hundred by
hundred grid:

- `ui/components/ReynaMark.kt` draws it in Compose, taking whatever colour it
  is given. Use `ReynaLogo` and size it with a modifier.
- `res/drawable/ic_reyna_mark.xml` is the tinted vector, for the notification
  icon and anything that needs a drawable.
- `res/drawable/ic_launcher_foreground.xml` places it in the adaptive icon's
  66dp safe zone by a single scale and translate, so the geometry stays
  identical rather than being redrawn to fit.

There is no disc behind it. The mark is the three drops; a filled circle is a
container it does not need, and one that stayed black regardless of theme meant
the app opened on a hard black puck before a near-white screen. Drawn straight
onto the background in the foreground colour, it is dark on a light theme and
light on a dark one without anything having to switch. That holds in four
places, and all four had to be fixed separately: the toolbar avatar, the
onboarding screen, the adaptive launcher icon (`drawable/` and
`drawable-night/`), and the Android 12 launch screen, which is configured in
`values/themes.xml` and `values-night/themes.xml` and otherwise falls back to
the launcher icon's plate.

On the onboarding screen the mark assembles itself: a drop falls, lands, ripples,
and the three drops splash up out of the point of impact. That motion is the
mark's own construction rather than something laid over it, since all three
drops are rotations about a pivot below the mark and their tails already point
at it. In chat the mark stands at the end of the conversation permanently, under the
last answer, the way a signature sits at the foot of a letter. At rest it is
quiet; while a question is being answered it darkens and its drops jiggle out
of step with each other, with the stage text beside it. One object in two
states, not two objects: an indicator that appeared on send and disappeared on
arrival meant the thing that had been working vanished at the moment it
finished, and the next question built a new one from nothing. It replaced the
spinner, so the screen carries one piece of visual language rather than two.

Two traps worth knowing. A Compose `Canvas` clips to its bounds, so the falling
drop cannot start above the frame; it starts just inside the top edge and fades
in. And onboarding content sits in a `verticalScroll`, which hands its child an
unbounded height, so `Arrangement.Center` silently did nothing and the mark sat
jammed at the top of the page; the viewport is now measured outside the scroll
and passed in as a minimum height.

Minimum size 16dp; below that the gaps close and it stops being three shapes.
Do not put a bar under it, outline it, squash it, fill it with a gradient, or
put it back on a disc. The old lightning bolt is gone.

## Still open

- **The seamless chat layout has not been seen on a device.** Answers now
  render as full-width prose with no bubble, since a bubble is a container for
  something one party said and the wrong shape for a document read back: it
  capped the answer at 286dp and made four sentences look like a wall of chat.
  Questions keep their bubble, which is what carries the turn-taking. Under
  each answer is a quiet row of controls: sources, copy, try again, and the
  time. Rating buttons were left out on purpose, because nothing records a
  rating and a control that silently discards what you tell it is worse than no
  control; read aloud was left out because it needs text to speech wired up.
- The candidate preview tap (**Open** on a row in the choice sheet) has not been
  exercised. Everything else in the chat has: the choice sheet, picking a
  document, the streamed progress, the bubble-less answers and the action row
  were all run on the emulator in both themes.
- Multiple conversations, like an LLM app. Not started.
- Drive folder policy: one root folder, never invent folders. Not started.
- The SIH hackathon deck. Not started.
- Photos are no longer filed to Drive, which follows from "documents only". If
  photos should still be filed but not read, that needs a separate path.
- 761 images already on the server are inert but still in the database.
- The APK's backend URL and the OAuth redirect break on every tunnel restart.
  Publishing the address somewhere stable for the app to discover would fix it;
  proposed and not built.

## Secrets

`.env`, `android/local.properties`, and `~/.reyna/reyna-release.jks` hold
everything. The first two are gitignored. The keystore is outside the repo and
losing it means existing installs can never be updated. Never commit or print
any of them.

The device token is baked into the APK on purpose. That is a convenience for a
personal build, not a security design.

## How to work here

Run things, do not just build them. Report what was measured, say plainly what
was not tested. Plain English, no emojis, no em dashes anywhere including in
documents. No subagents or workflows unless asked for by name.

Update this file at the end of a working session so the next one is not
reading history.
