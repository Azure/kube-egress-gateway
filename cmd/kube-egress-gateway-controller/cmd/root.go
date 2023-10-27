// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cmd

import (
	"context"
	goflag "flag"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	controllers "github.com/Azure/kube-egress-gateway/controllers/manager"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	//+kubebuilder:scaffold:imports
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kube-egress-gateway-controller",
	Short: "Monitor StaticGatewayConfiguration CR events, and manage GatewayLBConfiguration and GatewayVMConfiguration",
	Long:  `Monitor StaticGatewayConfiguration CR events, and manage GatewayLBConfiguration and GatewayVMConfiguration`,
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
	cloudConfigFile      string
	cloudConfig          config.CloudConfig
	scheme               = runtime.NewScheme()
	metricsPort          int
	enableLeaderElection bool
	probePort            int
	zapOpts              = zap.Options{
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
	rootCmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	zapOpts.BindFlags(goflag.CommandLine)
	rootCmd.Flags().AddGoFlagSet(goflag.CommandLine)

	utilruntime.Must(egressgatewayv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	logger := zap.New(zap.UseFlagOptions(&zapOpts))
	ctrl.SetLogger(logger)
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
	var err error
	var setupLog = ctrl.Log.WithName("setup")

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":" + strconv.Itoa(metricsPort),
		},
		WebhookServer:           webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress:  ":" + strconv.Itoa(probePort),
		LeaderElection:          enableLeaderElection,
		LeaderElectionNamespace: "kube-system",
		LeaderElectionID:        "0a299682.microsoft.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		LeaderElectionReleaseOnCancel: true,
		BaseContext: func() context.Context {
			return ctrl.LoggerInto(context.Background(), ctrl.Log)
		},
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
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
		UseManagedIdentityExtension: cloudConfig.UseUserAssignedIdentity,
	}, &arm.ClientOptions{
		AuxiliaryTenants: []string{cloudConfig.TenantID},
		ClientOptions: azcore.ClientOptions{
			Telemetry: policy.TelemetryOptions{
				ApplicationID: cloudConfig.UserAgent,
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create auth provider")
		os.Exit(1)
	}
	var cred azcore.TokenCredential
	if cloudConfig.UseUserAssignedIdentity {
		cred = authProvider.ManagedIdentityCredential
	} else {
		cred = authProvider.ClientSecretCredential
	}
	var factory azclient.ClientFactory
	factory, err = azclient.NewClientFactory(&azclient.ClientFactoryConfig{SubscriptionID: cloudConfig.SubscriptionID}, &azclient.ARMClientConfig{Cloud: cloudConfig.Cloud, UserAgent: cloudConfig.UserAgent}, cred)
	if err != nil {
		setupLog.Error(err, "unable to create client factory")
		os.Exit(1)
	}
	az, err := azmanager.CreateAzureManager(&cloudConfig, factory)
	if err != nil {
		setupLog.Error(err, "unable to create azure manager")
		os.Exit(1)
	}

	if err = (&controllers.StaticGatewayConfigurationReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("staticGatewayConfiguration-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StaticGatewayConfiguration")
		os.Exit(1)
	}
	//if err = (&egressgatewayv1alpha1.StaticGatewayConfiguration{}).SetupWebhookWithManager(mgr); err != nil {
	//	setupLog.Error(err, "unable to create webhook", "webhook", "StaticGatewayConfiguration")
	//	os.Exit(1)
	//}
	if err = (&controllers.GatewayLBConfigurationReconciler{
		Client:       mgr.GetClient(),
		AzureManager: az,
		Recorder:     mgr.GetEventRecorderFor("gatewayLBConfiguration-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayLBConfiguration")
		os.Exit(1)
	}
	if err = (&controllers.GatewayVMConfigurationReconciler{
		Client:       mgr.GetClient(),
		AzureManager: az,
		Recorder:     mgr.GetEventRecorderFor("gatewayVMConfiguration-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayVMConfiguration")
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
