package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ghdwlsgur/louder/internal/collector"
)

func main() {
	var providerName string
	var accountID string
	var fixtureName string
	flag.StringVar(&providerName, "provider", "", "The cloud provider for this collection.")
	flag.StringVar(&accountID, "account-id", "", "The billing account scope for this collection.")
	flag.StringVar(&fixtureName, "fixture", "", "The embedded fixture to collect, such as embedded:aws.")
	flag.Parse()

	if err := collector.Run(context.Background(), providerName, accountID, fixtureName, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
