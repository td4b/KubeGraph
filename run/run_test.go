package run

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/td4b/KubeGraph/models"
	"gopkg.in/yaml.v3"
)

func TestKubeGraph(t *testing.T) {
	// 🟢 Run the same `kustomize build` as a subprocess
	cmd := exec.Command("kustomize", "build", "../ArgoCD/SampleApp/tests/.")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}

	var inputBuf bytes.Buffer
	multiWriter := io.MultiWriter(&inputBuf)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start kustomize: %v", err)
	}

	// 🟢 Tee the stdout so we can capture the raw input too
	go func() {
		io.Copy(multiWriter, stdout)
	}()

	// 🟢 Wait for kustomize to finish before running Run()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("kustomize did not exit cleanly: %v", err)
	}

	rawInput := inputBuf.Bytes()
	fmt.Println("====== RAW INPUT YAML ======")
	fmt.Println(string(rawInput))

	// 🟢 Load and render the rules template for debug
	rulesRaw, err := os.ReadFile("../ArgoCD/SampleApp/rules.yaml")
	if err != nil {
		t.Fatalf("failed to read rules.yaml: %v", err)
	}

	// Simulate minimal vars block
	var vars map[string]interface{} = map[string]interface{}{}
	rulesTmpl, err := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
	if err != nil {
		t.Fatalf("failed to parse rules template: %v", err)
	}

	var renderedRules bytes.Buffer
	if err := rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars}); err != nil {
		t.Fatalf("failed to render rules template: %v", err)
	}

	fmt.Println("====== RENDERED RULES TEMPLATE ======")
	fmt.Println(renderedRules.String())

	// 🟢 Also parse rules to check for injectFile and newResources
	var rulesFile models.RulesFile
	if err := yaml.Unmarshal(renderedRules.Bytes(), &rulesFile); err != nil {
		t.Fatalf("failed to unmarshal rendered rules: %v", err)
	}

	// 🟢 If injectFile is defined, print its raw contents
	for _, rule := range rulesFile.Rules {
		if rule.Patches != "" {
			fmt.Println("====== RAW patches:", rule.Patches, "======")
			data, err := os.ReadFile("../ArgoCD/SampleApp/" + rule.Patches)
			if err != nil {
				t.Fatalf("failed to read injectFile %s: %v", rule.Patches, err)
			}
			fmt.Println(string(data))
		}

		// 🟢 If newResources are defined, print each one
		for _, newPath := range rule.NewResources {
			fmt.Println("====== RAW newResource:", newPath, "======")
			data, err := os.ReadFile("../ArgoCD/SampleApp/" + newPath)
			if err != nil {
				t.Fatalf("failed to read newResource %s: %v", newPath, err)
			}
			fmt.Println(string(data))
		}
	}

	// 🟢 Finally, call Run()
	var out bytes.Buffer
	err = Run("../ArgoCD/SampleApp/rules.yaml", "", bytes.NewReader(rawInput), &out)
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	got := out.String()
	fmt.Println("====== OUTPUT YAML ======")
	fmt.Println(got)

	if !strings.Contains(got, "kind:") {
		t.Errorf("Output does not contain expected YAML: %s", got)
	}
}
