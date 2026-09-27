package majorupgrade

import (
	"testing"

	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

func validSpec() kubegresv1.MajorUpgradeSpec {
	hook := func() kubegresv1.MajorUpgradeHook {
		return kubegresv1.MajorUpgradeHook{Image: "postgres:16", Command: []string{"/run/step"}}
	}
	return kubegresv1.MajorUpgradeSpec{
		SourceKubegres: "app", SourceImage: "postgres:15", TargetImage: "postgres:16",
		Strategy: kubegresv1.MajorUpgradeLogicalReplication,
		Preflight: hook(), Bootstrap: hook(), Copy: hook(), Validate: hook(), Cutover: hook(),
	}
}

func TestValidateRequiresMajorChangeAndHooks(t *testing.T) {
	if err := Validate(validSpec()); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	cases := []struct {
		name string
		edit func(*kubegresv1.MajorUpgradeSpec)
	}{
		{"same major", func(s *kubegresv1.MajorUpgradeSpec) { s.TargetImage = "postgres:15.7" }},
		{"floating source", func(s *kubegresv1.MajorUpgradeSpec) { s.SourceImage = "postgres:latest" }},
		{"missing cutover hook", func(s *kubegresv1.MajorUpgradeSpec) { s.Cutover = kubegresv1.MajorUpgradeHook{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.edit(&spec)
			if err := Validate(spec); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHookNamesIncludesOptionalCleanup(t *testing.T) {
	spec := validSpec()
	if got := HookNames(spec); len(got) != 5 || got[0] != "preflight" || got[4] != "cutover" {
		t.Fatalf("unexpected hook order: %v", got)
	}
	spec.Cleanup = &kubegresv1.MajorUpgradeHook{Image: "postgres:16", Command: []string{"/cleanup"}}
	got := HookNames(spec)
	if got[len(got)-1] != "cleanup" {
		t.Fatalf("cleanup was not appended: %v", got)
	}
}
