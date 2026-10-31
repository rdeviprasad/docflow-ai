# AGENTS.md — DocFlow AI Architecture & Coding Guidelines

DocFlow AI is an autonomous, zero-retention document collection and multimodal verification engine tailored for legal, tax, and compliance onboarding workflows.

This document serves as the operational guide for human engineers and autonomous coding agents working inside this repository.

---

## 1. System Philosophy & Non-Negotiable Tenets

- **Zero-Payload Application Tier:** Never stream large binary payloads (PDFs, TIFFs, raw images) through the Go HTTP API. The application tier only coordinates access: client transfers occur directly against Google Cloud Storage (GCS) using short-lived V4 pre-signed URLs.
- **Provider-Agnostic LLM Layer:** All inference requests target an OpenAI-compatible interface (`/chat/completions`). The system runs against Meta's Model API (`api.meta.ai/v1`) by default, but swapping endpoints (Fireworks, Baseten, Modal) must only require updating `BASE_URL` and `API_KEY` environment variables.
- **Strict Output Determinism:** Model outputs must conform to validated JSON schemas. Any LLM invocation that produces conversational chatter, Markdown wrappers (```` ```json ````), or fails schema validation must be treated as a pipeline failure.
- **Zero-Retention Principles:** PII and raw documents are processed ephemerally. Model evaluations extract compliance status and metadata; raw uploaded files must be bound to strict lifecycle policies.

---

## 2. Architecture & Data Flow

```
[ Client / Browser ]
        │
        ├── 1. POST /api/v1/portal/:token/upload-url ──► [ Go API / Cloud Run ]
        │                                                          │
        │◄── 2. Returns GCS V4 Signed PUT URL ─────────────────────┤
        │                                                          ▼
        ├── 3. PUT binary directly to Bucket ────────────► [ GCS Ingestion Bucket ]
        │                                                          │
        │                                                          │ (Object Created)
        │                                                          ▼
        ├── 4. POST /api/v1/portal/:token/verify ────────► [ Go API / Worker ]
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
│   ├── database/                # pgxpool setup, connection parameters, queries & repository
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
- **`internal/database`**: Exposes query and repository methods accepting `context.Context` and returning concrete models. Direct SQL belongs here or in a dedicated repository layer—never inline inside HTTP handlers.
- **`internal/handlers`**: Thin HTTP adapters that parse input, enforce authentication, call domain services, and return JSON.
- **`internal/models`**: Defines domain structs and JSON transfer representations. Has zero external dependencies.

---

## 4. Coding Standards & Go Conventions

### Idiomatic Patterns
- **Standard Library First:** Use standard `net/http` for routing and HTTP handling.
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

## 6. Concrete API Specification & Endpoint Contracts

All routes are mounted under `/api/v1` and handle JSON requests/responses unless noted otherwise.

### A. Intake Sessions (Admin / Firm Facing)
- `POST /api/v1/sessions`
  - **Request Body:**
    ```json
    {
      "organization_id": "uuid",
      "client_name": "string",
      "client_email": "string",
      "checklist_items": [
        {
          "document_type": "string",
          "description": "string",
          "is_required": true
        }
      ]
    }
    ```
  - **Response (201 Created):**
    ```json
    {
      "session_id": "uuid",
      "magic_token": "string",
      "intake_url": "https://domain.com/portal?token=xyz",
      "expires_at": "RFC3339"
    }
    ```

### B. Client Portal (Magic Link Token Gated)
- `GET /api/v1/portal/:token`
  - **Auth:** Resolves and validates `:token` from `intake_sessions`. Rejects expired or invalid tokens with HTTP 401.
  - **Response (200 OK):**
    ```json
    {
      "session_id": "uuid",
      "client_name": "string",
      "status": "PENDING",
      "expires_at": "RFC3339",
      "items": [
        {
          "id": "uuid",
          "document_type": "string",
          "description": "string",
          "is_required": true,
          "status": "PENDING",
          "remediation_message": null
        }
      ]
    }
    ```

- `POST /api/v1/portal/:token/upload-url`
  - **Request Body:**
    ```json
    {
      "checklist_item_id": "uuid",
      "filename": "w2.pdf",
      "mime_type": "application/pdf",
      "file_size_bytes": 1048576
    }
    ```
  - **Validation:** Enforces file size (`<= 25MB`) and allowed MIME types (`application/pdf`, `image/jpeg`, `image/png`).
  - **Response (200 OK):**
    ```json
    {
      "upload_url": "https://storage.googleapis.com/...",
      "storage_path": "sessions/{session_id}/{checklist_item_id}_{timestamp}.pdf"
    }
    ```

- `POST /api/v1/portal/:token/verify`
  - **Request Body:**
    ```json
    {
      "checklist_item_id": "uuid",
      "storage_path": "string",
      "filename": "string",
      "mime_type": "string",
      "file_size_bytes": 1048576,
      "extracted_content": "string"
    }
    ```
  - **Workflow Logic:**
    1. Records new row in `document_submissions`.
    2. Dispatches payload to `verifier.MetaVerifier`.
    3. If `is_valid == true`, updates `checklist_items.status = 'VERIFIED'`.
    4. If `is_valid == false`, updates `checklist_items.status = 'REJECTED'` and writes `remediation_message`.
    5. Returns structured evaluation results directly.
  - **Response (200 OK):** Conforms to the `VerificationResponse` JSON schema.

---

## 7. Go Domain Structs & Database Idioms

### Canonical Structs (`internal/models/models.go`)
```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type IntakeSession struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	ClientName     string    `json:"client_name"`
	ClientEmail    string    `json:"client_email"`
	MagicToken     string    `json:"magic_token"`
	Status         string    `json:"status"` // PENDING, COMPLETED, REVIEW_REQUIRED
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type ChecklistItem struct {
	ID                 uuid.UUID `json:"id"`
	SessionID          uuid.UUID `json:"session_id"`
	DocumentType       string    `json:"document_type"`
	Description        string    `json:"description,omitempty"`
	IsRequired         bool      `json:"is_required"`
	Status             string    `json:"status"` // PENDING, VERIFIED, REJECTED, NEEDS_REVIEW
	RemediationMessage *string   `json:"remediation_message,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type DocumentSubmission struct {
	ID                   uuid.UUID              `json:"id"`
	ChecklistItemID      uuid.UUID              `json:"checklist_item_id"`
	GCSStoragePath       string                 `json:"gcs_storage_path"`
	OriginalFilename     string                 `json:"original_filename"`
	FileMimeType         string                 `json:"file_mime_type"`
	FileSizeBytes        int64                  `json:"file_size_bytes"`
	DetectedDocumentType string                 `json:"detected_document_type"`
	IsValid              bool                   `json:"is_valid"`
	ConfidenceScore      float64                `json:"confidence_score"`
	IssuesDetected       []string               `json:"issues_detected"`
	ExtractedMetadata    map[string]interface{} `json:"extracted_metadata"`
	RemediationMessage   *string                `json:"remediation_message"`
	EvaluatedAt          *time.Time             `json:"evaluated_at"`
	CreatedAt            time.Time              `json:"created_at"`
}
```

### PostgreSQL / pgx Data Access Patterns
- Use parameterized queries with `$1, $2, ...` to prevent SQL injection.
- Prefer returning domain structs or standard slices directly from repository functions.
- Handle nullable columns in SQL using standard Go pointer types (`*string`, `*time.Time`).
- Reference scanning query pattern:
```go
func (r *Repository) GetSessionByToken(ctx context.Context, token string) (*models.IntakeSession, error) {
	query := `
		SELECT id, organization_id, client_name, client_email, magic_token, status, expires_at, created_at
		FROM intake_sessions
		WHERE magic_token = $1 AND expires_at > NOW();
	`
	var s models.IntakeSession
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&s.ID, &s.OrganizationID, &s.ClientName, &s.ClientEmail,
		&s.MagicToken, &s.Status, &s.ExpiresAt, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("session lookup failed: %w", err)
	}
	return &s, nil
}
```

---

## 8. Testing Blueprint for Agents

When generating tests, agents must follow these conventions:

1. **Table-Driven Tests:** All unit tests must use standard Go table-driven patterns (`[]struct{ name string, ... }`).
2. **HTTP Handler Tests:** Use `net/http/httptest` to test routes without starting a live TCP listener.
3. **External Model Mocking:** Never hit `api.meta.ai` during tests. Mock the Meta endpoint using `httptest.NewServer`:
```go
func setupMockMetaAPI(t *testing.T, expectedResponse string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(expectedResponse))
	}))
}
```
4. **Mock GCS Signer:** Storage tests must rely on an interface rather than initiating actual Google Cloud network calls during unit testing.
5. **Synthetic Fixtures Only:** Files placed in `test_fixtures/` must contain purely synthetic data. Never check in genuine government IDs, passports, or tax forms.
6. **Go Mod Hygiene:** Always run `go mod tidy` before committing changes to ensure `go.mod` and `go.sum` remain synchronized and uncorrupted.