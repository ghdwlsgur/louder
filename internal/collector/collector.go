package collector

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ghdwlsgur/louder/internal/provider"
)

//go:embed testdata/*.json
var fixtures embed.FS

func Run(ctx context.Context, providerName, accountID, fixtureName string, output io.Writer) error {
	if providerName == "" || accountID == "" || output == nil {
		return fmt.Errorf("provider, account ID, and output are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !strings.HasPrefix(fixtureName, "embedded:") {
		return fmt.Errorf("live provider collection is not implemented; an embedded fixture is required")
	}
	fixtureProvider := strings.TrimPrefix(fixtureName, "embedded:")
	if fixtureProvider != providerName {
		return fmt.Errorf("fixture provider does not match requested provider")
	}
	if providerName != "aws" && providerName != "gcp" && providerName != "azure" {
		return fmt.Errorf("unsupported fixture provider")
	}

	data, err := fixtures.ReadFile("testdata/" + fixtureProvider + ".json")
	if err != nil {
		return fmt.Errorf("read embedded fixture: %w", err)
	}
	records, err := decodeFixture(data, providerName, accountID)
	if err != nil {
		return fmt.Errorf("decode embedded fixture: %w", err)
	}

	encoder := json.NewEncoder(output)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode cost record: %w", err)
		}
	}
	return nil
}

func decodeFixture(data []byte, providerName, accountID string) ([]provider.RawCostRecord, error) {
	var records []provider.RawCostRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&records); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("fixture contains multiple JSON values")
		}
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("fixture contains no records")
	}
	for i := range records {
		if records[i].Provider != providerName || records[i].SourceRecordID == "" || records[i].Amount == "" || records[i].Currency == "" {
			return nil, fmt.Errorf("fixture contains an invalid cost record")
		}
		records[i].BillingScope = accountID
	}
	return records, nil
}
