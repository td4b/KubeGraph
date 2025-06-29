# 🚀 KubeGraph

**KubeGraph** is a simple **Terraform-style graph-aware YAML templating tool** for Kubernetes.  
It renders Kubernetes YAML from templates, supporting:
- `{{ var.* }}` style variables
- `{{ resource "kind.Deployment.metadata.name.myapp & spec.selector.matchLabels" }}` style cross-resource lookups
- Smart graph linking like Terraform’s `depends_on`
- Dry-run planning (`kubectl apply --dry-run=server`)
- Integration as an ArgoCD **Custom CMP**

---

## ✨ Features

✅ Render YAML templates with Go text/template + `sprig` functions  
✅ Reference other resources by kind/name/fields  
✅ Use `rules.yaml` to:
  - Match resources
  - Inject new fields
  - Merge `inject-file.yaml` patches
  - Attach **new** resource templates dynamically (like Services, ConfigMaps)

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

**1️⃣ Base input: `deployment.yaml`**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: meow
  annotations:
    team: boo
spec:
  selector:
    matchLabels:
      app: nginx
