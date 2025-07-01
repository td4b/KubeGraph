package main

import (
	"KubeGraph/models"
	"KubeGraph/resolvers"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

func main() {
	var rulesPath string
	var inputPath string

	var rootCmd = &cobra.Command{
		Use:   "kubegraph",
		Short: "KubeGraph - Apply rules to Kubernetes YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Run(rulesPath, inputPath, os.Stdin, os.Stdout)
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

func Run(rulesPath string, inputPath string, stdin io.Reader, stdout io.Writer) error {
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

	var input []byte
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		input, _ = io.ReadAll(stdin)
	} else if inputPath != "" {
		input = loadInputFromPath(inputPath)
	} else {
		input = loadInputFromPath(".")
	}

	resourceGraph := resolvers.BuildGraph(input, rulesDir, rules, vars)

	for _, res := range resourceGraph {
		out, _ := yaml.Marshal(res.Data)
		fmt.Fprintf(stdout, "---\n%s", out)
	}

	return nil
}

func loadInputFromPath(path string) []byte {
	var combined bytes.Buffer
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") || strings.HasSuffix(info.Name(), ".yml") {
			data, _ := os.ReadFile(p)
			combined.Write(data)
			combined.WriteString("\n---\n")
		}
		return nil
	})
	return combined.Bytes()
}
