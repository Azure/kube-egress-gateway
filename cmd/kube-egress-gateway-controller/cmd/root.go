/*
MIT License

Copyright (c) Microsoft Corporation.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/
package cmd

import (
	goflag "flag"
	"fmt"
	"os"

	controllers "github.com/Azure/kube-egress-gateway/controllers/manager"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	networkattachmentv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	setupLog             = ctrl.Log.WithName("setup")
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	zapOpts              = zap.Options{
		Development: true,
	}
	configFile string
)

func init() {
	cobra.OnInitialize(initCloudConfig)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cloudConfigFile, "cloud-config", "/etc/kubernetes/kube-egress-gateway/azure.json", "cloud config file")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	rootCmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	rootCmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	rootCmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	zapOpts.BindFlags(goflag.CommandLine)
	rootCmd.Flags().AddGoFlagSet(goflag.CommandLine)

	utilruntime.Must(egressgatewayv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkattachmentv1.AddToScheme(scheme))
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
	var err error
	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "0a299682.microsoft.com",
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
	}
	if configFile != "" {
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
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

	var factory azureclients.AzureClientsFactory
	if cloudConfig.UseUserAssignedIdentity {
		factory, err = azureclients.NewAzureClientsFactoryWithManagedIdentity(cloudConfig.Cloud, cloudConfig.SubscriptionID, cloudConfig.UserAssignedIdentityID)
		if err != nil {
			setupLog.Error(err, "unable to create azure clients")
			os.Exit(1)
		}
	} else {
		factory, err = azureclients.NewAzureClientsFactoryWithClientSecret(cloudConfig.Cloud, cloudConfig.SubscriptionID, cloudConfig.TenantID,
			cloudConfig.AADClientID, cloudConfig.AADClientSecret)
		if err != nil {
			setupLog.Error(err, "unable to create azure clients")
			os.Exit(1)
		}
	}
	az, err := azmanager.CreateAzureManager(&cloudConfig, factory)
	if err != nil {
		setupLog.Error(err, "unable to create azure manager")
		os.Exit(1)
	}

	if err = (&controllers.StaticGatewayConfigurationReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StaticGatewayConfiguration")
		os.Exit(1)
	}
	if err = (&egressgatewayv1alpha1.StaticGatewayConfiguration{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "StaticGatewayConfiguration")
		os.Exit(1)
	}
	if err = (&controllers.GatewayLBConfigurationReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		AzureManager: az,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayLBConfiguration")
		os.Exit(1)
	}
	if err = (&controllers.GatewayVMConfigurationReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		AzureManager: az,
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
