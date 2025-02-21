package command

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/argoproj/pkg/stats"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/argoproj/argo-cd/v2/reposerver/apiclient"
	logutils "github.com/argoproj/argo-cd/v2/util/log"
	"github.com/argoproj/argo-cd/v2/util/tls"

	"github.com/argoproj/argo-cd/v2/applicationset/controllers"
	"github.com/argoproj/argo-cd/v2/applicationset/generators"
	"github.com/argoproj/argo-cd/v2/applicationset/utils"
	"github.com/argoproj/argo-cd/v2/applicationset/webhook"
	cmdutil "github.com/argoproj/argo-cd/v2/cmd/util"
	"github.com/argoproj/argo-cd/v2/common"
	"github.com/argoproj/argo-cd/v2/util/env"
	"github.com/argoproj/argo-cd/v2/util/github_app"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	appsetmetrics "github.com/argoproj/argo-cd/v2/applicationset/metrics"
	"github.com/argoproj/argo-cd/v2/applicationset/services"
	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/cli"
	"github.com/argoproj/argo-cd/v2/util/db"
	"github.com/argoproj/argo-cd/v2/util/errors"
	argosettings "github.com/argoproj/argo-cd/v2/util/settings"
)

var gitSubmoduleEnabled = env.ParseBoolFromEnv(common.EnvGitSubmoduleEnabled, true)

