# Hackathon Sprint Tasks

- [ ] Implement database repository layer in internal/database/repository.go for intake_sessions and checklist_items per AGENTS.md Section 8, with unit tests.
- [ ] Implement GCS V4 pre-signed URL generator in internal/storage/gcs.go with interface abstraction and unit tests.
- [ ] Implement HTTP handler for GET /api/v1/portal/:token in internal/handlers/portal.go per AGENTS.md Section 7, with table-driven tests.
- [ ] Implement HTTP handler for POST /api/v1/portal/:token/upload-url in internal/handlers/portal.go with size and MIME type checks.
- [ ] Implement verification endpoint POST /api/v1/portal/:token/verify orchestrating verifier.MetaVerifier and recording results in PostgreSQL.
- [ ] Wire all endpoints into cmd/api/main.go with graceful shutdown and environment configuration.