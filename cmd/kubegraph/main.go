package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/td4b/KubeGraph/run"
)

func main() {
	var rulesPath string
	var inputPath string

	var rootCmd = &cobra.Command{
		Use:   "kubegraph",
		Short: "KubeGraph - Apply rules to Kubernetes YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.Run(rulesPath, inputPath, nil, os.Stdout)
		},
	}

	rootCmd.Flags().StringVar(&rulesPath, "rules", "", "Path to the rules.yaml file (required)")
	rootCmd.Flags().StringVar(&inputPath, "input", "", "Input file or directory if stdin is empty")
	rootCmd.MarkFlagRequired("rules")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
