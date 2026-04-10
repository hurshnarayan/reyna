# service

Business-logic layer. Currently this directory is a placeholder — the business
logic still lives inline in `internal/api/handlers.go`.

The intended split is:
- `internal/api/`        — thin HTTP handlers (parse request, call service, write response)
- `internal/service/`    — business logic (classification, retrieval, Q&A orchestration)
- `internal/repository/` — DB layer (SQL, no business rules)

Splitting the existing 2000-line handlers.go into a service layer is a planned
refactor — deferred until the feature surface stabilises.
