# Reyna, from a cold checkout to a phone that is capturing files.
#
# `just` with no arguments lists everything. Two things are assumed and both
# are checked by `just doctor`: a JDK 21 and the Android SDK. The backend needs
# only Go and a .env.

set shell := ["bash", "-uc"]

home := env_var("HOME")

# The JDK the Android Gradle Plugin accepts, which is 17 to 21.
#
# Deliberately ignores an ambient JAVA_HOME. This machine has Java 26 exported,
# AGP rejects it, and the failure prints only the version number with no clue
# that the version is the problem. The mise install wins when it is there, and
# JAVA_HOME is the fallback rather than the default.
mise_jdk := home / ".local/share/mise/installs/java/temurin-21"
java_home := if path_exists(mise_jdk) == "true" { mise_jdk } else { env_var_or_default("JAVA_HOME", "") }
android_home := env_var_or_default("ANDROID_HOME", "/opt/homebrew/share/android-commandlinetools")
adb := android_home / "platform-tools/adb"
apk := "android/app/build/outputs/apk/debug/app-debug.apk"
apk_release := "android/app/build/outputs/apk/release/app-release.apk"

# List the recipes.
default:
    @just --list --unsorted

# ── Setup ──────────────────────────────────────────────────────────────────

# Check every tool and file this repo needs, and say what is missing.
doctor:
    #!/usr/bin/env bash
    ok() { printf '  ok    %s\n' "$1"; }
    no() { printf '  MISSING %s\n' "$1"; }
    echo "Toolchain"
    command -v go >/dev/null && ok "go $(go version | awk '{print $3}')" || no "go"
    [ -x "{{java_home}}/bin/java" ] && ok "jdk 21" || no "jdk 21 at {{java_home}}"
    [ -x "{{adb}}" ] && ok "adb" || no "adb at {{adb}}"
    command -v sqlite3 >/dev/null && ok "sqlite3" || no "sqlite3"
    echo "Configuration"
    [ -f .env ] && ok ".env" || no ".env  (run: just env-init)"
    [ -f android/local.properties ] && grep -q reyna.backendUrl android/local.properties \
      && ok "android/local.properties has reyna.backendUrl" \
      || no "reyna.backendUrl in android/local.properties  (run: just point-at-lan)"
    echo "Runtime"
    curl -sf -m 2 http://127.0.0.1:8080/api/health >/dev/null && ok "backend up on :8080" || no "backend not running  (run: just backend)"
    "{{adb}}" devices 2>/dev/null | grep -qw device && ok "a device is attached" || no "no device attached"

# Create .env from the example, generating the two secrets.
env-init:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -f .env ]; then echo ".env already exists, leaving it alone"; exit 0; fi
    sed -e "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" \
        -e "s|^DEVICE_TOKEN=.*|DEVICE_TOKEN=$(openssl rand -hex 32)|" \
        .env.example > .env
    echo "wrote .env. Add GEMINI_API_KEY and the Google OAuth pair before starting."

# Print the device token, which the app needs if it was not baked in.
token:
    @grep '^DEVICE_TOKEN=' .env | cut -d= -f2

# Point app builds at this machine's LAN address, for a real phone.
point-at-lan:
    #!/usr/bin/env bash
    # Re-run whenever you change network. The address is baked in at build
    # time, so a stale one looks exactly like a dead backend.
    set -euo pipefail
    ip=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)
    [ -n "$ip" ] || { echo "no wifi address found"; exit 1; }
    tok=$(grep '^DEVICE_TOKEN=' .env | cut -d= -f2)
    touch android/local.properties
    grep -v '^reyna\.' android/local.properties > android/local.properties.tmp || true
    { cat android/local.properties.tmp
      echo "reyna.backendUrl=http://$ip:8080"
      echo "reyna.deviceToken=$tok"
    } > android/local.properties
    rm -f android/local.properties.tmp
    echo "app builds will now target http://$ip:8080"

