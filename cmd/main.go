package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
	"github.com/garamsh/gagent-operator/internal/controller"
	"github.com/garamsh/gagent-operator/internal/garam"
	"github.com/garamsh/gagent-operator/internal/garam/constructor"
	"github.com/garamsh/gagent-operator/internal/garam/credentialstore"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// podNamespaceVariable names the environment variable the manager's Pod carries
// its own namespace in. It is where the Secret holding this operator's
// credential is looked for, and where the agents it constructs are built.
const podNamespaceVariable = "POD_NAMESPACE"

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(agentv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var garamAddress, garamCertificateFile, garamKeyFile, garamTrustFile string
	var garamCredentialSecret string
	var agentImage, agentStorageSize, agentCopyImage string
	var garamPollInterval, garamRenewalInterval time.Duration
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&garamAddress, "garam-address", "",
		"The host and port of garam's machine listener. Unset leaves this operator reading no definitions.")
	flag.StringVar(&garamCertificateFile, "garam-certificate-file", "",
		"The file holding the certificate this operator authenticates to garam with.")
	flag.StringVar(&garamKeyFile, "garam-key-file", "",
		"The file holding the private key of the certificate this operator authenticates to garam with.")
	flag.StringVar(&garamTrustFile, "garam-trust-file", "",
		"The file holding what garam's machine listener is verified against. This is not the organization "+
			"issuer an operator's own certificate arrives with.")
	flag.DurationVar(&garamPollInterval, "garam-poll-interval", time.Minute,
		"How often this operator reads the definitions garam holds for it.")
	flag.StringVar(&garamCredentialSecret, "garam-credential-secret", "",
		"The Secret in this Pod's own namespace holding the certificate and key named above, "+
			"which a renewal is written back to. Unset leaves this operator renewing nothing.")
	flag.DurationVar(&garamRenewalInterval, "garam-renewal-interval", time.Hour,
		"How often this operator asks garam to replace the certificate it authenticates with.")
	flag.StringVar(&agentImage, "agent-image", "",
		"The container image every agent this operator constructs runs. It has no default: name the image "+
			"and the tag or digest explicitly. Required where garam-address is set.")
	flag.StringVar(&agentStorageSize, "agent-storage-size", "",
		"The size of the volume every agent this operator constructs keeps its state on, as a Kubernetes "+
			"quantity. Required where garam-address is set.")
	flag.StringVar(&agentCopyImage, "agent-copy-image", "",
		"The image the init container of every agent's Pod runs to copy that agent's credential into the "+
			"volume the agent reads it from. It needs a shell and install, and nothing of the agent. It has "+
			"no default and is always required: an agent whose credential arrives any other way is one its "+
			"reader refuses.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b97af7b3.garam.sh",
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
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	// Every agent's Pod carries the init container this image runs, so an
	// operator without one builds no workload at all. Refusing to start says so
	// once, where a workload built without it would say nothing until its
	// reader refused the credential.
	if agentCopyImage == "" {
		setupLog.Error(errors.New("agent-copy-image is required"),
			"Failed to configure the workload an Agent is built into")
		os.Exit(1)
	}

	if err := (&controller.AgentReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		CopyImage: agentCopyImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "agent")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if garamAddress != "" {
		tlsConfig, err := garam.MutualTLS(garamCertificateFile, garamKeyFile, garamTrustFile)
		if err != nil {
			setupLog.Error(err, "Failed to configure the connection to garam")
			os.Exit(1)
		}
		// The namespace is the Pod's own and arrives on the downward API: the
		// deployment cannot state it in an argument, because the kustomize
		// transformer that sets the namespace does not reach into one.
		namespace := os.Getenv(podNamespaceVariable)
		if namespace == "" {
			setupLog.Error(errors.New(podNamespaceVariable+" is unset"),
				"Failed to name the namespace this operator writes in")
			os.Exit(1)
		}
		// Both flags are checked for having been given, and neither value for
		// being usable: an image reference and a volume size are the API
		// server's to refuse, and it says so on the object.
		if agentImage == "" {
			setupLog.Error(errors.New("agent-image is required where garam-address is set"),
				"Failed to construct agents")
			os.Exit(1)
		}
		storageSize, err := resource.ParseQuantity(agentStorageSize)
		if err != nil {
			setupLog.Error(err, "Failed to read agent-storage-size, which is required where garam-address is set",
				"agent-storage-size", agentStorageSize)
			os.Exit(1)
		}
		garamClient := garam.NewClient(garamAddress, tlsConfig)
		builder := constructor.NewAgent(mgr.GetClient(), mgr.GetScheme(), namespace, agentImage, storageSize)
		if err := mgr.Add(garam.NewPoller(garamClient, builder, garamPollInterval)); err != nil {
			setupLog.Error(err, "Failed to add the garam poller", "address", garamAddress)
			os.Exit(1)
		}
		if garamCredentialSecret != "" {
			// The Secret's keys are the names the kubelet gives the files in
			// the volume, which is what the two flags above already point at.
			store := credentialstore.NewSecret(mgr.GetClient(),
				types.NamespacedName{Namespace: namespace, Name: garamCredentialSecret},
				filepath.Base(garamCertificateFile), filepath.Base(garamKeyFile))
			if err := mgr.Add(garam.NewRenewer(garamClient, store, garamRenewalInterval)); err != nil {
				setupLog.Error(err, "Failed to add the garam renewer", "secret", garamCredentialSecret)
				os.Exit(1)
			}
		} else {
			setupLog.Info("Renewing nothing: garam-credential-secret is unset")
		}
	} else {
		setupLog.Info("Reading no definitions: garam-address is unset")
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}
