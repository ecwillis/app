package main

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/app/internal"
	"github.com/docker/app/internal/packager"
	"github.com/docker/app/internal/renderer"
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/stack/kubernetes"
	"github.com/docker/cli/cli/command/stack/options"
	"github.com/docker/cli/cli/command/stack/swarm"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	deployComposeFiles  []string
	deploySettingsFiles []string
	deployEnv           []string
	deployOrchestrator  string
	deployKubeConfig    string
	deployNamespace     string
	deployStackName     string
	sendRegistryAuth    bool
}

// deployCmd represents the deploy command
func deployCmd(dockerCli *command.DockerCli) *cobra.Command {
	var opts deployOptions

	cmd := &cobra.Command{
		Use:   "deploy [<app-name>]",
		Short: "Deploy or update an application",
		Long:  `Deploy the application on either Swarm or Kubernetes.`,
		Args:  cli.RequiresMaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(dockerCli, firstOrEmpty(args), opts)
		},
	}

	cmd.Flags().StringArrayVarP(&opts.deploySettingsFiles, "settings-files", "f", []string{}, "Override settings files")
	cmd.Flags().StringArrayVarP(&opts.deployEnv, "set", "s", []string{}, "Override settings values")
	cmd.Flags().StringVarP(&opts.deployOrchestrator, "orchestrator", "o", "swarm", "Orchestrator to deploy on (swarm, kubernetes)")
	cmd.Flags().StringVarP(&opts.deployKubeConfig, "kubeconfig", "k", "", "kubeconfig file to use")
	cmd.Flags().StringVarP(&opts.deployNamespace, "namespace", "n", "default", "namespace to deploy into")
	cmd.Flags().StringVarP(&opts.deployStackName, "name", "d", "", "stack name (default: app name)")
	cmd.Flags().BoolVarP(&opts.sendRegistryAuth, "with-registry-auth", "", false, "send registry auth")
	if internal.Experimental == "on" {
		cmd.Flags().StringArrayVarP(&opts.deployComposeFiles, "compose-files", "c", []string{}, "Override Compose files")
	}
	return cmd
}

func runDeploy(dockerCli *command.DockerCli, appname string, opts deployOptions) error {
	appname, cleanup, err := packager.Extract(appname)
	if err != nil {
		return err
	}
	defer cleanup()
	deployOrchestrator := opts.deployOrchestrator
	if do, ok := os.LookupEnv("DOCKER_ORCHESTRATOR"); ok {
		deployOrchestrator = do
	}
	if deployOrchestrator != "swarm" && deployOrchestrator != "kubernetes" {
		return fmt.Errorf("orchestrator must be either 'swarm' or 'kubernetes'")
	}
	d, err := parseSettings(opts.deployEnv)
	if err != nil {
		return err
	}
	rendered, err := renderer.Render(appname, opts.deployComposeFiles, opts.deploySettingsFiles, d)
	if err != nil {
		return err
	}
	dockerCli.Initialize(&cliflags.ClientOptions{
		Common: &cliflags.CommonOptions{
			Orchestrator: deployOrchestrator,
		},
	})
	stackName := opts.deployStackName
	if stackName == "" {
		stackName = internal.AppNameFromDir(appname)
	}
	if deployOrchestrator == "swarm" {
		ctx := context.Background()
		return swarm.DeployCompose(ctx, dockerCli, rendered, options.Deploy{
			Namespace: stackName,
		})
	}
	// kube mode
	kubeCli, err := kubernetes.WrapCli(dockerCli, kubernetes.Options{
		Namespace: opts.deployNamespace,
		Config:    opts.deployKubeConfig,
	})
	if err != nil {
		return err
	}
	return kubernetes.DeployStack(kubeCli, options.Deploy{Namespace: stackName}, rendered)
}
