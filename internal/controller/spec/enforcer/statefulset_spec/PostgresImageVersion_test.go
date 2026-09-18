package statefulset_spec

import "testing"

func TestPostgresMajorVersion(t *testing.T) {
	tests := map[string]int{
		"postgres:16.4":                  16,
		"postgres:16.4-alpine":           16,
		"registry.example/postgres:17.2": 17,
	}
	for image, want := range tests {
		got, err := PostgresMajorVersion(image)
		if err != nil || got != want {
			t.Fatalf("PostgresMajorVersion(%q) = %d, %v; want %d", image, got, err, want)
		}
	}
}

func TestPostgresMajorVersionRejectsAmbiguousImages(t *testing.T) {
	for _, image := range []string{"postgres:latest", "postgres", "postgres@sha256:abc"} {
		if _, err := PostgresMajorVersion(image); err == nil {
			t.Errorf("PostgresMajorVersion(%q) unexpectedly accepted image", image)
		}
	}
}

func TestIsPostgresMinorUpgrade(t *testing.T) {
	minor, err := IsPostgresMinorUpgrade("postgres:16.3", "postgres:16.4")
	if err != nil || !minor {
		t.Fatalf("minor upgrade = %v, %v; want true, nil", minor, err)
	}

	minor, err = IsPostgresMinorUpgrade("postgres:16.4", "postgres:16.4")
	if err != nil || minor {
		t.Fatalf("same image = %v, %v; want false, nil", minor, err)
	}

	if _, err := IsPostgresMinorUpgrade("postgres:16.4", "postgres:17.0"); err == nil {
		t.Fatal("major upgrade unexpectedly accepted")
	}
}
