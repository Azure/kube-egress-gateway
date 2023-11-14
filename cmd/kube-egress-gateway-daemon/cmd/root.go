// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cmd

import (
	"context"
	goflag "flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	controllers "github.com/Azure/kube-egress-gateway/controllers/daemon"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kube-egress-gateway-daemon",
	Short: "Monitor GatewayWireguardEndpoint CR and PodWireguardEndpoint CR, configures the network namespaces, interfaces, and routes on gateway nodes",
	Long:  `Monitor GatewayWireguardEndpoint CR and PodWireguardEndpoint CR, configures the network namespaces, interfaces, and routes on gateway nodes`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: startControllers,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	cloudConfigFile    string
	cloudConfig        config.CloudConfig
	scheme             = runtime.NewScheme()
	setupLog           = ctrl.Log.WithName("setup")
	metricsPort        int
	probePort          int
	gatewayLBProbePort int
	zapOpts            = zap.Options{
		Development: true,
	}
)

func init() {
	cobra.OnInitialize(initCloudConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cloudConfigFile, "cloud-config", "/etc/kubernetes/kube-egress-gateway/azure.json", "cloud config file")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	rootCmd.Flags().IntVar(&metricsPort, "metrics-bind-port", 8080, "The port the metric endpoint binds to.")
	rootCmd.Flags().IntVar(&probePort, "health-probe-bind-port", 8081, "The port the probe endpoint binds to.")
	rootCmd.Flags().IntVar(&gatewayLBProbePort, "gateway-lb-probe-port", 8082, "The port the gateway lb probe endpoint binds to.")

	zapOpts.BindFlags(goflag.CommandLine)
	rootCmd.Flags().AddGoFlagSet(goflag.CommandLine)

	utilruntime.Must(egressgatewayv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// initCloudConfig reads in cloud config file and ENV variables if set.
func initCloudConfig() {
	if cloudConfigFile == "" {
		fmt.Fprintln(os.Stderr, "Error: cloud config file is not provided")
		os.Exit(1)
	}
	viper.SetConfigFile(cloudConfigFile)

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		fmt.Fprintln(os.Stderr, "Error: failed to find cloud config file:", cloudConfigFile)
		os.Exit(1)
	}
}

func startControllers(cmd *cobra.Command, args []string) {

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":" + strconv.Itoa(metricsPort),
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: ":" + strconv.Itoa(probePort),
		LeaderElection:         false, // daemonSet on each gateway node
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := viper.Unmarshal(&cloudConfig); err != nil {
		setupLog.Error(err, "unable to unmarshal cloud configuration file")
		os.Exit(1)
	}

	cloudConfig.TrimSpace()
	if err := cloudConfig.Validate(); err != nil {
		setupLog.Error(err, "cloud configuration is invalid")
		os.Exit(1)
	}

	var authProvider *azclient.AuthProvider
	authProvider, err = azclient.NewAuthProvider(azclient.AzureAuthConfig{
		TenantID:                    cloudConfig.TenantID,
		AADClientID:                 cloudConfig.AADClientID,
		AADClientSecret:             cloudConfig.AADClientSecret,
		UserAssignedIdentityID:      cloudConfig.UserAssignedIdentityID,
		UseManagedIdentityExtension: cloudConfig.UseManagedIdentityExtension,
	}, &arm.ClientOptions{
		AuxiliaryTenants: []string{cloudConfig.TenantID},
	})
	if err != nil {
		setupLog.Error(err, "unable to create auth provider")
		os.Exit(1)
	}
	var cred azcore.TokenCredential
	if cloudConfig.UseManagedIdentityExtension {
		cred = authProvider.ManagedIdentityCredential
	} else {
		cred = authProvider.ClientSecretCredential
	}

	factory, err := azclient.NewClientFactory(&azclient.ClientFactoryConfig{SubscriptionID: cloudConfig.SubscriptionID}, &azclient.ARMClientConfig{Cloud: cloudConfig.Cloud}, cred)
	if err != nil {
		setupLog.Error(err, "unable to create client factory")
		os.Exit(1)
	}
	az, err := azmanager.CreateAzureManager(&cloudConfig, factory)
	if err != nil {
		setupLog.Error(err, "unable to create azure manager")
		os.Exit(1)
	}

	if err := controllers.InitNodeMetadata(); err != nil {
		setupLog.Error(err, "unable to retrieve node metadata")
		os.Exit(1)
	}

	lbProbeServer := healthprobe.NewLBProbeServer(gatewayLBProbePort)
	if err := mgr.Add(manager.RunnableFunc(lbProbeServer.Start)); err != nil {
		setupLog.Error(err, "unbaled to set up gateway health probe server")
		os.Exit(1)
	}

	netnsCleanupEvents := make(chan event.GenericEvent)
	if err = (&controllers.StaticGatewayConfigurationReconciler{
		Client:        mgr.GetClient(),
		AzureManager:  az,
		TickerEvents:  netnsCleanupEvents,
		LBProbeServer: lbProbeServer,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StaticGatewayConfiguration")
		os.Exit(1)
	}

	peerCleanupEvents := make(chan event.GenericEvent)
	if err = (&controllers.PodWireguardEndpointReconciler{
		Client:       mgr.GetClient(),
		TickerEvents: peerCleanupEvents,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PodWireguardEndpoint")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	ctx := ctrl.SetupSignalHandler()
	startCleanupTicker(ctx, netnsCleanupEvents, 1*time.Minute) // clean up gwConfig network namespace every 1 min
	startCleanupTicker(ctx, peerCleanupEvents, 1*time.Minute)  // clean up wireguard peer configurations every 1 min
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func startCleanupTicker(ctx context.Context, tickerEvents chan<- event.GenericEvent, period time.Duration) {
	setupLog.Info("starting background cleanup ticker")
	log := log.FromContext(ctx)
	ticker := time.NewTicker(period)
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("stopping background cleanup ticker")
				return
			case <-ticker.C:
				event := event.GenericEvent{
					Object: &metav1.PartialObjectMetadata{
						ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: ""},
					},
				}
				tickerEvents <- event
			}
		}
	}()
}
