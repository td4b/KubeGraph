package main

import (
	"KubeGraph/models"
	"KubeGraph/resolvers"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func Run(rulesPath, inputPath string, in io.Reader, out io.Writer) error {
	rulesDir := filepath.Dir(rulesPath)

	valuesData, err := os.ReadFile("values.yaml")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var vars map[string]interface{}
	if len(valuesData) > 0 {
		if err := yaml.Unmarshal(valuesData, &vars); err != nil {
			return err
		}
	} else {
		vars = map[string]interface{}{}
	}

	rulesRaw, _ := os.ReadFile(rulesPath)
	rulesTmpl, _ := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
	var renderedRules bytes.Buffer
	rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars})
	var rules models.RulesFile
	yaml.Unmarshal(renderedRules.Bytes(), &rules)

	var stdin []byte
	if in != nil {
		stdin, _ = io.ReadAll(in)
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			stdin, _ = io.ReadAll(os.Stdin)
		} else if inputPath != "" {
			stdin = resolvers.LoadInputFromPath(inputPath)
		} else {
			stdin = resolvers.LoadInputFromPath(".")
		}
	}

	resourceGraph := resolvers.BuildGraph(stdin, rulesDir, rules, vars)

	for _, res := range resourceGraph {
		y, _ := yaml.Marshal(res.Data)
		fmt.Fprintf(out, "---\n%s", y)
	}

	return nil
}

func main() {
	var rulesPath string
	var inputPath string

	var rootCmd = &cobra.Command{
		Use:   "kubegraph",
		Short: "KubeGraph - Apply rules to Kubernetes YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Run(rulesPath, inputPath, nil, os.Stdout)
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
