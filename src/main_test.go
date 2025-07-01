package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestKubeGraph(t *testing.T) {
	inputYaml, err := exec.Command("kustomize", "build", "../ArgoCD/SampleApp/tests/.").Output()
	if err != nil {
		t.Fatalf("failed to run kustomize: %v", err)
	}

	var out bytes.Buffer
	err = Run("../ArgoCD/SampleApp/rules.yaml", "", bytes.NewReader(inputYaml), &out)
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "kind:") {
		t.Errorf("Output does not contain expected YAML: %s", got)
	}
}
