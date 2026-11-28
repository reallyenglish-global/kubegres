package kindcluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Neither endpoint exists. Even the ambient kubeconfig only contains fixtures.
const fixture = `apiVersion: v1
kind: Config
current-context: unrelated
contexts:
- name: unrelated
  context: {cluster: other, user: other}
- name: kind-kubegres
  context: {cluster: kind-kubegres, user: kind-kubegres}
clusters:
- name: other
  cluster: {server: 'https://other.invalid'}
- name: kind-kubegres
  cluster: {server: 'https://suite.invalid'}
users:
- name: other
  user: {token: unrelated-token}
- name: kind-kubegres
  user: {token: suite-token}
`

func TestNamedBinding(t *testing.T) {
	dir := t.TempDir()
	ambient := filepath.Join(dir, "ambient")
	// Selecting the right context from the wrong file must also fail the test.
	stale := strings.ReplaceAll(fixture, "suite.invalid", "stale.invalid")
	if err := os.WriteFile(ambient, []byte(stale), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", ambient)
	// Only this fake executable is reachable, even on a developer workstation.
	stub := `#!/bin/sh
case "$*" in
  'get clusters') printf 'kubegres\n';;
  'get kubeconfig --name kubegres') printf '%s' "$EXPORT"; [ "$EXPORT" != failed ];;
  *) exit 90;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "kind"), []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, tc := range []struct {
		name, data string
		ok         bool
	}{
		{"unrelated current", fixture, true},
		{"missing export", "", false},
		{"failed export", "failed", false},
		{"invalid export", "[", false},
		{"wrong context", strings.ReplaceAll(fixture, "kind-kubegres", "kind-elsewhere"), false},
		{"wrong cluster", strings.Replace(fixture, "cluster: kind-kubegres", "cluster: other", 1), false},
		{"wrong credentials", strings.Replace(fixture, "user: kind-kubegres", "user: other", 1), false},
		{"missing endpoint", strings.Replace(fixture, "https://suite.invalid", "", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EXPORT", tc.data)
			path := filepath.Join(t.TempDir(), "kubeconfig")
			env, err := PrepareEnvironment(path, "crds")
			if !tc.ok {
				if err == nil || env != nil {
					t.Fatalf("accepted invalid binding")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("unexpected diagnostics config")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if env.Config == nil || env.Config.Host != "https://suite.invalid" || env.Config.BearerToken != "suite-token" {
				t.Fatal("selected unrelated endpoint or identity")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tc.data {
				t.Fatal("diagnostics missing the named export")
			}
		})
	}
}
