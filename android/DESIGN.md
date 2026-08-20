# Reyna design specification

The brief: match Cal AI's interface exactly in structure and feel, adapted to what
Reyna actually does. This document is the spec the implementation follows. If a
screen disagrees with this file, the file is wrong and should be updated.

## The mapping

Cal AI tracks meals against a daily calorie target. Reyna tracks captured files
against how confidently they are attributed. The two have the same shape: a
headline number with a ring showing progress toward completeness, a row of three
secondary metrics, and a reverse-chronological feed of recent items.

| Cal AI | Reyna | Why it maps |
|---|---|---|
| Calories eaten, 1250/2500, with ring | Files attributed, 189/247, with ring | One headline number against a total, ring shows the fraction resolved |
| Protein / Carbs / Fats, three rings | Named / Chat only / Unknown, three rings | Three mutually exclusive buckets summing to the total |
| Week strip with per-day rings | Week strip with per-day capture counts | Same rhythm, same glanceable history |
| Recently uploaded, thumbnail plus macros | Recently captured, file glyph plus attribution | Feed of items with a secondary metadata line |
| Streak pill, top right | Watching pill, top right | Status the user wants confirmed without tapping |
| Plus FAB, add a meal | Import FAB, catch up a chat | The primary action that is not on screen already |
| Home / Progress / Settings | Home / Library / Settings | Three destinations, identical structure |

The Ask box sits above the hero card on Home. It is Reyna's primary interaction
and Cal AI has no equivalent, so it takes the position Cal AI gives to the week
strip, and the week strip moves below it.

## The three confidence colors

This is the one place Reyna's palette carries meaning rather than decoration. It
mirrors Cal AI's protein/carbs/fats triad in position and weight, but each color
states how far the attribution can be trusted.

| Band | Color | What Reyna is allowed to say |
|---|---|---|
| Named, at or above 0.70 | Green `#34C759` | "Mohit, 2 hours ago" |
| Chat only, 0.30 to 0.69 | Amber `#FF9F0A` | "Sem 5 CS, 18 August" |
| Unknown, below 0.30 | Gray `#8E8E93` | "Found on your phone, 18 August" |

Green is not "good" and gray is not "bad". They say how much is known. A gray row
is Reyna being honest, and the interface must never make it look like a defect or
an error state.

## Tokens

Taken from the screenshots.

### Light

| Token | Value | Use |
|---|---|---|
| `background` | `#F6F6F4` | Page. Warm off-white, never pure white. |
| `surface` | `#FFFFFF` | Cards. |
| `surfaceSunken` | `#F2F2F0` | Nested rows inside a card. |
| `onSurface` | `#111113` | Headings, numbers. |
| `onSurfaceMuted` | `#8A8A8E` | Labels, secondary lines. |
| `accent` | `#000000` | Filled buttons, active nav, FAB. |
| `outline` | `#EAEAE8` | Hairlines. |

### Dark

| Token | Value |
|---|---|
| `background` | `#0B0B0C` |
| `surface` | `#1A1A1C` |
| `surfaceSunken` | `#232326` |
| `onSurface` | `#F5F5F7` |
| `onSurfaceMuted` | `#8A8A8E` |
| `accent` | `#FFFFFF` |
| `outline` | `#2A2A2E` |

### Shape and elevation

Cards are 24dp radius, small cards 20dp, pills fully rounded. Shadows are almost
invisible: 2dp offset, 12dp blur, 6 percent black. The separation between card
and page comes from the background tint, not from a drop shadow.

Page padding 16dp. Card padding 18dp. Gap between cards 12dp.

### Type

One family, weight doing the work.

| Role | Size | Weight |
|---|---|---|
| Hero number | 40sp | Bold, tight tracking |
| Stat number | 20sp | Bold |
| Section header | 16sp | SemiBold |
| Body | 15sp | Medium |
| Label | 12sp | Medium, muted |

## The logo

A white lightning bolt on black. It appears in three places, and nowhere else:

1. The launcher icon: bolt on a black background, full bleed.
2. The top bar on Home: the bolt glyph followed by the wordmark "Reyna", exactly
   as Cal AI places its apple beside "Cal AI".
3. The onboarding screen, centered, above the value sentence.

The bolt is a vector, never a bitmap, so it stays sharp and can be tinted for
dark mode.

## Icons

Material Symbols throughout, via `androidx.compose.material:material-icons-extended`.
Rounded style, to match the geometry of the cards.

No emoji anywhere in the interface. Not in labels, not in empty states, not in
the confidence rows. Reyna is telling someone what it does and does not know
about their documents, and an emoji undercuts that.

No dashes as separators either. Use a middot for metadata joins ("Mohit · 2
hours ago") and a line break or a real sentence anywhere else.

| Meaning | Icon |
|---|---|
| Home | `Icons.Rounded.Home` |
| Library | `Icons.Rounded.FolderOpen` |
| Settings | `Icons.Rounded.Settings` |
| Ask | `Icons.Rounded.Search` |
| Import a chat | `Icons.Rounded.IosShare` |
| Named confidently | `Icons.Rounded.Person` |
| Chat only | `Icons.Rounded.Groups` |
| Unknown | `Icons.Rounded.HelpOutline` |
| Watching | `Icons.Rounded.Bolt` |
| A document | `Icons.Rounded.Description` |
| An image | `Icons.Rounded.Image` |

## Screens

### Home

Top bar, left: bolt glyph plus "Reyna". Right: a pill reading "Watching" with
the bolt icon, mirroring Cal AI's streak pill.

Then, in order:

1. **Ask box.** Full width, rounded 20dp, surface colored, search icon leading,
   placeholder "that thing about the deposit".
2. **Week strip.** Seven days, each a small circle containing the number of files
   captured that day. Today is ringed in accent, past days are muted, days with
   nothing captured are a dotted outline.
3. **Hero card.** The count of attributed files over the total, the label
   "Files attributed", and a ring on the right showing the fraction. This is the
   one card that overhangs the section above it, exactly as Cal AI's calorie card
   does.
4. **Three stat cards.** Named, Chat only, Unknown. Each: the count, a muted
   label, and a ring in that band's color with its icon at the center.
5. **Page dots.** Three, first active. Reserved for the carousel that will hold
   per-chat breakdowns.
6. **Recently captured.** Section header, then rows. Each row: a rounded square
   holding the file-type glyph, tinted by confidence band; the filename, one
   line, ellipsized; the attribution line, which is whatever
   `Attribution.describe` returns and nothing else; the time, right aligned.
   A row below the naming threshold carries a small "not sure who shared this"
   affordance that opens the repair flow.
7. **FAB.** Accent circle, import glyph, bottom right, floating clear of the nav.

### Library

The Drive folder tree. Secondary, reachable, deliberately not the front door: a
file browser would make Reyna a worse Google Drive.

### Settings

Drive account, permission status with live state, and delete everything.

## Rules

**Never render a name the confidence does not support.** The interface reads
`Attribution.describe` and shows what it returns. No screen constructs its own
sender string.

**Absence looks like absence.** No placeholder avatar for an unknown sender, no
the word "Unknown" in the sender slot where it reads as a person's name, no
relative time when the date was inferred rather than known.

**Capture is silent.** No notification per file. A daily digest at most, off by
default.
