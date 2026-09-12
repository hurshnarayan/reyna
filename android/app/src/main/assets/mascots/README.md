# Reyna Mascot Assets Reference

This directory preserves the source mascot graphics for easy reference and future UI work.

## 1. Navigation Bar Mascot (`nav/`)
- Source: 9-frame sequence from `/Users/harsh/Documents/Reyna Nav Mascot/`
- Files: `1.png` to `9.png`
- Used in app as: `R.drawable.reyna_nav_1` .. `R.drawable.reyna_nav_9` via `ReynaNavMascot()`
- Frame descriptions:
  - `1.png`: Neutral rest pose, centered forward gaze.
  - `2.png`: Ears perk up, alert forward gaze.
  - `3.png`: Eyelids half-closing.
  - `4.png`: Eyelids fully closed in happy smile blink.
  - `5.png`: Eyes open, glancing slightly right.
  - `6.png`: Eyes centered.
  - `7.png`: Eyes glancing right, ears tilt.
  - `8.png`: Eyes glancing left, ears shift.
  - `9.png`: Return to neutral rest pose (seamless match with frame 1).

## 2. Normal Thinking / Idle Mascot (`normal/`)
- Files: `reyna_mascot_1.png` to `reyna_mascot_5.png`
- Used in app as: `R.drawable.reyna_mascot_1` .. `R.drawable.reyna_mascot_5` via `ReynaMascotAnimated()`
- Used during active search / querying in chat message rows (`MarkRow`).
