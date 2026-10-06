# Reyna — The Sovereign Document Archive

> **"The file you need is already on your phone."**  
> Zero-upload, privacy-first personal document system. Turns chaotic WhatsApp chats into a self-filing, searchable digital library.

Built for **Smart India Hackathon 2026** (Problem Statement ID: `SIH260150`, Theme: Smart Education) and designed for anyone who manages life and work over WhatsApp.

---

## The Problem: Where Critical Documents Die

Over **three billion people** run their daily lives on WhatsApp. 

Inside those chats live the most vital documents of modern existence:
- Flight and train tickets
- Hospital discharge summaries and medical prescriptions
- Tax receipts and GST invoices
- Rent agreements and landlord receipts
- College lecture notes and previous-year question papers (PYQs)
- Freelance contracts and salary vouchers

**Two days pass.** What happens?  
Buried alive under ten thousand messages, group banter, memes, and festival greetings.

**When emergency strikes**, panic follows:
- Standing at airport security with a missing boarding pass
- Facing a doctor asking for an MRI scan from last year
- Disputing a deposit with a landlord over a lost receipt
- Sitting outside an exam hall frantically searching for Module 2 notes

Traditional solutions (Dropbox, Google Drive, OneDrive, university portals) fail because they demand **homework**: download the file, open an app, rename it, pick a folder, add tags, and manually upload. Nobody does homework consistently. Friction kills it. Chats remain document graveyards.

---

## The Solution: Pure Magic, Zero Friction

Reyna flips this model completely:

1. **Zero Manual Upload**  
   You change zero habits. When someone shares a document in WhatsApp, WhatsApp downloads it to your phone storage (`/Android/media/com.whatsapp/...`). The split second the file lands, Reyna’s background watcher captures it silently. Zero taps required.

2. **Zero Bots Spying in Chats**  
   No bot phone number ever joins your private groups. Zero risk of WhatsApp phone bans. Zero third parties reading your private chat messages.

3. **Sovereign Personal Cloud Storage**  
   Captured files stream directly into your **personal Google Drive** (`/Reyna/<Category>/...`). Reyna stores zero files on central servers. You own and control every single byte.

4. **Forensic Attribution Engine ("Never Guess")**  
   A file saved on phone storage carries no sender name. Reyna reconstructs the author using four forensic signals (arrival timestamps, WhatsApp filename regex, `/Sent/` directory absence, and optional chat export logs).  
   *Strict 0.70 Confidence Floor:* If confidence is below 70%, Reyna never guesses a person's name. It downgrades gracefully to the group chat name or "Found on your phone".

5. **Instant Conversational Retrieval & Source Verification**  
   Ask questions in plain English, Hindi, or conversational shorthand:
   - *"When is my flight to Mumbai departing?"*
   - *"What was the total on last month's electric bill?"*
   - *"Show me the rent agreement landlord sent in August."*
   - *"Who sent that MRI scan?"*  
   Reyna answers instantly with exact facts, displays a verified sender badge, and renders a `[ 1 source ]` button. Tapping it opens the native in-app document viewer, leaping straight to the cited page and highlighting the exact sentence.

---

## System Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                      📱 CLIENT: NATIVE ANDROID                         │
│                                                                        │
│  UI: Single-activity Jetpack Compose (ChatScreen, SourcesSheet)        │
│  State: ReynaViewModel with immutable StateFlow streams                │
│  File Capture: Background FileObserver daemon on WhatsApp media dir    │
│  Local Database: Room SQLite with FTS4 full-text search                │
│  On-Device OCR: Google Play Services ML Kit Vision Text Recognition    │
│  Document Viewer: Native Android PdfRenderer (direct page jump)        │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Encrypted HTTPS (Retrofit / OkHttp)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                      🌐 SECURE GATEWAY / TUNNEL                        │
│                                                                        │
│  ngrok permanent reserved domain (stable OAuth redirect + API URL)     │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                      ⚙️ BACKEND ENGINE: GO SERVER                      │
│                                                                        │
│  Runtime: Go 1.22+ standalone binary (sub-millisecond execution)       │
│  Metadata DB: SQLite in WAL mode (pure SQL, store.go, zero ORM)        │
│  Tokenizer: Unicode UAX #29 (Devanagari, Tamil, Telugu, Arabic)        │
│  Relevance Scorer: BM25-style whole-word boundary matching             │
│  Zero-Cost Parser: Pure Go zip + xml for .docx, .pptx, .xlsx (free)    │
│  Attribution Engine: 4-signal composite confidence scoring             │
│  Quota Governor: 0.03s fast-fail returning amber Notice cards          │
└───────────────────────┬────────────────────────┬───────────────────────┘
                        │                        │
                        ▼                        ▼