func NewCommand() *cobra.Command {
	var (
		clientConfig                        clientcmd.ClientConfig
		metricsAddr                         string
		probeBindAddr                       string
		webhookAddr                         string
		enableLeaderElection                bool
		applicationSetNamespaces            []string
		argocdRepoServer                    string
		policy                              string
		enablePolicyOverride                bool
		debugLog                            bool
		dryRun                              bool
		enableProgressiveSyncs              bool
		enableNewGitFileGlobbing            bool
		repoServerPlaintext                 bool
		repoServerStrictTLS                 bool
		repoServerTimeoutSeconds            int
		maxConcurrentReconciliations        int
		scmRootCAPath                       string
		allowedScmProviders                 []string
		globalPreservedAnnotations          []string
		globalPreservedLabels               []string
		metricsAplicationsetLabels          []string
		enableScmProviders                  bool
		webhookParallelism                  int
		tokenRefStrictMode                  bool
		namespacedModeMultiNamespaceEnabled bool
		applicationNamespaces               []string
	)
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appv1alpha1.AddToScheme(scheme)
	command := cobra.Command{
		Use:   "controller",
		Short: "Starts Argo CD ApplicationSet controller",
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()

			vers := common.GetVersion()
			namespace, _, err := clientConfig.Namespace()
			applicationSetNamespaces = append(applicationSetNamespaces, namespace)

			errors.CheckError(err)
			vers.LogStartupInfo(
				"ArgoCD ApplicationSet Controller",
				map[string]any{
					"namespace": namespace,
				},
			)

			cli.SetLogFormat(cmdutil.LogFormat)
			cli.SetLogLevel(cmdutil.LogLevel)

			ctrl.SetLogger(logutils.NewLogrusLogger(logutils.NewWithCurrentConfig()))

			// Recover from panic and log the error using the configured logger instead of the default.
			defer func() {
				if r := recover(); r != nil {
					log.WithField("trace", string(debug.Stack())).Fatal("Recovered from panic: ", r)
				}
			}()

			restConfig, err := clientConfig.ClientConfig()
			errors.CheckError(err)

			restConfig.UserAgent = fmt.Sprintf("argocd-applicationset-controller/%s (%s)", vers.Version, vers.Platform)

			policyObj, exists := utils.Policies[policy]
			if !exists {
				log.Error("Policy value can be: sync, create-only, create-update, create-delete, default value: sync")
				os.Exit(1)
			}

			cfg := ctrl.GetConfigOrDie()
			err = appv1alpha1.SetK8SConfigDefaults(cfg)
			if err != nil {
				log.Error(err, "Unable to apply K8s REST config defaults")
				os.Exit(1)
			}

			dynamicClient, err := dynamic.NewForConfig(cfg) //mgr.GetConfig())
			errors.CheckError(err)
			k8sClient, err := kubernetes.NewForConfig(cfg) //mgr.GetConfig())
			errors.CheckError(err)

			// ******
			tlsConfig := apiclient.TLSConfiguration{
				DisableTLS:       repoServerPlaintext,
				StrictValidation: repoServerStrictTLS,
			}

			if !repoServerPlaintext && repoServerStrictTLS {
				pool, err := tls.LoadX509CertPool(
					fmt.Sprintf("%s/reposerver/tls/tls.crt", env.StringFromEnv(common.EnvAppConfigPath, common.DefaultAppConfigPath)),
					fmt.Sprintf("%s/reposerver/tls/ca.crt", env.StringFromEnv(common.EnvAppConfigPath, common.DefaultAppConfigPath)),
				)
				errors.CheckError(err)
				tlsConfig.Certificates = pool
			}

			webhookMux := http.NewServeMux()

			newControllerInstance(ctx, cfg, scheme, dryRun, namespacedModeMultiNamespaceEnabled, applicationNamespaces, applicationSetNamespaces, k8sClient, namespace, scmRootCAPath, allowedScmProviders, enableScmProviders, tokenRefStrictMode, repoServerTimeoutSeconds, tlsConfig, dynamicClient, webhookMux, webhookParallelism, enableNewGitFileGlobbing, policyObj, enablePolicyOverride, enableProgressiveSyncs, globalPreservedAnnotations, globalPreservedLabels, maxConcurrentReconciliations, metricsAplicationsetLabels)

			startWebhookServer(webhookMux, webhookAddr)

			return nil
		},
	}
	clientConfig = cli.AddKubectlFlagsToCmd(&command)
	command.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	command.Flags().StringVar(&probeBindAddr, "probe-addr", ":8081", "The address the probe endpoint binds to.")
	command.Flags().StringVar(&webhookAddr, "webhook-addr", ":7000", "The address the webhook endpoint binds to.")
	command.Flags().BoolVar(&enableLeaderElection, "enable-leader-election", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_LEADER_ELECTION", false),
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	command.Flags().StringSliceVar(&applicationSetNamespaces, "applicationset-namespaces", env.StringsFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_NAMESPACES", []string{}, ","), "Argo CD applicationset namespaces")
	command.Flags().StringVar(&argocdRepoServer, "argocd-repo-server", env.StringFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_REPO_SERVER", common.DefaultRepoServerAddr), "Argo CD repo server address")
	command.Flags().StringVar(&policy, "policy", env.StringFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_POLICY", ""), "Modify how application is synced between the generator and the cluster. Default is '' (empty), which means AppSets default to 'sync', but they may override that default. Setting an explicit value prevents AppSet-level overrides, unless --allow-policy-override is enabled. Explicit options are: 'sync' (create & update & delete), 'create-only', 'create-update' (no deletion), 'create-delete' (no update)")
	command.Flags().BoolVar(&enablePolicyOverride, "enable-policy-override", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_POLICY_OVERRIDE", policy == ""), "For security reason if 'policy' is set, it is not possible to override it at applicationSet level. 'allow-policy-override' allows user to define their own policy")
	command.Flags().BoolVar(&debugLog, "debug", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_DEBUG", false), "Print debug logs. Takes precedence over loglevel")
	command.Flags().StringVar(&cmdutil.LogFormat, "logformat", env.StringFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_LOGFORMAT", "text"), "Set the logging format. One of: text|json")
	command.Flags().StringVar(&cmdutil.LogLevel, "loglevel", env.StringFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_LOGLEVEL", "info"), "Set the logging level. One of: debug|info|warn|error")
	command.Flags().StringSliceVar(&allowedScmProviders, "allowed-scm-providers", env.StringsFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ALLOWED_SCM_PROVIDERS", []string{}, ","), "The list of allowed custom SCM provider API URLs. This restriction does not apply to SCM or PR generators which do not accept a custom API URL. (Default: Empty = all)")
	command.Flags().BoolVar(&enableScmProviders, "enable-scm-providers", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_SCM_PROVIDERS", true), "Enable retrieving information from SCM providers, used by the SCM and PR generators (Default: true)")
	command.Flags().BoolVar(&dryRun, "dry-run", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_DRY_RUN", false), "Enable dry run mode")
	command.Flags().BoolVar(&tokenRefStrictMode, "token-ref-strict-mode", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_TOKENREF_STRICT_MODE", false), fmt.Sprintf("Set to true to require secrets referenced by SCM providers to have the %s=%s label set (Default: false)", common.LabelKeySecretType, common.LabelValueSecretTypeSCMCreds))
	command.Flags().BoolVar(&enableProgressiveSyncs, "enable-progressive-syncs", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_PROGRESSIVE_SYNCS", false), "Enable use of the experimental progressive syncs feature.")
	command.Flags().BoolVar(&enableNewGitFileGlobbing, "enable-new-git-file-globbing", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_NEW_GIT_FILE_GLOBBING", false), "Enable new globbing in Git files generator.")
	command.Flags().BoolVar(&repoServerPlaintext, "repo-server-plaintext", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_REPO_SERVER_PLAINTEXT", false), "Disable TLS on connections to repo server")
	command.Flags().BoolVar(&repoServerStrictTLS, "repo-server-strict-tls", env.ParseBoolFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_REPO_SERVER_STRICT_TLS", false), "Whether to use strict validation of the TLS cert presented by the repo server")
	command.Flags().IntVar(&repoServerTimeoutSeconds, "repo-server-timeout-seconds", env.ParseNumFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_REPO_SERVER_TIMEOUT_SECONDS", 60, 0, math.MaxInt64), "Repo server RPC call timeout seconds.")
	command.Flags().IntVar(&maxConcurrentReconciliations, "concurrent-reconciliations", env.ParseNumFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_CONCURRENT_RECONCILIATIONS", 10, 1, 100), "Max concurrent reconciliations limit for the controller")
	command.Flags().StringVar(&scmRootCAPath, "scm-root-ca-path", env.StringFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_SCM_ROOT_CA_PATH", ""), "Provide Root CA Path for self-signed TLS Certificates")
	command.Flags().StringSliceVar(&globalPreservedAnnotations, "preserved-annotations", env.StringsFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_GLOBAL_PRESERVED_ANNOTATIONS", []string{}, ","), "Sets global preserved field values for annotations")
	command.Flags().StringSliceVar(&globalPreservedLabels, "preserved-labels", env.StringsFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_GLOBAL_PRESERVED_LABELS", []string{}, ","), "Sets global preserved field values for labels")
	command.Flags().IntVar(&webhookParallelism, "webhook-parallelism-limit", env.ParseNumFromEnv("ARGOCD_APPLICATIONSET_CONTROLLER_WEBHOOK_PARALLELISM_LIMIT", 50, 1, 1000), "Number of webhook requests processed concurrently")
	command.Flags().StringSliceVar(&metricsAplicationsetLabels, "metrics-applicationset-labels", []string{}, "List of Application labels that will be added to the argocd_applicationset_labels metric")
	command.Flags().BoolVar(&namespacedModeMultiNamespaceEnabled, "namespaced-mode-multi-namespace-enabled", env.ParseBoolFromEnv("ARGOCD_NAMESPACED_MODE_MULTI_NAMESPACE_ENABLED", false), "Allows the Argo CD notification controller to watch multiple namespaces having applications and create notifications eventhough the controller is running in namespaced scope")
	command.Flags().StringSliceVar(&applicationNamespaces, "application-namespaces", env.StringsFromEnv("ARGOCD_APPLICATION_NAMESPACES", []string{}, ","), "List of additional namespaces that this controller should send notifications for")
	return &command
}

func createWebhookHandlers(mux *http.ServeMux, namespace string, webhookParallelism int, argoSettingsMgr *argosettings.SettingsManager, mgr manager.Manager, topLevelGenerators map[string]generators.Generator) {
	var err error
	var webhookHandler *webhook.WebhookHandler
	webhookHandler, err = webhook.NewWebhookHandler(namespace, webhookParallelism, argoSettingsMgr, mgr.GetClient(), topLevelGenerators)
	if err != nil {
		log.Error(err, "failed to create webhook handler")
	}
	if webhookHandler != nil {
		mux.HandleFunc("/api/webhook/"+namespace, webhookHandler.Handler)
	}
}

func startWebhookServer(mux *http.ServeMux, webhookAddr string) {
	go func() {
		log.Infof("Starting webhook server %s", webhookAddr)
		err := http.ListenAndServe(webhookAddr, mux)
		if err != nil {
			log.Error(err, "failed to start webhook server")
			os.Exit(1)
		}
	}()
}

func newControllerInstance(ctx context.Context, cfg *rest.Config, scheme *runtime.Scheme, dryRun bool, namespacedModeMultiNamespaceEnabled bool, applicationNamespaces []string, applicationSetNamespaces []string, k8sClient *kubernetes.Clientset, ns string, scmRootCAPath string, allowedScmProviders []string, enableScmProviders, tokenRefStrictMode bool, repoServerTimeoutSeconds int, tlsConfig apiclient.TLSConfiguration, dynamicClient *dynamic.DynamicClient, webhookMux *http.ServeMux, webhookParallelism int, enableNewGitFileGlobbing bool, policyObj appv1alpha1.ApplicationsSyncPolicy, enablePolicyOverride, enableProgressiveSyncs bool, globalPreservedAnnotations []string, globalPreservedLabels []string, maxConcurrentReconciliations int, metricsAplicationsetLabels []string) {
	var namespaces []string
	if namespacedModeMultiNamespaceEnabled {
		namespaces = applicationNamespaces
	} else {
		namespaces = []string{ns}
	}

	for _, namespace := range namespaces {
		go func() {

			// By default, watch all namespaces
			var watchedNamespace string = ""

			// If the applicationset-namespaces contains only one namespace it corresponds to the current namespace
			if len(applicationSetNamespaces) == 1 {
				watchedNamespace = (applicationSetNamespaces)[0]
			} else if enableScmProviders && len(allowedScmProviders) == 0 {
				log.Error("When enabling applicationset in any namespace using applicationset-namespaces, you must either set --enable-scm-providers=false or specify --allowed-scm-providers")
				os.Exit(1)
			}

			var cacheOpt ctrlcache.Options

			if watchedNamespace != "" {
				cacheOpt = ctrlcache.Options{
					DefaultNamespaces: map[string]ctrlcache.Config{
						watchedNamespace: {},
					},
				}
			}

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: scheme,
				Metrics: metricsserver.Options{
					BindAddress: "0", //metricsAddr,
				},
				Cache:                  cacheOpt,
				HealthProbeBindAddress: "0",   //probeBindAddr,
				LeaderElection:         false, //enableLeaderElection,
				LeaderElectionID:       "58ac56fa.applicationsets.argoproj.io",
				Client: ctrlclient.Options{
					DryRun: &dryRun,
				},
			})
			if err != nil {
				log.Error(err, "unable to start manager")
				os.Exit(1)
			}

			metrics := appsetmetrics.NewApplicationsetMetrics(
				utils.NewAppsetLister(mgr.GetClient()),
				metricsAplicationsetLabels,
				func(appset *appv1alpha1.ApplicationSet) bool {
					return utils.IsNamespaceAllowed(applicationSetNamespaces, appset.Namespace)
				})

			argoSettingsMgr := argosettings.NewSettingsManager(ctx, k8sClient, namespace)
			argoCDDB := db.NewDB(namespace, argoSettingsMgr, k8sClient)

			scmConfig := generators.NewSCMConfig(scmRootCAPath, allowedScmProviders, enableScmProviders, github_app.NewAuthCredentials(argoCDDB.(db.RepoCredsDB)), tokenRefStrictMode)

			repoClientset := apiclient.NewRepoServerClientset(fmt.Sprintf("argocd-repo-server.%s.svc.cluster.local:8081", ns), repoServerTimeoutSeconds, tlsConfig)
			argoCDService, err := services.NewArgoCDService(argoCDDB.GetRepository, gitSubmoduleEnabled, repoClientset, enableNewGitFileGlobbing)
			errors.CheckError(err)

			topLevelGenerators := generators.GetGenerators(ctx, mgr.GetClient(), k8sClient, namespace, argoCDService, dynamicClient, scmConfig)
			createWebhookHandlers(webhookMux, namespace, webhookParallelism, argoSettingsMgr, mgr, topLevelGenerators)

			if err = (&controllers.ApplicationSetReconciler{
				Generators:                 topLevelGenerators,
				Client:                     mgr.GetClient(),
				Scheme:                     mgr.GetScheme(),
				Recorder:                   mgr.GetEventRecorderFor("applicationset-controller"),
				Renderer:                   &utils.Render{},
				Policy:                     policyObj,
				EnablePolicyOverride:       enablePolicyOverride,
				KubeClientset:              k8sClient,
				ArgoDB:                     argoCDDB,
				ArgoCDNamespace:            namespace,
				ApplicationSetNamespaces:   applicationSetNamespaces,
				EnableProgressiveSyncs:     enableProgressiveSyncs,
				SCMRootCAPath:              scmRootCAPath,
				GlobalPreservedAnnotations: globalPreservedAnnotations,
				GlobalPreservedLabels:      globalPreservedLabels,
				Metrics:                    &metrics,
			}).SetupWithManager(mgr, enableProgressiveSyncs, maxConcurrentReconciliations); err != nil {
				log.Error(err, "unable to create controller", "controller", "ApplicationSet")
				os.Exit(1)
			}
			stats.StartStatsTicker(10 * time.Minute)
			log.Info("Starting manager")
			if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
				log.Error(err, "problem running manager")
				os.Exit(1)
			}
		}()
	}
}

func generateRandomHexString(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
