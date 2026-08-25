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

## Still open

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
