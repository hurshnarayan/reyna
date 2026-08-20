# Reyna design specification

Reyna is a conversation with your own file archive. You ask it for something you
half remember, and it answers with the file, who shared it, and when. That makes
it a messaging app, not a dashboard, and the reference is Signal.

An earlier version of this document mapped Cal AI's tracking dashboard onto
Reyna. It was wrong. Rings and streaks make the product read as something you
check up on, when the whole interaction is asking a question and getting an
answer back. The numbers still matter, but they belong in one place the user
opens deliberately, not spread across the surface they use every day.

## What Signal gets right for us

Signal's chat surface is almost exactly the shape Reyna needs:

- **A conversation is the home screen.** No dashboard between the user and the
  thing they came to do.
- **Bubbles carry attachments.** Signal renders a PDF as a chip inside the
  message: icon, filename, size. Reyna's answers are files, so its answers are
  that chip plus attribution.
- **The composer is always there.** One rounded input, pinned to the bottom.
- **Chrome is quiet.** No cards, no shadows, no borders. Separation comes from
  whitespace and bubble color.
- **The toolbar carries identity and context.** Avatar, name, and a subtitle
  saying what this conversation is.

## The mapping

| Signal | Reyna |
|---|---|
| Conversation with a person | Conversation with your archive |
| Contact avatar and name in toolbar | Bolt avatar, "Reyna", and what it is watching |
| Their message, incoming bubble | Reyna's answer |
| Your message, outgoing bubble | Your question |
| PDF attachment chip in a bubble | The file Reyna found, with attribution |
| Sender name above a group message | Who shared the file, when confidence allows |
| Chat list | Files, most recent first |
| Chats / Calls / Stories | Chat / Files / Settings |

## The toolbar stat

The user needs to know Reyna is working without asking it. The toolbar subtitle
carries the live state in one line:

> Watching 3 chats · 247 files

Tapping it, or the chart icon beside it, opens **Tracking**: a full sheet showing
what Reyna has, what it knows, and what it cannot yet answer. This is the only
place counts and progress appear.

Tracking shows:

1. **Attribution split.** Named, chat only, unknown, as three bars with counts.
   Tapping the unknown bar opens the repair flow.
2. **Watched chats.** Each with its file count and when it last saw something.
3. **Permissions.** Notification access, storage, battery. Live state, with the
   consequence spelled out when one is missing, not a bare toggle.
4. **Storage.** How much is on the device and how much is in Drive.

## Tokens

### Brand

The mark is a lightning bolt, so the accent is electric. `#3B5BFE`. It fills
outgoing bubbles in both themes, exactly as Signal's blue does, and is the only
saturated color in the chrome.

### Light

| Token | Value | Use |
|---|---|---|
| `background` | `#FFFFFF` | Everything. No page tint, no cards. |
| `bubbleIncoming` | `#F1F1F4` | Reyna's answers. |
| `bubbleOutgoing` | `#3B5BFE` | Your questions. |
| `onBubbleOutgoing` | `#FFFFFF` | |
| `onSurface` | `#0A0A0B` | |
| `onSurfaceMuted` | `#6B6B70` | Timestamps, subtitles, placeholder. |
| `divider` | `#E8E8EA` | Hairlines, composer outline. |
| `surfaceRaised` | `#F7F7F9` | File chips inside an incoming bubble. |

### Dark

| Token | Value |
|---|---|
| `background` | `#121214` |
| `bubbleIncoming` | `#29292E` |
| `bubbleOutgoing` | `#3B5BFE` |
| `onSurface` | `#F2F2F4` |
| `onSurfaceMuted` | `#96969C` |
| `divider` | `#2A2A2F` |
| `surfaceRaised` | `#1E1E22` |

### The confidence triad

The one place color carries meaning rather than decoration. It appears only on
file chips and in Tracking.

| Band | Color | What Reyna may say |
|---|---|---|
| Named, 0.70 and above | `#2FA84F` | "Mohit · 18 August" |
| Chat only, 0.30 to 0.69 | `#E08600` | "Sem 5 CS · 18 August" |
| Unknown, below 0.30 | `#8A8A90` | "Found on your phone · 18 August" |

Green is not "good" and gray is not "bad". They say how much is known. A gray
chip is Reyna being honest and must never be styled as an error.

### Shape and type

Bubbles are 18dp radius, with the corner nearest the sender tightened to 4dp,
which is what makes a bubble read as coming from a side. File chips are 12dp.
The composer is fully rounded.

Message text 15sp. Timestamps 11sp muted. Toolbar title 17sp semibold, subtitle
12sp muted. Section headers 13sp muted.

## Screens

### Chat

The home screen. A conversation, scrolled to the bottom.

**Toolbar.** Bolt avatar, "Reyna", and beneath it the live subtitle. On the
right, a chart icon opening Tracking, and an overflow.

**Messages.** Your questions right-aligned in the accent. Reyna's answers
left-aligned in the incoming tone. An answer that found files renders the prose
first, then one file chip per result.

**File chip.** Glyph tinted by confidence band, filename, and the attribution
line, which is whatever `Attribution.describe` returns and nothing else. When
the band is below the naming threshold, the chip carries a quiet "who shared
this?" action that opens the repair flow. Tapping the chip opens the file.

**Composer.** Rounded input reading "Ask Reyna", an attach button that offers
"Import a chat", and a send button that is only enabled with text.

**Empty state.** Not an illustration. Reyna opens with a message saying what it
has and offering two or three real questions the user could ask, drawn from
files it actually holds.

### Files

Signal's chat list, applied to documents. Rows, no cards: a rounded glyph tile
where Signal puts an avatar, filename as the title, attribution as the preview
line, time right-aligned. Grouped under section headers by recency.

### Settings

Drive account, permission status with live state, delete everything.

## Rules

**Never render a name the confidence does not support.** Everything reads
`Attribution.describe`. No screen builds its own sender string.

**Absence looks like absence.** No placeholder avatar for an unknown sender, no
the word "Unknown" sitting where a name goes, no relative time when the date was
inferred rather than known.

**No emoji anywhere in the interface.** Not in labels, not in empty states, not
in the confidence rows. Reyna is telling someone what it does and does not know
about their documents, and an emoji undercuts that.

**No dashes as separators.** Metadata joins with a middot.

**Capture is silent.** No notification per file. A daily digest at most, off by
default.
