# Local Development Instructions

## Git Safety

- NEVER create a git commit unless I explicitly ask you to commit.
- NEVER push to any remote repository unless I explicitly ask you to push.
- NEVER create, open, submit, merge, or interact with pull requests unless I explicitly ask you to.
- NEVER create or delete branches unless I explicitly ask you to.
- NEVER amend, squash, rebase, or rewrite commits unless I explicitly ask you to.
- NEVER use destructive git commands that could discard my work unless I explicitly authorize them.

## Commit Checkpoints

- After completing a logical unit of work, STOP before starting the next unrelated unit.
- Ask me whether I want to commit the current changes before proceeding.
- If I say no, leave the changes uncommitted and wait for my instructions.
- Never interpret "continue", "looks good", or similar as permission to commit.
- A commit requires explicit approval such as "commit this" or equivalent.

## Scope

- Only make changes necessary for the task I requested.
- Do not proactively refactor, redesign, upgrade dependencies, or make unrelated improvements.
- Ask before making changes outside the requested scope.

## Verification

- Run appropriate tests, linting, and checks when useful.
- If verification fails, report it rather than hiding or bypassing it.

================================================================================
CRITICAL WORKSPACE & SCOPE DIRECTIVES FOR REYNA:
================================================================================
1. ACTIVE CODEBASE LOCATION:
   All active work takes place in `/Users/harsh/code/reyna-app`.
   `/Users/harsh/code/reyna` is the legacy v0.1 prototype repository.

2. SYSTEM ARCHITECTURE:
   - Client: Native Android (Kotlin + Jetpack Compose + Room SQLite) in `android/`.
   - Backend: Go 1.22+ server in `cmd/server/main.go` and `internal/`.
   - The WhatsApp web bot and React dashboard are obsolete. Do not suggest or touch bot code.

3. HARD INVARIANTS:
   - Attribution Floor: Never name an individual below 0.70 confidence.
   - Tokenization: Whole-word matching only (no substring matching).
   - Fast-fail Quota Wall: Fast return amber Notice card on Gemini quota limits.
   - Run/Build: Controlled via `justfile` (`just backend-bg`, `just tunnel-up`, `just build`).
