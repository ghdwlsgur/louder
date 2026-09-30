package collector

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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

	return encodeRecords(output, records)
}

func RunWithRegistry(ctx context.Context, registry *provider.Registry, providerName, accountID string, now time.Time, output io.Writer) error {
	if providerName == "" || accountID == "" || registry == nil || output == nil {
		return fmt.Errorf("provider registry, provider, account ID, and output are required")
	}
	cloudProvider, err := registry.Resolve(providerName)
	if err != nil {
		return err
	}
	start, end := PreviousCompleteUTCDay(now)
	request := provider.CollectRequest{
		AccountID:    accountID,
		StartTime:    start,
		EndTime:      end,
		CollectionID: providerName + ":" + accountID + ":" + start.Format("2006-01-02"),
	}
	return RunProvider(ctx, cloudProvider, request, output)
}

func RunProvider(ctx context.Context, cloudProvider provider.Provider, request provider.CollectRequest, output io.Writer) error {
	if cloudProvider == nil || output == nil {
		return fmt.Errorf("provider and output are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cloudProvider.ValidateCredentials(ctx); err != nil {
		return err
	}
	records, err := cloudProvider.CollectCosts(ctx, request)
	if err != nil {
		return err
	}
	return encodeRecords(output, records)
}

func encodeRecords(output io.Writer, records []provider.RawCostRecord) error {
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
