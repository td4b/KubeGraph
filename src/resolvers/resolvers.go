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

	docs := strings.Split(string(input), "---")
	resourceGraph := []models.Resource{}

	// Phase 1: Just parse raw YAML docs
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var parsed map[string]interface{}
		yaml.Unmarshal([]byte(doc), &parsed)

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

	for i := range resourceGraph {
		for _, rule := range rules.Rules {
			if helpers.Matches(resourceGraph[i].Data, rule.Match) {
				// If you want to inject inline blocks
				if rule.Inject != nil {
					resourceGraph[i].Data = helpers.MergeMaps(resourceGraph[i].Data, rule.Inject)
				}

				// If you want to inject from a file
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

				// Handle new resources
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
						panic(fmt.Sprintf(
							"[ERROR] Rendered newResource is missing kind!\nFinal YAML:\n%s",
							buf.String(),
						))
					}

					resourceGraph = append(resourceGraph, models.Resource{
						Kind: kindStr,
						Data: newParsed,
					})
				}
			}
		}
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

	switch len(parts) {
	case 1:
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))

	case 2:
		left := parts[0]
		right := parts[1]

		leftParts := strings.Split(strings.TrimSpace(left), ".")
		if len(leftParts) == 2 {
			kind := leftParts[1]
			for _, res := range resources {
				if strings.ToLower(res.Kind) != strings.ToLower(kind) {
					continue
				}
				return helpers.Walk(res.Data, strings.Split(right, "."))
			}
			return "<no match>"
		}

		if len(leftParts) < 4 {
			panic(fmt.Sprintf("Invalid left selector: %s", left))
		}

		kind := leftParts[1]
		attrPath := strings.Join(leftParts[2:len(leftParts)-1], ".")
		attrVal := strings.TrimSpace(leftParts[len(leftParts)-1])

		for _, res := range resources {
			if strings.ToLower(res.Kind) != strings.ToLower(kind) {
				continue
			}

			val := helpers.Walk(res.Data, strings.Split(attrPath, "."))
			if fmt.Sprintf("%v", val) == attrVal {
				out := helpers.Walk(res.Data, strings.Split(right, "."))
				return out
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

			val := helpers.Walk(res.Data, strings.Split(attrPath, "."))

			m, ok := val.(map[string]interface{})
			if !ok {
				continue
			}
			foundVal, ok := m[mapKey]
			if !ok {
				continue
			}

			if fmt.Sprintf("%v", foundVal) != mapVal {
				continue
			}

			return helpers.Walk(res.Data, strings.Split(right, "."))
		}
		return "<no match>"

	default:
		panic(fmt.Sprintf("Invalid resource syntax: %s", input))
	}
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
