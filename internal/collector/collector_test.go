package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ghdwlsgur/louder/internal/provider"
)

func TestRunEmitsFixtureRecordsAsJSONLines(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "embedded:aws", &output); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var record provider.RawCostRecord
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("output is not a JSON record: %v", err)
	}
	if record.Provider != "aws" || record.BillingScope != "synthetic-account" || record.SourceRecordID == "" {
		t.Errorf("record = %#v, want AWS fixture record scoped to synthetic-account", record)
	}
	if output.Bytes()[output.Len()-1] != '\n' {
		t.Error("JSON Lines output must end with a newline")
	}
}

func TestRunRejectsFixtureForDifferentProvider(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "embedded:gcp", &output); err == nil {
		t.Fatal("Run() error = nil, want provider/fixture mismatch error")
	}
	if output.Len() != 0 {
		t.Errorf("output = %q, want no partial output on fixture mismatch", output.String())
	}
}

func TestRunSupportsInitialProviderFixtures(t *testing.T) {
	for _, providerName := range []string{"aws", "gcp", "azure"} {
		t.Run(providerName, func(t *testing.T) {
			var output bytes.Buffer
			if err := Run(context.Background(), providerName, "synthetic-account", "embedded:"+providerName, &output); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if output.Len() == 0 {
				t.Fatal("Run() output is empty")
			}
		})
	}
}

func TestRunDoesNotFallBackToSyntheticFixture(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "", &output); err == nil {
		t.Fatal("Run() error = nil without an explicit fixture")
	}
	if output.Len() != 0 {
		t.Errorf("output = %q, want no synthetic data without explicit fixture mode", output.String())
	}
}

func TestDecodeFixtureRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeFixture([]byte(`[{"provider":`), "aws", "synthetic-account"); err == nil {
		t.Fatal("decodeFixture() error = nil for malformed JSON")
	}
}
