package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	notification "ns-scoped-multi-ns/argo-cd/cmd/argocd-notification/commands"

	applicationset "ns-scoped-multi-ns/argo-cd/cmd/argocd-applicationset-controller/commands"
)

const (
	binaryNameEnv = "ARGOCD_BINARY_NAME"
)

func main() {
	var command *cobra.Command

	binaryName := filepath.Base(os.Args[0])
	if val := os.Getenv(binaryNameEnv); val != "" {
		binaryName = val
	}

	switch binaryName {
	case "argocd-notifications":
		command = notification.NewCommand()
	case "argocd-applicationset-controller":
		command = applicationset.NewCommand()

	}

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
