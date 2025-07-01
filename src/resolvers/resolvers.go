package resolvers

import (
	"KubeGraph/helpers"
	"KubeGraph/models"
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/sprig"
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
		yaml.Unmarshal(doc, &parsed)

		kindRaw := parsed["kind"]
		kindStr, ok := kindRaw.(string)
		if !ok {
			panic(fmt.Sprintf("Resource is missing kind! Raw: %#v", parsed))
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

				// ✅ 2️⃣ handle newResources (before InjectFile)
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

					var newParsed map[string]interface{}
					yaml.Unmarshal(buf.Bytes(), &newParsed)

					kindRaw := newParsed["kind"]
					kindStr, ok := kindRaw.(string)
					if !ok || kindStr == "" {
						panic(fmt.Sprintf("[ERROR] Rendered newResource is missing kind!\nYAML:\n%s", buf.String()))
					}

					resourceGraph = append(resourceGraph, models.Resource{
						Kind: kindStr,
						Data: newParsed,
					})
				}

				// ✅ 3️⃣ inject file AFTER newResources are in graph
				if rule.InjectFile != "" {
					injectFilePath := filepath.Join(rulesDir, rule.InjectFile)
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
					yaml.Unmarshal(renderedInject.Bytes(), &fileMap)

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
		yaml.Unmarshal(buf.Bytes(), &parsed)

		resourceGraph = append(resourceGraph, models.Resource{
			Kind: parsed["kind"].(string),
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
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))
	}

	left := parts[0]
	right := parts[1]

	leftParts := strings.Split(left, ".")

	// Filter: must be at least kind.X.attr1.attr2...key.value
	if len(leftParts) < 4 {
		panic(fmt.Sprintf("Invalid left selector: %s", left))
	}

	kind := leftParts[1]
	mapVal := leftParts[len(leftParts)-1]
	mapKey := leftParts[len(leftParts)-2]
	attrPath := strings.Join(leftParts[2:len(leftParts)-2], ".")

	for _, res := range resources {
		if !strings.EqualFold(res.Kind, kind) {
			continue
		}

		val := helpers.Walk(res.Data, strings.Split(attrPath, "."))

		switch m := val.(type) {
		case map[string]interface{}:
			if v, ok := m[mapKey]; ok && fmt.Sprintf("%v", v) == mapVal {
				return helpers.Walk(res.Data, strings.Split(right, "."))
			}
		case map[interface{}]interface{}:
			for k, v := range m {
				if fmt.Sprintf("%v", k) == mapKey && fmt.Sprintf("%v", v) == mapVal {
					return helpers.Walk(res.Data, strings.Split(right, "."))
				}
			}
		}
	}

	return "<no match>"
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
