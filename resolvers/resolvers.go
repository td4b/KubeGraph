package resolvers

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/td4b/KubeGraph/helpers"
	"github.com/td4b/KubeGraph/models"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v2"
)

func BuildGraph(input []byte, rulesDir string, rules models.RulesFile, vars map[string]interface{}) []models.Resource {
	docs := bytes.Split(input, []byte("---"))
	resourceGraph := []models.Resource{}

	// Phase 1: parse base docs
	for _, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		var parsed map[string]interface{}
		if err := yaml.Unmarshal(doc, &parsed); err != nil {
			panic(fmt.Sprintf("[ERROR] Failed to parse base doc YAML:\n%s\nErr: %v", doc, err))
		}

		kindRaw := parsed["kind"]
		kindStr, ok := kindRaw.(string)
		if !ok || kindStr == "" {
			panic(fmt.Sprintf("[ERROR] Resource is missing kind! Raw: %#v", parsed))
		}

		resourceGraph = append(resourceGraph, models.Resource{
			Kind: kindStr,
			Data: parsed,
		})
	}

	// Phase 2: match and mutate graph until stable
	i := 0
	for i < len(resourceGraph) {
		for _, rule := range rules.Rules {
			if helpers.Matches(resourceGraph[i].Data, rule.Match) {
				// ✅ 1️⃣ inject inline
				if rule.Inject != nil {
					resourceGraph[i].Data = helpers.MergeMaps(resourceGraph[i].Data, rule.Inject)
				}

				// ✅ 2️⃣ handle newResources — supports multiple docs
				for _, newPath := range rule.NewResources {
					newResourcePath := filepath.Join(rulesDir, newPath)
					rawNew, _ := os.ReadFile(newResourcePath)
					newTmpl, _ := template.New(newPath).
						Funcs(sprig.TxtFuncMap()).
						Funcs(template.FuncMap{
							"resource": func(input string) interface{} {
								return ResolveResource(resourceGraph, input)
							},
						}).Parse(string(rawNew))

					var buf bytes.Buffer
					newTmpl.Execute(&buf, map[string]interface{}{"var": vars})

					// Split into multiple YAML docs if needed
					docs := bytes.Split(buf.Bytes(), []byte("---"))
					for _, doc := range docs {
						doc = bytes.TrimSpace(doc)
						if len(doc) == 0 {
							continue
						}

						var newParsed map[string]interface{}
						if err := yaml.Unmarshal(doc, &newParsed); err != nil {
							panic(fmt.Sprintf("[ERROR] Failed to parse newResource YAML:\n%s\nErr: %v", doc, err))
						}

						kindRaw := newParsed["kind"]
						kindStr, ok := kindRaw.(string)
						if !ok || kindStr == "" {
							panic(fmt.Sprintf("[ERROR] Rendered newResource is missing kind!\nYAML:\n%s", doc))
						}

						resourceGraph = append(resourceGraph, models.Resource{
							Kind: kindStr,
							Data: newParsed,
						})
					}
				}

				// ✅ 3️⃣ inject file AFTER newResources
				if rule.Patches != "" {
					injectFilePath := filepath.Join(rulesDir, rule.Patches)
					fileData, _ := os.ReadFile(injectFilePath)

					injectTmpl, _ := template.New("injectFile").
						Funcs(sprig.TxtFuncMap()).
						Funcs(template.FuncMap{
							"resource": func(input string) interface{} {
								return ResolveResource(resourceGraph, input)
							},
						}).Parse(string(fileData))

					var renderedInject bytes.Buffer
					injectTmpl.Execute(&renderedInject, map[string]interface{}{"var": vars})

					var fileMap map[string]interface{}
					if err := yaml.Unmarshal(renderedInject.Bytes(), &fileMap); err != nil {
						panic(fmt.Sprintf("[ERROR] Failed to parse InjectFile YAML:\n%s\nErr: %v", renderedInject.String(), err))
					}

					resourceGraph[i].Data = helpers.MergeMaps(resourceGraph[i].Data, fileMap)
				}
			}
		}
		i++
	}

	return resourceGraph
}

