package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
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
	NewResources []string               `yaml:"newResources"`
}

type RulesFile struct {
	Rules []Rule `yaml:"rules"`
}

func resolveResource(resources []Resource, input string) interface{} {
	parts := strings.Split(input, "&")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	switch len(parts) {
	case 1:
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))

	case 2:
		left := parts[0]
		right := parts[1]

		leftParts := strings.Split(left, ".")
		if len(leftParts) == 2 {
			// Direct match: just pick first resource of that kind
			kind := leftParts[1]
			for _, res := range resources {
				if strings.ToLower(res.Kind) != strings.ToLower(kind) {
					continue
				}
				return walk(res.Data, strings.Split(right, "."))
			}
			return "<no match>"
		}

		// Classic filter
		if len(leftParts) < 4 {
			panic(fmt.Sprintf("Invalid left selector: %s", left))
		}

		kind := leftParts[1]
		attrPath := strings.Join(leftParts[2:len(leftParts)-1], ".")
		attrVal := leftParts[len(leftParts)-1]

		for _, res := range resources {
			if strings.ToLower(res.Kind) != strings.ToLower(kind) {
				continue
			}

			val := walk(res.Data, strings.Split(attrPath, "."))
			if fmt.Sprintf("%v", val) == attrVal {
				return walk(res.Data, strings.Split(right, "."))
			}
		}
		return "<no match>"

	case 3:
		left := parts[0]
		mapMatch := parts[1]
		right := parts[2]

		leftParts := strings.Split(left, ".")
		if len(leftParts) < 3 {
			panic(fmt.Sprintf("Invalid left selector: %s", left))
		}

		kind := leftParts[1]
		attrPath := strings.Join(leftParts[2:], ".")

		mapKV := strings.SplitN(mapMatch, ".", 2)
		if len(mapKV) != 2 {
			panic(fmt.Sprintf("Invalid map match: %s", mapMatch))
		}
		mapKey := mapKV[0]
		mapVal := mapKV[1]

		for _, res := range resources {
			if strings.ToLower(res.Kind) != strings.ToLower(kind) {
				continue
			}

			val := walk(res.Data, strings.Split(attrPath, "."))
			m, ok := val.(map[string]interface{})
			if !ok {
				continue
			}
			foundVal, ok := m[mapKey]
			if !ok || fmt.Sprintf("%v", foundVal) != mapVal {
				continue
			}

			return walk(res.Data, strings.Split(right, "."))
		}
		return "<no match>"

	default:
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))
	}
}

// walk safely walks a path with map/array support
func walk(data interface{}, path []string) interface{} {
	target := data
	for _, p := range path {
		switch node := target.(type) {
		case map[string]interface{}:
			v, ok := node[p]
			if !ok {
				return fmt.Sprintf("<missing: %s>", p)
			}
			target = v
		case []interface{}:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return fmt.Sprintf("<invalid index: %s>", p)
			}
			target = node[idx]
		default:
			return fmt.Sprintf("<unexpected node at: %s>", p)
		}
	}
	return target
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

	rulesRaw, _ := os.ReadFile("rules.yaml")
	rulesTmpl, _ := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
	var renderedRules bytes.Buffer
	rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars})
	var rules RulesFile
	yaml.Unmarshal(renderedRules.Bytes(), &rules)

	stdin, _ := io.ReadAll(os.Stdin)
	docs := strings.Split(string(stdin), "---")
	resourceGraph := []Resource{}

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

	for i := range resourceGraph {
		for _, rule := range rules.Rules {
			if matches(resourceGraph[i].Data, rule.Match) {
				if rule.Inject != nil {
					resourceGraph[i].Data = mergeMaps(resourceGraph[i].Data, rule.Inject)
				}
				if rule.InjectFile != "" {
					fileData, _ := os.ReadFile(rule.InjectFile)

					injectTmpl, _ := template.New("injectFile").
						Funcs(sprig.TxtFuncMap()).
						Funcs(template.FuncMap{"resource": func(input string) interface{} {
							return resolveResource(resourceGraph, input)
						}}).Parse(string(fileData))

					var renderedInject bytes.Buffer
					injectTmpl.Execute(&renderedInject, map[string]interface{}{"var": vars})

					var fileMap map[string]interface{}
					yaml.Unmarshal(renderedInject.Bytes(), &fileMap)

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

	for _, res := range resourceGraph {
		out, _ := yaml.Marshal(res.Data)
		fmt.Printf("---\n%s", out)
	}
}
