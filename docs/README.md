# Reyna demo assets

Drop your screen recording here as `demo.mp4` (autoplays + loops in the main README).

For a still poster image, save as `demo-poster.png` — recommended:

```bash
ffmpeg -ss 00:00:00 -i docs/demo.mp4 -frames:v 1 -q:v 2 docs/demo-poster.png
```