# Point the next app build at the emulator's alias for this machine.
point-at-emulator:
    #!/usr/bin/env bash
    set -euo pipefail
    tok=$(grep '^DEVICE_TOKEN=' .env | cut -d= -f2)
    touch android/local.properties
    grep -v '^reyna\.' android/local.properties > android/local.properties.tmp || true
    { cat android/local.properties.tmp
      echo "reyna.backendUrl=http://10.0.2.2:8080"
      echo "reyna.deviceToken=$tok"
    } > android/local.properties
    rm -f android/local.properties.tmp
    echo "app builds will now target http://10.0.2.2:8080"

# ── Backend ────────────────────────────────────────────────────────────────

# Run the backend in the foreground, reading .env.
backend:
    @go run ./cmd/server

# Run the backend detached so it survives this shell. Logs to /tmp/reyna-backend.log
backend-bg:
    #!/usr/bin/env bash
    set -euo pipefail
    go build -o /tmp/reyna-server ./cmd/server
    lsof -ti :8080 | xargs -r kill 2>/dev/null || true
    sleep 1
    printf '#!/bin/sh\ncd %s\nset -a\n. ./.env\nset +a\nexec /tmp/reyna-server\n' "$PWD" > /tmp/run-reyna.sh
    chmod +x /tmp/run-reyna.sh
    nohup /tmp/run-reyna.sh > /tmp/reyna-backend.log 2>&1 &
    sleep 4
    curl -sf -m 3 http://127.0.0.1:8080/api/health >/dev/null && echo "backend up on :8080" || { echo "did not come up, see /tmp/reyna-backend.log"; exit 1; }

# Stop the detached backend.
backend-stop:
    @lsof -ti :8080 | xargs -r kill 2>/dev/null || true; echo "stopped"

# Follow the detached backend's log.
backend-log:
    @tail -f /tmp/reyna-backend.log

# Wipe the backend database and start clean. Files already in Drive stay.
backend-fresh: backend-stop
    @rm -f reyna.db reyna.db-shm reyna.db-wal && just backend-bg

# Build, vet and test the Go.
check:
    @go build ./... && go vet ./... && go test ./...

# ── Web dashboard ──────────────────────────────────────────────────────────

# Install the dashboard's dependencies.
web-install:
    @cd web && npm install

# Run the dashboard on :5173.
web:
    @cd web && npm run dev

# ── App ────────────────────────────────────────────────────────────────────

# Build the debug APK.
build:
    @JAVA_HOME={{java_home}} ./android/gradlew -p android :app:assembleDebug

# Build and install onto the attached device, keeping its data.
install: build
    @{{adb}} install -r {{apk}} && echo "installed"

# Build, install and launch.
run: install
    @{{adb}} shell am force-stop app.reyna
    @{{adb}} shell am start -n app.reyna/.ui.MainActivity >/dev/null && echo "launched"

# Uninstall, then install clean. A true first run, nothing but the baked config.
reinstall: build
    @{{adb}} uninstall app.reyna >/dev/null 2>&1 || true
    @{{adb}} install {{apk}} && echo "clean install"

# Copy the built debug APK somewhere you can share it from.
apk dest="~/Desktop/reyna.apk": build
    @cp {{apk}} {{dest}} && ls -lh {{dest}}

# Build a signed release APK, which is what to sideload onto a real phone.
build-release:
    @JAVA_HOME={{java_home}} ./android/gradlew -p android :app:assembleRelease

# Copy the signed release APK somewhere you can send it from.
apk-release dest="~/Desktop/reyna.apk": build-release
    @cp {{apk_release}} {{dest}} && ls -lh {{dest}}

# Install the signed release build over USB, which Play Protect does not block.
install-release: build-release
    @{{adb}} install -r {{apk_release}} && echo "installed"

