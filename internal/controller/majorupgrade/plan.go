package majorupgrade

import (
	"fmt"
	"regexp"
	"strconv"

	kubegresv1 "reactive-tech.io/kubegres/api/v1"
)

var taggedImage = regexp.MustCompile(`:(\d+)(?:\.\d+){0,2}$`)

// Validate rejects unsafe shortcuts before a workflow can touch either cluster.
// Major upgrades require immutable source evidence, a different major version,
// and explicit user-owned hooks for every irreversible phase.
func Validate(spec kubegresv1.MajorUpgradeSpec) error {
	if spec.SourceKubegres == "" || spec.SourceImage == "" || spec.TargetImage == "" {
		return fmt.Errorf("sourceKubegres, sourceImage, and targetImage are required")
	}
	if spec.SourceImage == spec.TargetImage {
		return fmt.Errorf("sourceImage and targetImage must differ")
	}
	sourceMajor, err := major(spec.SourceImage)
	if err != nil {
		return fmt.Errorf("sourceImage: %w", err)
	}
	targetMajor, err := major(spec.TargetImage)
	if err != nil {
		return fmt.Errorf("targetImage: %w", err)
	}
	if sourceMajor == targetMajor {
		return fmt.Errorf("targetImage must use a different PostgreSQL major version")
	}
	if spec.Strategy != kubegresv1.MajorUpgradeLogicalReplication && spec.Strategy != kubegresv1.MajorUpgradeDumpRestore {
		return fmt.Errorf("unsupported strategy %q", spec.Strategy)
	}
	for name, hook := range map[string]kubegresv1.MajorUpgradeHook{
		"preflight": spec.Preflight, "bootstrap": spec.Bootstrap, "copy": spec.Copy,
		"validate": spec.Validate, "cutover": spec.Cutover,
	} {
		if err := validateHook(name, hook); err != nil {
			return err
		}
	}
	if spec.Cleanup != nil {
		if err := validateHook("cleanup", *spec.Cleanup); err != nil {
			return err
		}
	}
	return nil
}

func validateHook(name string, hook kubegresv1.MajorUpgradeHook) error {
	if hook.Image == "" {
		return fmt.Errorf("%s hook image is required", name)
	}
	if len(hook.Command) == 0 && hook.Script == nil {
		return fmt.Errorf("%s hook must define command or script", name)
	}
	if hook.Script != nil && (hook.Script.ConfigMapName == "" || hook.Script.Key == "" || hook.Script.MountPath == "") {
		return fmt.Errorf("%s hook script requires configMapName, key, and mountPath", name)
	}
	return nil
}

func major(image string) (int, error) {
	match := taggedImage.FindStringSubmatch(image)
	if len(match) != 2 {
		return 0, fmt.Errorf("must end in a numeric PostgreSQL tag (digest-only and floating tags are unsupported)")
	}
	version, err := strconv.Atoi(match[1])
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid PostgreSQL major version")
	}
	return version, nil
}

// HookNames is the stable execution order for an external workflow runner.
// The controller may observe and persist these phases without owning the
// deployment-specific migration commands.
func HookNames(spec kubegresv1.MajorUpgradeSpec) []string {
	names := []string{"preflight", "bootstrap", "copy", "validate", "cutover"}
	if spec.Cleanup != nil {
		names = append(names, "cleanup")
	}
	return names
}
