package verifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type VerificationRequest struct {
	RequestedDocType string `json:"requested_doc_type"`
	IngestedContent  string `json:"ingested_content"`
}

type VerificationResponse struct {
	DetectedType       string                 `json:"document_type_detected"`
	IsValid            bool                   `json:"is_valid"`
	ConfidenceScore    float64                `json:"confidence_score"`
	ExtractedAttrs     map[string]interface{} `json:"extracted_attributes"`
	Issues             []string               `json:"issues_detected"`
	RemediationMessage *string                `json:"client_remediation_message"`
}

type MetaVerifier struct {
	baseURL    string
	apiKey     string
	modelName  string
	httpClient *http.Client
}

func NewMetaVerifier(baseURL, apiKey, modelName string) *MetaVerifier {
	return &MetaVerifier{
		baseURL:    baseURL,
		apiKey:     apiKey,
		modelName:  modelName,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (v *MetaVerifier) VerifyDocument(ctx context.Context, req VerificationRequest) (*VerificationResponse, error) {
	systemPrompt := `You are an automated document compliance and intake verification engine for DocFlow AI.
Analyze the provided document text/data against the requested document specification.
Evaluate:
1. Document type match (e.g., W-2 vs 1099, passport vs driver's license).
2. Validity and freshness (expiration dates, tax years, required signatures).
3. Legibility and completeness.

You MUST respond strictly with valid raw JSON matching this schema with no markdown wrapping:
{
  "document_type_detected": "string",
  "is_valid": true,
  "confidence_score": 0.95,
  "extracted_attributes": {},
  "issues_detected": [],
  "client_remediation_message": null
}`

	userPrompt := fmt.Sprintf("Target Requested: %s\nIngested Content:\n%s", req.RequestedDocType, req.IngestedContent)

	payload := map[string]interface{}{
		"model": v.modelName,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", v.baseURL+"/chat/completions", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+v.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("meta api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta api returned non-200 code: %d", resp.StatusCode)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response envelope: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("meta api returned 0 choices")
	}

	var result VerificationResponse
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse structured json payload: %w (raw: %s)", err, chatResp.Choices[0].Message.Content)
	}

	return &result, nil
}