┌────────────────────────────────┐      ┌────────────────────────────────┐
│   ☁️ GOOGLE DRIVE API v3        │      │   🧠 GEMINI 2.5 FLASH API      │
│                                │      │                                │
│  User's personal Google Drive  │      │  Dynamic query expansion       │
│  Direct OAuth 2.0 delegation   │      │  768-dim vector embeddings     │
│  Zero centralized hosting      │      │  Selective self-RAG synthesis  │
└────────────────────────────────┘      └────────────────────────────────┘
```

---

## Prerequisites & Toolchain

Before firing up Reyna, ensure your development machine has:

- **Go 1.22+** (`brew install go`)
- **JDK 21** (`brew install temurin@21` or via mise)
- **Android SDK & adb** (Android Studio command-line tools or `brew install android-platform-tools`)
- **SQLite 3** (`brew install sqlite3`)
- **just** command runner (`brew install just`)
- **ngrok** tunnel client (`brew install ngrok`)

---

## Quick Start & Fire-Up Guide

Everything in Reyna is automated through [`just`](https://github.com/casey/just).

### Step 1: Check Your Environment
Run the built-in system doctor:

```bash
just doctor
```

This verifies Go, JDK 21, adb, SQLite3, `.env`, and Android device attachment.

---

### Step 2: Initialize Configuration

Create your `.env` file with generated secrets:

```bash
just env-init
```

This generates cryptographic `JWT_SECRET` and `DEVICE_TOKEN` keys inside `.env`.

---

### Step 3: Configure API Keys & Authentication

Open `.env` and fill in the required keys:

#### 1. Gemini API Key (Free)
1. Go to [Google AI Studio](https://aistudio.google.com/apikey).
2. Generate a free API key.
3. In `.env`, set:
   ```env
   GEMINI_API_KEY=your-gemini-api-key-here
   ```

#### 2. Google Cloud OAuth (Google Drive API)
All free. No billing required.
1. Visit [console.cloud.google.com](https://console.cloud.google.com).
2. Create a project named **Reyna**.
3. Enable the **Google Drive API** (APIs & Services → Library → Search "Google Drive API" → Enable).
4. Go to **OAuth consent screen**:
   - User Type: **External**
   - Fill in App name (`Reyna`) and user support emails.
   - Under **Test Users**, add your personal Google account email.
5. Go to **Credentials** → **Create Credentials** → **OAuth Client ID**:
   - Application Type: **Web Application**
   - Name: `Reyna Web Client`
6. Copy the **Client ID** and **Client Secret** into `.env`:
   ```env
   GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
   GOOGLE_CLIENT_SECRET=your-client-secret
   ```

#### 3. ngrok Stable Tunnel Domain
ngrok free tier includes one permanent reserved domain so redirect URLs never break:
1. Sign up free at [dashboard.ngrok.com](https://dashboard.ngrok.com/signup).
2. Connect authtoken:
   ```bash
   ngrok config add-authtoken <your-ngrok-token>
   ```
3. Claim your free reserved domain at [dashboard.ngrok.com/domains](https://dashboard.ngrok.com/domains) (e.g., `reyna-app.ngrok-free.app`).
4. Set it in `.env` (without `https://`):
   ```env
   NGROK_DOMAIN=reyna-app.ngrok-free.app
   ```
5. In Google Cloud Console under your OAuth Client ID, add the Authorized Redirect URI:
   ```
   https://reyna-app.ngrok-free.app/api/auth/google/callback
   ```

---

### Step 4: Fire Up the Tunnel & Backend

