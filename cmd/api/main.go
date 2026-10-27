package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/your-username/docflow-ai/internal/database"
	"github.com/your-username/docflow-ai/internal/verifier"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Note: No .env file loaded, reading directly from OS environment")
	}

	ctx := context.Background()

	// 1. Initialize DB pool
	dbURL := os.Getenv("DATABASE_URL")
	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("PostgreSQL connection error: %v", err)
	}
	defer pool.Close()
	fmt.Println("PostgreSQL connection verified successfully.")

	// 2. Initialize Verifier
	metaKey := os.Getenv("META_MODEL_API_KEY")
	metaBaseURL := os.Getenv("META_MODEL_BASE_URL")
	metaModelName := os.Getenv("META_MODEL_NAME")

	client := verifier.NewMetaVerifier(metaBaseURL, metaKey, metaModelName)

	// 3. Run a test verification simulation
	fmt.Println("Sending test document payload to Meta Model API...")
	res, err := client.VerifyDocument(ctx, verifier.VerificationRequest{
		RequestedDocType: "IRS Form W-2 (Tax Year 2024)",
		IngestedContent:  "FORM 1099-MISC - Miscellaneous Information. Tax Year: 2023. Payer: Acme Global Corp. Recipient: John Doe.",
	})
	if err != nil {
		log.Fatalf("Verification pipeline failed: %v", err)
	}

	output, _ := json.MarshalIndent(res, "", "  ")
	fmt.Printf("Model Verification Result:\n%s\n", string(output))
}
