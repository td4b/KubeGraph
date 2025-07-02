package models

type Resource struct {
	Kind string
	Data map[string]interface{}
}

type Rule struct {
	Match        map[string]interface{} `yaml:"match"`
	Inject       map[string]interface{} `yaml:"inject"`
	Patches      string                 `yaml:"patches"`
	NewResources []string               `yaml:"newResources"`
}

type RulesFile struct {
	Rules []Rule `yaml:"rules"`
}
