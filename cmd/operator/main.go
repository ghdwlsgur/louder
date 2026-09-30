package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/controller"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	var metricsAddr string
	var probeAddr string
	var watchNamespace string
	var collectorImage string
	var collectorFixtureMode bool
	var leaderElect bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the health probes bind to.")
	flag.StringVar(&watchNamespace, "watch-namespace", "cloud-cost", "The namespace containing CloudAccount resources and credential Secrets.")
	flag.StringVar(&collectorImage, "collector-image", "", "The image containing the Collector executable.")
	flag.BoolVar(&collectorFixtureMode, "collector-fixture-mode", false, "Run synthetic embedded fixtures instead of provider APIs.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election for controller manager replicas.")
	flag.Parse()
	if collectorImage == "" {
		fmt.Fprintln(os.Stderr, "--collector-image is required")
		os.Exit(2)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{watchNamespace: {}}},
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "operator.finops.sre.local",
	})
	if err != nil {
		os.Exit(1)
	}

	if err := (&controller.CloudAccountReconciler{CollectorImage: collectorImage, CollectorFixtureMode: collectorFixtureMode}).SetupWithManager(mgr); err != nil {
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		os.Exit(1)
	}
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		os.Exit(1)
	}
}
