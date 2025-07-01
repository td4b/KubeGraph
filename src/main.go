package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
			kind := leftParts[1]
			for _, res := range resources {
				if strings.ToLower(res.Kind) != strings.ToLower(kind) {
					continue
				}
				return walk(res.Data, strings.Split(right, "."))
			}
			return "<no match>"
		}

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
	// New: accept --rules and --input
	rulesPath := flag.String("rules", "", "Path to the rules.yaml file (required)")
	inputPath := flag.String("input", "", "Input file or directory if stdin is empty")
	flag.Parse()

	if *rulesPath == "" {
		fmt.Println("Error: --rules is required")
		os.Exit(1)
	}

	// The base dir for relative injects
	rulesDir := filepath.Dir(*rulesPath)

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

	// Load and render rules
	rulesRaw, _ := os.ReadFile(*rulesPath)
	rulesTmpl, _ := template.New("rules").Funcs(sprig.TxtFuncMap()).Parse(string(rulesRaw))
	var renderedRules bytes.Buffer
	rulesTmpl.Execute(&renderedRules, map[string]interface{}{"var": vars})
	var rules RulesFile
	yaml.Unmarshal(renderedRules.Bytes(), &rules)

	// Load stdin or fallback
	stat, _ := os.Stdin.Stat()
	var stdin []byte
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		stdin, _ = io.ReadAll(os.Stdin)
	} else if *inputPath != "" {
		stdin = loadInputFromPath(*inputPath)
	} else {
		stdin = loadInputFromPath(".")
	}

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
					injectFilePath := filepath.Join(rulesDir, rule.InjectFile)
					fileData, _ := os.ReadFile(injectFilePath)

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
					newResourcePath := filepath.Join(rulesDir, newPath)
					rawNew, err := os.ReadFile(newResourcePath)
					if err != nil {
						panic(fmt.Sprintf("Failed to read newResource file: %s", newResourcePath))
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
