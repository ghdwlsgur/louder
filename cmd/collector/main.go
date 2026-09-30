package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/ghdwlsgur/louder/internal/collector"
	"github.com/ghdwlsgur/louder/internal/provider"
	awsprovider "github.com/ghdwlsgur/louder/internal/provider/aws"
	"github.com/ghdwlsgur/louder/internal/storage/clickhouse"
)

func main() {
	var providerName string
	var accountID string
	var fixtureName string
	flag.StringVar(&providerName, "provider", "", "The cloud provider for this collection.")
	flag.StringVar(&accountID, "account-id", "", "The billing account scope for this collection.")
	flag.StringVar(&fixtureName, "fixture", "", "The embedded fixture to collect, such as embedded:aws.")
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
	if err := runLive(ctx, providerName, accountID, time.Now(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLive(ctx context.Context, providerName, accountID string, now time.Time, output *os.File) error {
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	registry := provider.NewRegistry()
	if err := registry.Register("aws", func() (provider.Provider, error) {
		return awsprovider.NewFromConfig(awsConfig), nil
	}); err != nil {
		return err
	}
	return collector.RunWithRegistryAndStorage(ctx, registry, providerName, accountID, now, output, store)
}

func storageConfigured() bool {
	for _, name := range []string{"CLICKHOUSE_ADDR", "CLICKHOUSE_DATABASE", "CLICKHOUSE_USERNAME", "CLICKHOUSE_PASSWORD", "CLICKHOUSE_SECURE"} {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}
