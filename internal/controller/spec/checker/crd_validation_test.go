package checker

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

// The SpecChecker validates cron tasks and related fields at reconcile time.
// These tests prove the same constraints are declared in the generated CRD so
// invalid objects are rejected at admission instead of failing later.
func loadKubegresSpecSchema(t *testing.T) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "config", "crd", "bases", "kubegres.reactive-tech.io_kubegres.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var crd map[string]any
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	version := crd["spec"].(map[string]any)["versions"].([]any)[0].(map[string]any)
	return version["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)["properties"].(map[string]any)["spec"].(map[string]any)
}

func props(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	p, ok := schema["properties"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("property %q not found", name)
	}
	return p
}

func requiredSet(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, r := range asSlice(schema["required"]) {
		out[r.(string)] = true
	}
	return out
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func TestCRDDeclaresCronTaskConstraints(t *testing.T) {
	spec := loadKubegresSpecSchema(t)
	cronTasks := props(t, spec, "cronTasks")
	if cronTasks["x-kubernetes-list-type"] != "map" || len(asSlice(cronTasks["x-kubernetes-list-map-keys"])) != 1 || asSlice(cronTasks["x-kubernetes-list-map-keys"])[0] != "name" {
		t.Errorf("cronTasks is not a map list keyed by name: %v / %v", cronTasks["x-kubernetes-list-type"], cronTasks["x-kubernetes-list-map-keys"])
	}
	item := cronTasks["items"].(map[string]any)
	required := requiredSet(item)
	for _, field := range []string{"name", "schedule", "image"} {
		if !required[field] {
			t.Errorf("cronTasks[].%s is not required in the CRD", field)
		}
	}
	name := props(t, item, "name")
	if name["pattern"] == nil || name["maxLength"] != float64(63) {
		t.Errorf("cronTasks[].name lacks DNS-1123 label constraints: pattern=%v maxLength=%v", name["pattern"], name["maxLength"])
	}
	policy := props(t, item, "concurrencyPolicy")
	if got := asSlice(policy["enum"]); len(got) != 3 {
		t.Errorf("cronTasks[].concurrencyPolicy enum = %v, want Allow/Forbid/Replace", got)
	}
	script := props(t, item, "script")
	scriptRequired := requiredSet(script)
	for _, field := range []string{"configMapName", "key", "mountPath"} {
		if !scriptRequired[field] {
			t.Errorf("cronTasks[].script.%s is not required in the CRD", field)
		}
	}
}

func TestCRDDeclaresPodDisruptionBudgetMinimum(t *testing.T) {
	spec := loadKubegresSpecSchema(t)
	minAvailable := props(t, props(t, spec, "podDisruptionBudget"), "minAvailable")
	if minAvailable["minimum"] != float64(0) {
		t.Errorf("podDisruptionBudget.minAvailable minimum = %v, want 0", minAvailable["minimum"])
	}
}

func TestCRDDeclaresBackupSizeAsQuantity(t *testing.T) {
	spec := loadKubegresSpecSchema(t)
	size := props(t, props(t, spec, "backup"), "size")
	if size["pattern"] == nil {
		t.Error("backup.size has no quantity pattern")
	}
}
