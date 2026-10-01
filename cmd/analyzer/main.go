package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/analyzer"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/storage/clickhouse"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	namespace := flag.String("namespace", "cloud-cost", "The namespace containing the BudgetPolicy, accounts, and notification policies.")
	policyName := flag.String("budget-policy", "", "The BudgetPolicy name to evaluate.")
	flag.Parse()
	if *policyName == "" {
		fmt.Fprintln(os.Stderr, "--budget-policy is required")
		os.Exit(2)
	}
	if err := run(context.Background(), *namespace, *policyName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, namespace, policyName string) (runErr error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.New("Kubernetes configuration is unavailable")
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	kube, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return errors.New("Kubernetes client could not be created")
	}
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil && runErr == nil {
			runErr = err
		}
	}()
	_, err = analyzer.RunBudgetPolicy(ctx, kube, store, namespace, policyName, func(endpoint string) (notifier.Notifier, error) {
		return notifier.NewTeamsWebhookNotifier(endpoint, nil)
	}, time.Now())
	return err
}
