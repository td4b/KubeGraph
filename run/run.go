package run

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/td4b/KubeGraph/models"
	"github.com/td4b/KubeGraph/resolvers"
	"gopkg.in/yaml.v2"
)

func Run(rulesPath, inputPath string, in io.Reader, out io.Writer) error {
	rulesDir := filepath.Dir(rulesPath)

	valuesPath := filepath.Join(rulesDir, "values.yaml")
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		if os.IsNotExist(err) {
			//fmt.Println("[WARN] No values.yaml found!")
		} else {
			panic(err)
		}
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