Start the stable tunnel and backend in one shot:

```bash
just tunnel-up
```

What this does automatically:
- Starts ngrok bound to your reserved domain.
- Updates `GOOGLE_REDIRECT_URL` in `.env`.
- Builds and starts the Go backend detached in the background on port `8080`.
- Points `android/local.properties` at the tunnel URL.
- Confirms backend health via `/api/health`.

To tail backend server logs:
```bash
just backend-log
```

---

### Step 5: Connect Google Drive

Authorize Reyna to back up files to your Drive:

```bash
just drive-connect
```

This opens your browser with Google's OAuth consent screen. Sign in with your test user Google account. Reyna will now automatically route captured documents into your personal Drive.

---

### Step 6: Build and Launch the Android App

#### Option A: Running on a Connected Android Device (USB / adb)
Connect your phone with USB debugging enabled:

```bash
just run
```
This compiles the debug APK, installs it onto the device, and launches the app.

#### Option B: Running on Android Emulator
```bash
just point-at-emulator
just run
```

#### Option C: Build Signed Release APK (To Sideload)
Generate a local signing key once:
```bash
just keygen
```
Then build the signed production APK:
```bash
just apk-release
```
The finished, signed APK is placed at `~/Desktop/reyna.apk`. Send it to your phone via AirDrop, cable, or chat and install.

---

## Daily Workflow & Useful Commands

| Task | Command | Description |
|:---|:---|:---|
| **System Diagnostics** | `just doctor` | Checks toolchain, env, backend status, and connected devices |
| **Start Tunnel & Server** | `just tunnel-up` | Launches ngrok tunnel + starts background Go server |
| **Start Server Foreground** | `just backend` | Runs Go backend in foreground (`:8080`) |
| **Stop Server** | `just backend-stop` | Cleanly terminates backend without killing tunnels |
| **Tail Server Logs** | `just backend-log` | Follows `/tmp/reyna-backend.log` |
| **Clean Reset DB** | `just backend-fresh` | Wipes local SQLite DB and restarts backend |
| **Run Linter & Tests** | `just check` | Runs `go build`, `go vet`, and package tests |
| **Build Debug APK** | `just build` | Assembles Android debug APK |
| **Install & Launch** | `just run` | Builds, installs, and starts Android app on device |
| **Signed Release APK** | `just apk-release` | Builds signed production APK to `~/Desktop/reyna.apk` |
| **Stream Device Logs** | `just logs` | Streams Android logcat filtered for `Reyna` |
| **Check Drive Sync** | `just drive-state` | Shows pending vs uploaded Drive files |
| **Force Drive Sync** | `just drive-push` | Pushes all staged files to Google Drive immediately |
| **Test AI Retrieval** | `just ask "question"` | Tests query retrieval and citation from terminal |

---

## Testing Retrieval From Terminal

You can test conversational search right from your terminal without opening the phone:

```bash
just ask "When is my flight to Mumbai departing?"
just ask "Show me the electric bill"
just ask "Who sent the compiler notes?"
```

Reyna will print the natural language answer along with the cited source files.

---

## Hard Architectural Invariants

1. **Strict 0.70 Attribution Floor:** A raw file on disk has no sender tag. If attribution confidence falls below `0.70`, Reyna **never** guesses a human name. It downgrades to the chat name or device origin.
2. **Device Identity Isolation:** Files uploaded from Android carry internal ID `device`. The system never attributes `device` as a human author.
3. **Whole-Word Matching Only:** Search matching uses whole-word boundaries. Substrings are forbidden (searching "OS" never matches "hospital").
4. **Zero-Cost Office File Parsing:** `.docx`, `.pptx`, and `.xlsx` files are parsed locally via Go stdlib `archive/zip` and `encoding/xml`. Only PDFs incur LLM processing calls.
5. **Fast-Fail Quota Wall:** When Gemini free-tier allowance runs low, the backend returns an amber Notice card in `0.03s` with the reset time, rather than stalling or hanging.
6. **Sovereignty First:** All user files belong in the user's personal Google Drive. Zero centralized storage liabilities.

---

## License

MIT — Free forever. Your data stays in **your** Drive.