# Print the signing certificate of the release APK.
signature:
    @JAVA_HOME={{java_home}} {{android_home}}/build-tools/35.0.0/apksigner verify --print-certs {{apk_release}} 2>/dev/null | grep -E 'DN|SHA-256'

# Create the release signing key. Run once; the key lives outside the repo.
keygen:
    #!/usr/bin/env bash
    set -euo pipefail
    ks="$HOME/.reyna/reyna-release.jks"
    if [ -f "$ks" ]; then echo "keystore already exists at $ks"; exit 0; fi
    mkdir -p "$HOME/.reyna"
    read -rsp "keystore password: " pw; echo
    {{java_home}}/bin/keytool -genkeypair -keystore "$ks" \
      -storepass "$pw" -keypass "$pw" -alias reyna \
      -keyalg RSA -keysize 2048 -validity 10000 \
      -dname "CN=Reyna, OU=Personal, O=Reyna, C=IN"
    grep -v '^reyna\.key' android/local.properties > android/local.properties.tmp 2>/dev/null || true
    { cat android/local.properties.tmp 2>/dev/null
      echo "reyna.keystore=$ks"
      echo "reyna.keystorePassword=$pw"
      echo "reyna.keyAlias=reyna"
      echo "reyna.keyPassword=$pw"
    } > android/local.properties
    rm -f android/local.properties.tmp
    echo "keystore written to $ks and referenced from android/local.properties"

# Follow only Reyna's log lines.
logs:
    @{{adb}} logcat -c && {{adb}} logcat | grep --line-buffered -E 'Reyna|AndroidRuntime'

# Screenshot the device to /tmp/reyna.png
shot:
    @{{adb}} exec-out screencap -p > /tmp/reyna.png && echo "/tmp/reyna.png"

# Clear the app's data. Onboarding, permissions and settings all reset.
app-clear:
    @{{adb}} shell pm clear app.reyna && echo "cleared"

# Show the app's own database.
app-db:
    @{{adb}} shell 'run-as app.reyna sqlite3 databases/reyna.db "select id,name,senderName,round(confidence,2),uploaded from files"'

# ── Drive ──────────────────────────────────────────────────────────────────

# What has reached Drive and what is still waiting.
drive-state:
    @curl -s "http://127.0.0.1:8080/api/device/drive/state?phone=device" -H "Authorization: Bearer $(just token)"; echo

# File everything waiting into Drive now, without waiting for the timer.
drive-push:
    @curl -s -X POST "http://127.0.0.1:8080/api/device/drive/push?phone=device" -H "Authorization: Bearer $(just token)"; echo

# Open the Google consent screen for Drive.
drive-connect:
    #!/usr/bin/env bash
    # Must be done on this machine. Google only allows an http redirect back to
    # localhost, so completing this from the phone's browser cannot work.
    set -euo pipefail
    url=$(curl -s "http://127.0.0.1:8080/api/device/drive/connect?phone=device" \
      -H "Authorization: Bearer $(grep '^DEVICE_TOKEN=' .env | cut -d= -f2)" \
      | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("url") or "")')
    [ -n "$url" ] || { echo "Drive is not configured on the server"; exit 1; }
    open "$url"

# ── Demo ───────────────────────────────────────────────────────────────────

# Everything needed to show the app working, in one command.
demo: backend-bg point-at-lan reinstall
    @echo
    @echo "Backend:  http://$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1):8080"
    @echo "The app is installed clean and will open on the welcome screen."
    @echo "Its server address and token are baked in, so there is nothing to type."

# Ask Reyna something from the command line, the way the app does.
ask question:
    @curl -s -X POST http://127.0.0.1:8080/api/nlp/retrieve \
      -H "Authorization: Bearer $(just token)" -H 'Content-Type: application/json' \
      -d "$(python3 -c 'import json,sys; print(json.dumps({"query": sys.argv[1], "user_phone": "device"}))' {{quote(question)}})" \
      | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("reply","")); [print("  -", f["file_name"]) for f in (d.get("files") or [])]'
