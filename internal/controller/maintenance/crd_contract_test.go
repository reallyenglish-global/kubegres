package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

const crdFileName = "kubegres.reactive-tech.io_maintenanceoperations.yaml"

func configPath(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{"..", "..", "..", "config"}, parts...)...)
}

func TestMaintenanceOperationCRDIsInstalledByKustomize(t *testing.T) {
	raw, err := os.ReadFile(configPath(t, "crd", "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "bases/"+crdFileName) {
		t.Fatalf("config/crd/kustomization.yaml does not install %s", crdFileName)
	}
}

func TestMaintenanceOperationCRDEnforcesContract(t *testing.T) {
	raw, err := os.ReadFile(configPath(t, "crd", "bases", crdFileName))
	if err != nil {
		t.Fatal(err)
	}
	var crd map[string]any
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	spec := dig(t, crd, "spec", "versions").([]any)[0].(map[string]any)
	props := dig(t, spec, "schema", "openAPIV3Schema", "properties", "spec", "properties").(map[string]any)

	for _, field := range []string{"purpose", "expiresAt"} {
		if !hasImmutableRule(props[field]) {
			t.Errorf("spec.%s is not declared immutable in the CRD", field)
		}
	}
	targetNode := props["targetNode"].(map[string]any)["properties"].(map[string]any)
	if !hasImmutableRule(targetNode["uid"]) {
		t.Errorf("spec.targetNode.uid is not declared immutable in the CRD")
	}

	safety := props["safety"].(map[string]any)["properties"].(map[string]any)
	for field, want := range map[string]float64{"maxReplayLagSeconds": 5, "stableForSeconds": 60, "relocationDeadlineSeconds": 900} {
		got, _ := safety[field].(map[string]any)["default"].(float64)
		if got != want {
			t.Errorf("spec.safety.%s default = %v, want %v", field, got, want)
		}
	}
}

func hasImmutableRule(schema any) bool {
	m, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	rules, _ := m["x-kubernetes-validations"].([]any)
	for _, r := range rules {
		if rule, _ := r.(map[string]any)["rule"].(string); rule == "self == oldSelf" {
			return true
		}
	}
	return false
}

func dig(t *testing.T, node any, keys ...string) any {
	t.Helper()
	for _, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("expected map at %q", k)
		}
		node = m[k]
	}
	return node
}

func TestMaintenanceOperationSampleIsPublished(t *testing.T) {
	const sample = "kubegres_v1_maintenanceoperation.yaml"
	kustomization, err := os.ReadFile(configPath(t, "samples", "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kustomization), sample) {
		t.Fatalf("config/samples/kustomization.yaml does not list %s", sample)
	}
	raw, err := os.ReadFile(configPath(t, "samples", sample))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := yaml.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["kind"] != "MaintenanceOperation" {
		t.Fatalf("sample kind = %v", obj["kind"])
	}
	spec := obj["spec"].(map[string]any)
	if dig(t, spec, "targetNode", "uid") == nil || spec["expiresAt"] == nil || spec["purpose"] == nil || dig(t, spec, "kubegresRef", "name") == nil {
		t.Fatalf("sample is missing required spec fields: %v", spec)
	}
}

func TestMaintenanceOperationAdminRolesAreInstalled(t *testing.T) {
	kustomization, err := os.ReadFile(configPath(t, "rbac", "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"maintenanceoperation_editor_role.yaml", "maintenanceoperation_viewer_role.yaml"} {
		if !strings.Contains(string(kustomization), role) {
			t.Errorf("config/rbac/kustomization.yaml does not list %s", role)
		}
		raw, err := os.ReadFile(configPath(t, "rbac", role))
		if err != nil {
			t.Errorf("%s: %v", role, err)
			continue
		}
		if !strings.Contains(string(raw), "- maintenanceoperations\n") {
			t.Errorf("%s does not grant access to maintenanceoperations", role)
		}
	}
}