func HandleInputs(docs []string, resourceGraph []models.Resource, vars map[string]interface{}) []models.Resource {
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		tmpl, _ := template.New("doc").Funcs(sprig.TxtFuncMap()).
			Funcs(template.FuncMap{"resource": func(input string) interface{} {
				return ResolveResource(resourceGraph, input)
			}}).Parse(doc)
		var buf bytes.Buffer
		tmpl.Execute(&buf, map[string]interface{}{"var": vars})

		var parsed map[string]interface{}
		if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
			panic(fmt.Sprintf("[ERROR] Failed to parse input YAML:\n%s\nErr: %v", buf.String(), err))
		}

		kindRaw := parsed["kind"]
		kindStr, ok := kindRaw.(string)
		if !ok || kindStr == "" {
			panic(fmt.Sprintf("[ERROR] Input YAML is missing kind:\n%s", buf.String()))
		}

		resourceGraph = append(resourceGraph, models.Resource{
			Kind: kindStr,
			Data: parsed,
		})
	}
	return resourceGraph
}

func ResolveResource(resources []models.Resource, input string) interface{} {
	parts := strings.Split(input, "&")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "ERROR: Invalid resource syntax: %s\n", input)
		os.Exit(1)
	}

	left := parts[0]
	right := parts[1]

	leftParts := strings.Split(left, ".")
	if len(leftParts) < 4 {
		fmt.Fprintf(os.Stderr, "ERROR: Invalid left selector: %s\n", left)
		os.Exit(1)
	}

	kind := leftParts[1]
	mapVal := leftParts[len(leftParts)-1]
	mapKey := leftParts[len(leftParts)-2]
	attrPath := strings.Join(leftParts[2:len(leftParts)-2], ".")

	foundKind := false

	for _, res := range resources {
		if !strings.EqualFold(res.Kind, kind) {
			continue
		}
		foundKind = true

		val := helpers.Walk(res.Data, strings.Split(attrPath, "."))

		switch m := val.(type) {
		case map[string]interface{}:
			if v, ok := m[mapKey]; ok {
				if fmt.Sprintf("%v", v) == mapVal {
					return helpers.Walk(res.Data, strings.Split(right, "."))
				}
			}
		case map[interface{}]interface{}:
			for k, v := range m {
				if fmt.Sprintf("%v", k) == mapKey && fmt.Sprintf("%v", v) == mapVal {
					return helpers.Walk(res.Data, strings.Split(right, "."))
				}
			}
		}
	}

	// Only print debug output if we failed to resolve
	fmt.Fprintf(os.Stderr, "\n[ResolveResource] Failed to resolve input: %s\n", input)
	fmt.Fprintf(os.Stderr, "Kind:      %s\n", kind)
	fmt.Fprintf(os.Stderr, "Attr Path: %s\n", attrPath)
	fmt.Fprintf(os.Stderr, "Map Key:   %s\n", mapKey)
	fmt.Fprintf(os.Stderr, "Map Val:   %s\n", mapVal)
	fmt.Fprintf(os.Stderr, "Right:     %s\n", right)

	if !foundKind {
		fmt.Fprintf(os.Stderr, "No resources found with kind: %s\n", kind)
	} else {
		fmt.Fprintf(os.Stderr, "Resources with matching kind found but filter did not match:\n")
		for _, res := range resources {
			if !strings.EqualFold(res.Kind, kind) {
				continue
			}

			val := helpers.Walk(res.Data, strings.Split(attrPath, "."))
			fmt.Fprintf(os.Stderr, "  Resource: kind=%s\n", res.Kind)
			fmt.Fprintf(os.Stderr, "  Walked Path Value: %#v\n", val)

			switch m := val.(type) {
			case map[string]interface{}:
				fmt.Fprintf(os.Stderr, "  Keys:\n")
				for k, v := range m {
					fmt.Fprintf(os.Stderr, "    %s: %v\n", k, v)
				}
			case map[interface{}]interface{}:
				fmt.Fprintf(os.Stderr, "  Keys:\n")
				for k, v := range m {
					fmt.Fprintf(os.Stderr, "    %v: %v\n", k, v)
				}
			default:
				fmt.Fprintf(os.Stderr, "  Non-map type at path: %#v\n", val)
			}
		}
	}

	os.Exit(1)
	return nil // unreachable
}

func LoadInputFromPath(path string) []byte {
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
