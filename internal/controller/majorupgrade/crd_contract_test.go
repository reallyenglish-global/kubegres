package majorupgrade

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestCRDAndSampleArePublished(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	crdPath := filepath.Join(root, "config", "crd", "bases", "kubegres.reactive-tech.io_majorupgrades.yaml")
	raw, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatal(err)
	}
	var crd map[string]any
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	if crd["kind"] != "CustomResourceDefinition" {
		t.Fatalf("unexpected CRD kind: %v", crd["kind"])
	}
	if _, err := os.Stat(filepath.Join(root, "config", "samples", "kubegres_v1_majorupgrade.yaml")); err != nil {
		t.Fatal(err)
	}
}
