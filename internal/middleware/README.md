# middleware

HTTP middleware for the Reyna API. Currently this directory is a placeholder —
the JWT validation logic still lives in `internal/auth/auth.go`. The plan is to
move the auth middleware here when the handlers are refactored to use a router
that wires middleware explicitly (e.g. chi or stdlib `http.ServeMux` with
`http.Handler` chains).

For now, see `internal/auth/auth.go` for the working JWT validation code.
