package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

type Resource struct {
	Kind string
	Data map[string]interface{}
}

type Rule struct {
	Match        map[string]interface{} `yaml:"match"`
	Inject       map[string]interface{} `yaml:"inject"`
	InjectFile   string                 `yaml:"injectFile"`
	NewResources []string               `yaml:"newResources"` // file paths
}

type RulesFile struct {
	Rules []Rule `yaml:"rules"`
}

func resolveResource(resources []Resource, input string) interface{} {
	parts := strings.Split(input, "&")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	leftParts := strings.Split(left, ".")
	if len(leftParts) < 4 {
		panic(fmt.Sprintf("Invalid left resource selector: %s", left))
	}

	kind := leftParts[1]
	attrPath := strings.Join(leftParts[2:len(leftParts)-1], ".")
	attrVal := leftParts[len(leftParts)-1]

	for _, res := range resources {
		if strings.ToLower(res.Kind) != strings.ToLower(kind) {
			continue
		}

		val := res.Data
		segments := strings.Split(attrPath, ".")
		for _, p := range segments {
			v, ok := val[p]
			if !ok {
				val = nil
				break
			}
			if m, ok := v.(map[string]interface{}); ok {
				val = m
			} else {
				val = map[string]interface{}{p: v}
			}
		}

		matchVal := val[segments[len(segments)-1]]
		if fmt.Sprintf("%v", matchVal) == attrVal {
			target := res.Data
			for _, p := range strings.Split(right, ".") {
				v, ok := target[p]
				if !ok {
					return fmt.Sprintf("<missing: %s>", p)
				}
				if m, ok := v.(map[string]interface{}); ok {
					target = m
				} else {
					return v
				}
			}
		}
	}

	return "<no match>"
}

func matches(actual, match map[string]interface{}) bool {
	for key, expected := range match {
		actVal, ok := actual[key]
		if !ok {
			return false
		}
		switch exp := expected.(type) {
		case map[string]interface{}:
			actMap, ok := actVal.(map[string]interface{})
			if !ok {
				return false
			}
			if !matches(actMap, exp) {
				return false
			}
		default:
			if fmt.Sprintf("%v", actVal) != fmt.Sprintf("%v", exp) {
				return false
			}
		}
	}
	return true
}

func mergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			switch srcVal := v.(type) {
			case map[string]interface{}:
				if dstVal, ok := existing.(map[string]interface{}); ok {
					dst[k] = mergeMaps(dstVal, srcVal)
				} else {
					dst[k] = srcVal
				}
			case []interface{}:
				if dstVal, ok := existing.([]interface{}); ok {
					dst[k] = append(dstVal, srcVal...)
				} else {
					dst[k] = srcVal
				}
			default:
				dst[k] = srcVal
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}

func main() {
	// Load optional values.yaml
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

	// Load rules.yaml
	rulesRaw, _ := os.ReadFile("rules.yaml")
	rulesTmpl, _ := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
	var renderedRules bytes.Buffer
	rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars})
	var rules RulesFile
	yaml.Unmarshal(renderedRules.Bytes(), &rules)

	// Read stdin for base resources
	stdin, _ := io.ReadAll(os.Stdin)
	docs := strings.Split(string(stdin), "---")
	resourceGraph := []Resource{}

	// Build base graph
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		tmpl, _ := template.New("doc").Funcs(sprig.TxtFuncMap()).
			Funcs(template.FuncMap{"resource": func(input string) interface{} {
				return resolveResource(resourceGraph, input)
			}}).Parse(doc)
		var buf bytes.Buffer
		tmpl.Execute(&buf, map[string]interface{}{"var": vars})

		var parsed map[string]interface{}
		yaml.Unmarshal(buf.Bytes(), &parsed)

		resourceGraph = append(resourceGraph, Resource{
			Kind: parsed["kind"].(string),
			Data: parsed,
		})
	}

	// Apply rules
	for i := range resourceGraph {
		for _, rule := range rules.Rules {
			if matches(resourceGraph[i].Data, rule.Match) {
				if rule.Inject != nil {
					resourceGraph[i].Data = mergeMaps(resourceGraph[i].Data, rule.Inject)
				}
				if rule.InjectFile != "" {
					fileData, _ := os.ReadFile(rule.InjectFile)
					var fileMap map[string]interface{}
					yaml.Unmarshal(fileData, &fileMap)
					resourceGraph[i].Data = mergeMaps(resourceGraph[i].Data, fileMap)
				}

				for _, newPath := range rule.NewResources {
					rawNew, err := os.ReadFile(newPath)
					if err != nil {
						panic(fmt.Sprintf("Failed to read newResource file: %s", newPath))
					}

					newTmpl, _ := template.New(newPath).Funcs(sprig.TxtFuncMap()).
						Funcs(template.FuncMap{"resource": func(input string) interface{} {
							return resolveResource(resourceGraph, input)
						}}).Parse(string(rawNew))

					var buf bytes.Buffer
					newTmpl.Execute(&buf, map[string]interface{}{"var": vars})

					var newParsed map[string]interface{}
					yaml.Unmarshal(buf.Bytes(), &newParsed)

					resourceGraph = append(resourceGraph, Resource{
						Kind: newParsed["kind"].(string),
						Data: newParsed,
					})
				}
			}
		}
	}

	// Output final
	for _, res := range resourceGraph {
		out, _ := yaml.Marshal(res.Data)
		fmt.Printf("---\n%s", out)
	}
}
