package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/ghdwlsgur/louder/internal/collector"
	"github.com/ghdwlsgur/louder/internal/provider"
	awsprovider "github.com/ghdwlsgur/louder/internal/provider/aws"
	gcpprovider "github.com/ghdwlsgur/louder/internal/provider/gcp"
	"github.com/ghdwlsgur/louder/internal/storage/clickhouse"
)

func main() {
	var providerName string
	var accountID string
	var fixtureName string
	var providerConfigJSON string
	flag.StringVar(&providerName, "provider", "", "The cloud provider for this collection.")
	flag.StringVar(&accountID, "account-id", "", "The billing account scope for this collection.")
	flag.StringVar(&fixtureName, "fixture", "", "The embedded fixture to collect, such as embedded:aws.")
	flag.StringVar(&providerConfigJSON, "provider-config", "{}", "JSON object with non-secret provider-specific settings.")
	flag.Parse()

	ctx := context.Background()
	if fixtureName != "" {
		var err error
		if storageConfigured() {
			var store *clickhouse.Store
			store, err = clickhouse.OpenFromEnv(ctx)
			if err == nil {
				defer store.Close()
				err = collector.RunWithStorage(ctx, providerName, accountID, fixtureName, os.Stdout, store)
			}
		} else {
			err = collector.Run(ctx, providerName, accountID, fixtureName, os.Stdout)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var providerConfig map[string]string
	if err := json.Unmarshal([]byte(providerConfigJSON), &providerConfig); err != nil {
		fmt.Fprintln(os.Stderr, "invalid provider config JSON")
		os.Exit(1)
	}
	if err := runLive(ctx, providerName, accountID, providerConfig, time.Now(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLive(ctx context.Context, providerName, accountID string, providerConfig map[string]string, now time.Time, output *os.File) error {
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	registry := provider.NewRegistry()
	if err := registry.Register("aws", func() (provider.Provider, error) {
		awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
		if err != nil {
			return nil, fmt.Errorf("load AWS SDK configuration: %w", err)
		}
		return awsprovider.NewFromConfig(awsConfig), nil
	}); err != nil {
		return err
	}
	if err := registry.Register("gcp", func() (provider.Provider, error) { return gcpprovider.New(), nil }); err != nil {
		return err
	}
	return collector.RunWithRegistryAndStorage(ctx, registry, providerName, accountID, now, output, store, providerConfig)
}

func storageConfigured() bool {
	for _, name := range []string{"CLICKHOUSE_ADDR", "CLICKHOUSE_DATABASE", "CLICKHOUSE_USERNAME", "CLICKHOUSE_PASSWORD", "CLICKHOUSE_SECURE"} {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}
