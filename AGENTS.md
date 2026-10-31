# AGENTS.md — DocFlow AI Architecture & Coding Guidelines

DocFlow AI is an autonomous, zero-retention document collection and multimodal verification engine tailored for legal, tax, and compliance onboarding workflows.

This document serves as the operational guide for human engineers and autonomous coding agents working inside this repository.

---

## 1. System Philosophy & Non-Negotiable Tenets

- **Zero-Payload Application Tier:** Never stream large binary payloads (PDFs, TIFFs, raw images) through the Go HTTP API. The application tier only coordinates access: client transfers occur directly against Google Cloud Storage (GCS) using short-lived V4 pre-signed URLs.
- **Provider-Agnostic LLM Layer:** All inference requests target an OpenAI-compatible interface (`/chat/completions`). The system runs against Meta's Model API (`api.meta.ai/v1`) by default, but swapping endpoints (Fireworks, Baseten, Modal) must only require updating `BASE_URL` and `API_KEY` environment variables.
- **Strict Output Determinism:** Model outputs must conform to validated JSON schemas. Any LLM invocation that produces conversational chatter, Markdown wrappers (`` ```json ``), or fails schema validation must be treated as a pipeline failure.
- **Zero-Retention Principles:** PII and raw documents are processed ephemerally. Model evaluations extract compliance status and metadata; raw uploaded files must be bound to strict lifecycle policies.

---

## 2. Architecture & Data Flow

```
[ Client / Browser ]
        │
        ├── 1. POST /api/sessions/:token/upload-url ──────► [ Go API / Cloud Run ]
        │                                                          │
        │◄── 2. Returns GCS V4 Signed PUT URL ─────────────────────┤
        │                                                          ▼
        ├── 3. PUT binary directly to Bucket ────────────► [ GCS Ingestion Bucket ]
        │                                                          │
        │                                                          │ (Object Created)
        │                                                          ▼
        ├── 4. POST /api/submissions/verify ──────────────► [ Worker / Task Handler ]
        │                                                          │
        │                                                          ├── Preprocessing / Heuristics
        │                                                          └── Meta Model API (Muse Spark)
        │                                                                  │
        │◄── 5. Status Poll / Dynamic Remediation ─────────────────┴───────▼
                                                             [ Cloud SQL (PostgreSQL) ]
```

---

## 3. Directory Layout & Module Boundaries

Agents working on the repository must respect package boundaries:

```text
.
├── cmd/
│   └── api/
│       └── main.go              # Application entrypoint, DI wiring, graceful shutdown
├── internal/
│   ├── auth/                    # Magic-link token hashing, expiration, and claims
│   ├── database/                # pgxpool setup, connection parameters, queries
│   ├── handlers/                # HTTP route definitions, request/response validation
│   ├── models/                  # Shared domain types, DB entities, JSON DTOs
│   ├── storage/                 # GCS V4 signing and storage abstractions
│   └── verifier/                # Multimodal & LLM client, verification prompts, parsers
├── migrations/                  # Sequential SQL migration files (e.g., 0001_init.sql)
├── test_fixtures/               # Sanitized, synthetic documents for test automation
├── docker-compose.yml           # Local dependency definitions (PostgreSQL 16)
├── .env.example                 # Canonical environment configuration template
├── go.mod                       # Go module dependencies
└── go.sum                       # Cryptographic checksums (must remain checked in)
```

### Module Responsibilities

- **`internal/verifier`**: Owns prompt templates, system instructions, token budgets, and client-side deserialization. It must never import `internal/handlers`.
- **`internal/storage`**: Encapsulates Cloud Storage SDK calls. Generates pre-signed URLs and wraps file metadata extraction.
- **`internal/database`**: Exposes query methods accepting `context.Context` and returning concrete models. Direct SQL belongs here or in a dedicated repository layer—never inline inside HTTP handlers.
- **`internal/handlers`**: Thin HTTP adapters that parse input, enforce authentication, call domain services, and return JSON.

---

## 4. Coding Standards & Go Conventions

### Idiomatic Patterns
- **Standard Library First:** Use `net/http` for routing and HTTP handling.
- **Explicit Context Propagation:** All functions dealing with I/O (database, external network calls, file systems) must accept `ctx context.Context` as their first parameter. Never use `context.Background()` inside downstream methods.
- **Explicit Error Wrapping:** Always wrap errors with actionable context using `%w`:
  ```go
  if err != nil {
      return fmt.Errorf("failed to sign GCS upload URL for key %s: %w", objectKey, err)
  }
  ```
- **Concurrency & Goroutines:** Never spin up unbound goroutines. If concurrent background verification is needed, manage worker pools or offload tasks to Google Cloud Tasks.

### Environment & Configuration
Configuration must strictly read from environment variables (optionally populated by `.env` via `godotenv` during local development):

| Key | Description | Default / Example |
| :--- | :--- | :--- |
| `PORT` | Listening HTTP port | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@localhost:5432/docflow_dev?sslmode=disable` |
| `META_MODEL_BASE_URL` | Base URL for model completions | `https://api.meta.ai/v1` |
| `META_MODEL_API_KEY` | Bearer authentication key | — |
| `META_MODEL_NAME` | Model identifier | `muse-spark-1.3` |
| `GCS_BUCKET_NAME` | Ingestion target bucket | `docflow-uploads-dev` |
| `GOOGLE_APPLICATION_CREDENTIALS` | Local GCP Service Account key path | `./service-account.json` |

---

## 5. Model Integration Contract (Meta Muse Spark)

When modifying or calling `internal/verifier`, the system prompt and payload parsing must enforce this strict contract:

```json
{
  "document_type_detected": "string",
  "is_valid": true,
  "confidence_score": 0.95,
  "extracted_attributes": {
    "entity_name": "string or null",
    "issue_or_tax_year": "string or null",
    "expiration_date": "string or null"
  },
  "issues_detected": [
    "Array of detected discrepancy strings"
  ],
  "client_remediation_message": "Actionable instructions returned to user if invalid, else null"
}
```

### Remediation Rules
- If `is_valid` is `false`, `client_remediation_message` must contain plain, non-technical instructions directing the user on how to fix the submission (e.g., *"Uploaded form is a 1099-NEC. Please provide the 2024 W-2 wage statement."*).
- Never allow model hallucinations to invent tax years or expiration dates; missing fields must be tagged in `issues_detected`.

---

## 6. Testing & Quality Assurance

- **Mock External APIs:** Unit tests must not trigger billable or rate-limited Meta API endpoints. Use mock HTTP test servers (`httptest.Server`) returning deterministic JSON payloads.
- **Synthetic Fixtures Only:** Files placed in `test_fixtures/` must contain purely synthetic data. Never check in genuine government IDs, passports, or tax forms.
- **Go Mod Hygiene:** Always run `go mod tidy` before committing changes to ensure `go.mod` and `go.sum` remain synchronized and uncorrupted.