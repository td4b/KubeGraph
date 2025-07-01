# 🚀 KubeGraph

**KubeGraph** is a simple **Terraform-style graph-aware YAML templating tool** for Kubernetes.  
It renders Kubernetes YAML from templates, supporting:
- `{{ var.* }}` style variables for direct template injection
- `{{ resource "kind.Deployment.metadata.name.myapp & spec.selector.matchLabels" }}` style cross-resource lookups
- `"{{ resource "kind.Ingress.metadata.annotations.kubegraph.managed & metadata.name" }}"` annotation value lookups
- Integration as an ArgoCD **Custom CMP**

---

## ✨ Features

✅ Render YAML templates with Go text/template + `sprig` functions  
✅ Reference other resources by kind/name/fields  
✅ Use `rules.yaml` to:
  - Match resources
  - Inject new fields
  - Merge `inject-file.yaml` patches
  - Attach **new** resource templates dynamically (like Services, ConfigMaps, IngressRoutes)

✅ Output ready to pipe to `kubectl` or ArgoCD

---

## 📂 Structure

- **`values.yaml`**  
  Key-value pairs for `{{ .var.* }}` in templates.

- **`rules.yaml`**  
  Define **resource match rules**, `inject` blocks, `injectFile`, and `newResources`.

- **Base YAML input**  
  Read from stdin or files. Should include Deployments or any base objects.

- **New resource templates**  
  Placed in your repo, e.g. `./render/service.yaml`.

---

## 🏗️ Example

If you run `go test` you should be able to see how the examples work. 

For references:
https://github.com/td4b/KubeGraph/tree/main/ArgoCD/SampleApp

Example usage would be something like:

```bash
cd ./src
kustomize build ../ArgoCD/SampleApp/tests/. | go run main.go --rules ../ArgoCD/SampleApp/rules.yaml
```
