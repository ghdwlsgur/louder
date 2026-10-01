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

	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
)

type CostWriter interface {
	WriteCosts(context.Context, []normalize.CostRecord) error
}

//go:embed testdata/*.json
var fixtures embed.FS

func Run(ctx context.Context, providerName, accountID, fixtureName string, output io.Writer) error {
	records, err := collectFixture(ctx, providerName, accountID, fixtureName, output)
	if err != nil {
		return err
	}
	return encodeRecords(output, records)
}

func collectFixture(ctx context.Context, providerName, accountID, fixtureName string, output io.Writer) ([]provider.RawCostRecord, error) {
	if providerName == "" || accountID == "" || output == nil {
		return nil, fmt.Errorf("provider, account ID, and output are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(fixtureName, "embedded:") {
		return nil, fmt.Errorf("live provider collection is not implemented; an embedded fixture is required")
	}
	fixtureProvider := strings.TrimPrefix(fixtureName, "embedded:")
	if fixtureProvider != providerName {
		return nil, fmt.Errorf("fixture provider does not match requested provider")
	}
	if providerName != "aws" && providerName != "gcp" && providerName != "azure" {
		return nil, fmt.Errorf("unsupported fixture provider")
	}

	data, err := fixtures.ReadFile("testdata/" + fixtureProvider + ".json")
	if err != nil {
		return nil, fmt.Errorf("read embedded fixture: %w", err)
	}
	records, err := decodeFixture(data, providerName, accountID)
	if err != nil {
		return nil, fmt.Errorf("decode embedded fixture: %w", err)
	}
	return records, nil
}

func RunWithStorage(ctx context.Context, providerName, accountID, fixtureName string, output io.Writer, storage CostWriter) error {
	if storage == nil {
		return fmt.Errorf("cost storage is required")
	}
	records, err := collectFixture(ctx, providerName, accountID, fixtureName, output)
	if err != nil {
		return err
	}
	return persistAndEncode(ctx, records, output, storage)
}

func RunWithRegistry(ctx context.Context, registry *provider.Registry, providerName, accountID string, now time.Time, output io.Writer, providerConfig ...map[string]string) error {
	if providerName == "" || accountID == "" || registry == nil || output == nil {
		return fmt.Errorf("provider registry, provider, account ID, and output are required")
	}
	cloudProvider, err := registry.Resolve(providerName)
	if err != nil {
		return err
	}
	start, end := PreviousSevenCompleteUTCDays(now)
	request := provider.CollectRequest{
		AccountID:      accountID,
		ProviderConfig: firstProviderConfig(providerConfig),
		StartTime:      start,
		EndTime:        end,
		CollectionID:   providerName + ":" + accountID + ":" + start.Format("2006-01-02"),
	}
	return RunProvider(ctx, cloudProvider, request, output)
}

func RunWithRegistryAndStorage(ctx context.Context, registry *provider.Registry, providerName, accountID string, now time.Time, output io.Writer, storage CostWriter, providerConfig ...map[string]string) error {
	if providerName == "" || accountID == "" || registry == nil || output == nil || storage == nil {
		return fmt.Errorf("provider registry, provider, account ID, output, and cost storage are required")
	}
	cloudProvider, err := registry.Resolve(providerName)
	if err != nil {
		return err
	}
	start, end := PreviousSevenCompleteUTCDays(now)
	request := provider.CollectRequest{AccountID: accountID, ProviderConfig: firstProviderConfig(providerConfig), StartTime: start, EndTime: end, CollectionID: providerName + ":" + accountID + ":" + start.Format("2006-01-02")}
	return RunProviderWithStorage(ctx, cloudProvider, request, output, storage)
}

func firstProviderConfig(configs []map[string]string) map[string]string {
	if len(configs) == 0 {
		return nil
	}
	return configs[0]
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

func RunProviderWithStorage(ctx context.Context, cloudProvider provider.Provider, request provider.CollectRequest, output io.Writer, storage CostWriter) error {
	if cloudProvider == nil || output == nil || storage == nil {
		return fmt.Errorf("provider, output, and cost storage are required")
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
	return persistAndEncode(ctx, records, output, storage)
}

func persistAndEncode(ctx context.Context, records []provider.RawCostRecord, output io.Writer, storage CostWriter) error {
	normalized := make([]normalize.CostRecord, 0, len(records))
	for _, record := range records {
		cost, err := normalize.Normalize(record)
		if err != nil {
			return fmt.Errorf("normalize cost record: %w", err)
		}
		normalized = append(normalized, cost)
	}
	if err := storage.WriteCosts(ctx, normalized); err != nil {
		return fmt.Errorf("persist cost records: %w", err)
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
