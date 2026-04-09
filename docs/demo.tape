# vhs tape file — generates docs/demo.gif from this script.
# Install vhs first:  brew install vhs  (or https://github.com/charmbracelet/vhs)
# Then run:           vhs docs/demo.tape
#
# This creates a fake "watch the bot in action" terminal animation. For a
# real WhatsApp-side recording use macOS QuickTime and save as docs/demo.mp4.

Output docs/demo.gif

Set FontSize 16
Set Width 1100
Set Height 700
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Set PlaybackSpeed 1.2

# ── Title card ──
Type "# 👑 Reyna — your group chat's knowledge base"
Sleep 800ms
Enter
Sleep 400ms

Type "# Watch what happens when a friend DMs the bot in WhatsApp:"
Enter
Sleep 600ms
Enter

# ── Stage 1: bot startup ──
Type@30ms "$ node bot.js"
Sleep 300ms
Enter

Type@5ms "  Reyna WhatsApp Bot starting..."
Enter
Sleep 100ms
Type@5ms "  Backend: http://localhost:8080"
Enter
Sleep 100ms
Type@5ms "  ┌──────────────────────────────────────────┐"
Enter
Type@5ms "  │  Reyna Bot — Connected                   │"
Enter
Type@5ms "  │  Bot owner: +91 ••• ••• ••••             │"
Enter
Type@5ms "  └──────────────────────────────────────────┘"
Enter
Sleep 700ms

# ── Stage 2: incoming DM ──
Type@5ms "  [DM] Mohit Singh: reyna find srikar's python notes"
Enter
Sleep 400ms
Type@5ms "  🔍 looking through your notes..."
Enter
Sleep 600ms
Type@5ms "  [NLP-RETRIEVE] who='srikar' what='python notes' when='' why='retrieve'"
Enter
Sleep 200ms
Type@5ms "  [SQL-NLP] tokens=[python] → 3 row(s) ranked"
Enter
Sleep 200ms
Type@5ms "  [DRIVE-CTX] what='python notes' → 3 match(es) (enriched)"
Enter
Sleep 600ms

# ── Stage 3: reply ──
Type@5ms "  → REYNA REPLIES IN WHATSAPP:"
Enter
Sleep 300ms
Type@5ms "  ┌──────────────────────────────────────────────────────┐"
Enter
Type@5ms "  │  📂 3 files from Srikar Reddy:                       │"
Enter
Type@5ms "  │                                                      │"
Enter
Type@5ms "  │  1.  Py_Module5.pptx.pdf                             │"
Enter
Type@5ms "  │      drive.google.com/file/d/1FS3LqNtBe4nhz...       │"
Enter
Type@5ms "  │  2.  Py_Module_4.pptx                                │"
Enter
Type@5ms "  │      drive.google.com/file/d/19pxwxFTgbB...          │"
Enter
Type@5ms "  │  3.  Py_Module_3.pptx                                │"
Enter
Type@5ms "  │      drive.google.com/file/d/1VFcWqylRA...           │"
Enter
Type@5ms "  │                                                      │"
Enter
Type@5ms "  │  reply with a number, or ask a question.             │"
Enter
Type@5ms "  └──────────────────────────────────────────────────────┘"
Enter
Sleep 1200ms

# ── Stage 4: follow-up ──
Type@5ms "  [DM] Mohit Singh: summarize module 5"
Enter
Sleep 400ms
Type@5ms "  🔍 looking through your notes..."
Enter
Sleep 200ms
Type@5ms "  [QA] question='summarize module 5' → who='' what='module 5'"
Enter
Type@5ms "  [QA] full-document extract for top hit: Py_Module5.pptx.pdf"
Enter
Sleep 800ms
Type@5ms "  → REYNA REPLIES:"
Enter
Sleep 300ms
Type@5ms "  Module 5 of Python Programming covers file I/O, file"
Enter
Type@5ms "  organization with shutil/shelve, and basic Excel handling"
Enter
Type@5ms "  via openpyxl. Shared by Srikar this morning."
Enter
Sleep 1200ms

# ── Stage 5: multi-language ──
Type@5ms "  [DM] Mohit Singh: aur asaan shabdon mein batao"
Enter
Sleep 400ms
Type@5ms "  🔍 looking through your notes..."
Enter
Sleep 600ms
Type@5ms "  → REYNA REPLIES (Hinglish — same language as the question):"
Enter
Sleep 300ms
Type@5ms "  Bilkul — Module 5 ka matlab hai files padhna, likhna, aur"
Enter
Type@5ms "  organize karna Python me. Excel files ko bhi handle kar sakte"
Enter
Type@5ms "  ho. Real basics. Srikar ki PDF page 2 se shuru hota hai."
Enter
Sleep 1500ms

Type@5ms "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
Enter
Sleep 200ms
Type@30ms "  Capture → Understand → Organise → Retrieve → Answer."
Enter
Sleep 200ms
Type@30ms "  Multilingual. Multi-turn. One bot. Zero new apps."
Enter
Sleep 200ms
Type@30ms "  That's Reyna."
Enter
Sleep 2500ms
