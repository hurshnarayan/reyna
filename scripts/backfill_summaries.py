#!/usr/bin/env python3
"""
Backfill AI summaries and extracted content for documents in reyna.db using gemini-3.5-flash-lite.
"""

import os
import sys
import sqlite3
import base64
import json
import urllib.request
import urllib.error
import time
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DB_PATH = REPO_ROOT / "reyna.db"
ENV_PATH = REPO_ROOT / ".env"
RAW_DIR = REPO_ROOT / "drive_storage" / "_raw"

# Extract API key and model from .env
api_key = ""
model = "gemini-3.5-flash-lite"
if ENV_PATH.exists():
    with open(ENV_PATH) as f:
        for line in f:
            if line.startswith("GEMINI_API_KEY="):
                api_key = line.split("=", 1)[1].strip()
            elif line.startswith("GEMINI_MODEL="):
                val = line.split("=", 1)[1].strip()
                if val:
                    model = val

if not api_key:
    print("Error: GEMINI_API_KEY not found in .env", file=sys.stderr)
    sys.exit(1)

print(f"Connecting to Gemini API using model '{model}'...")

conn = sqlite3.connect(DB_PATH)
c = conn.cursor()
c.execute("SELECT id, file_name, mime_type, file_size FROM files ORDER BY id ASC")
rows = c.fetchall()

INLINE_LIMIT = 14 * 1024 * 1024  # 14MB

for fid, name, mime, size in rows:
    raw_path = RAW_DIR / str(fid)
    print(f"\nProcessing [{fid}] {name} ({mime}, {size} bytes)...")

    prompt = f"""You are a document analysis agent for a WhatsApp study and document retrieval system.
Analyze this document and extract:
1. "content": detailed description of the topics, concepts, key terms, formulas, or contents it covers (max 800 chars)
2. "summary": one-line concise summary of what this document is (max 120 chars)

Filename: "{name}"

Respond ONLY with valid JSON, no markdown codeblocks:
{{"content": "...", "summary": "..."}}"""

    payload_dict = None
    has_inline = False

    if raw_path.exists() and size <= INLINE_LIMIT and ("pdf" in mime or "image" in mime):
        try:
            with open(raw_path, "rb") as rf:
                b64 = base64.b64encode(rf.read()).decode("utf-8")
            payload_dict = {
                "contents": [{
                    "parts": [
                        {"inlineData": {"mimeType": mime, "data": b64}},
                        {"text": prompt}
                    ]
                }],
                "generationConfig": {"responseMimeType": "application/json", "temperature": 0.2}
            }
            has_inline = True
        except Exception as e:
            print(f"  Warning: could not read raw file: {e}")

    if not payload_dict:
        # Text-only filename & metadata analysis
        text_prompt = f"""You are a document analysis agent. Given this document filename and metadata, infer what the document covers:
1. "content": detailed description of likely topics and concepts (max 500 chars)
2. "summary": one-line summary (max 100 chars)

Filename: "{name}", Type: {mime}, Size: {size} bytes

Respond ONLY with valid JSON:
{{"content": "...", "summary": "..."}}"""
        payload_dict = {
            "contents": [{
                "parts": [{"text": text_prompt}]
            }],
            "generationConfig": {"responseMimeType": "application/json", "temperature": 0.2}
        }

    url = f"https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={api_key}"
    req = urllib.request.Request(url, data=json.dumps(payload_dict).encode("utf-8"), headers={"Content-Type": "application/json"})

    content_val = ""
    summary_val = ""

    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                data = json.loads(resp.read().decode())
                txt = data["candidates"][0]["content"]["parts"][0]["text"].strip()
                # Clean json fences
                txt = txt.replace("```json", "").replace("```", "").strip()
                res = json.loads(txt)
                content_val = res.get("content", "").strip()
                summary_val = res.get("summary", "").strip()
                break
        except urllib.error.HTTPError as e:
            err_msg = e.read().decode()[:200]
            print(f"  HTTP error (attempt {attempt+1}): {e.code} - {err_msg}")
            if e.code == 429:
                time.sleep(2)
        except Exception as e:
            print(f"  Request error (attempt {attempt+1}): {e}")
            time.sleep(1)

    if not summary_val and name:
        summary_val = f"Document: {name}"

    print(f"  ✓ Summary: {summary_val}")
    if content_val:
        print(f"  ✓ Content: {content_val[:90]}...")

    c.execute("UPDATE files SET content_summary = ?, extracted_content = ? WHERE id = ?", (summary_val, content_val, fid))
    conn.commit()
    time.sleep(0.5)

conn.close()
print("\nDone! All documents backfilled successfully.")
