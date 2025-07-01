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
		Run: func(cmd *cobra.Command, args []string) {

			// Rules dir
			rulesDir := filepath.Dir(rulesPath)

			// Load values.yaml
			valuesData, err := os.ReadFile("values.yaml")
			if err != nil && !os.IsNotExist(err) {
				panic(err)
			}
			var vars map[string]interface{}
			if len(valuesData) > 0 {
				if err := yaml.Unmarshal(valuesData, &vars); err != nil {
					panic(err)
				}
			} else {
				vars = map[string]interface{}{}
			}

			// Load & render rules
			rulesRaw, _ := os.ReadFile(rulesPath)
			rulesTmpl, _ := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
			var renderedRules bytes.Buffer
			rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars})
			var rules models.RulesFile
			yaml.Unmarshal(renderedRules.Bytes(), &rules)

			// Load stdin or fallback
			stat, _ := os.Stdin.Stat()
			var stdin []byte
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				stdin, _ = io.ReadAll(os.Stdin)
			} else if inputPath != "" {
				stdin = loadInputFromPath(inputPath)
			} else {
				stdin = loadInputFromPath(".")
			}

			docs := strings.Split(string(stdin), "---")
			resourceGraph := []models.Resource{}

			resourceGraph = resolvers.BuildGraph(rulesDir, rules, vars, resourceGraph)
			resourceGraph = resolvers.HandleInputs(docs, resourceGraph, vars)

			for _, res := range resourceGraph {
				out, _ := yaml.Marshal(res.Data)
				fmt.Printf("---\n%s", out)
			}
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
