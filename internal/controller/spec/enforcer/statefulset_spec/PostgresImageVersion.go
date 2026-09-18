package statefulset_spec

import (
	"fmt"
	"strconv"
	"strings"
)

// PostgresMajorVersion returns the major version from a PostgreSQL image tag.
// PostgreSQL images are expected to use a numeric tag, for example postgres:16.4
// or registry.example/postgres:16.4-alpine. Untagged and digest-only images are
// rejected because their upgrade intent cannot be proven safely.
func PostgresMajorVersion(image string) (int, error) {
	image = strings.TrimSpace(image)
	lastSlash := strings.LastIndexByte(image, '/')
	lastColon := strings.LastIndexByte(image, ':')
	if lastColon <= lastSlash || lastColon == len(image)-1 {
		return 0, fmt.Errorf("PostgreSQL image %q has no numeric version tag", image)
	}

	tag := strings.SplitN(image[lastColon+1:], "-", 2)[0]
	parts := strings.Split(tag, ".")
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("PostgreSQL image %q has no numeric major version", image)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 1 {
		return 0, fmt.Errorf("PostgreSQL image %q has invalid major version", image)
	}
	return major, nil
}

func IsPostgresMinorUpgrade(currentImage, desiredImage string) (bool, error) {
	currentMajor, err := PostgresMajorVersion(currentImage)
	if err != nil {
		return false, err
	}
	desiredMajor, err := PostgresMajorVersion(desiredImage)
	if err != nil {
		return false, err
	}
	if currentMajor != desiredMajor {
		return false, fmt.Errorf("PostgreSQL major version change from %d to %d is not supported; use a dump/restore or pg_upgrade", currentMajor, desiredMajor)
	}
	return currentImage != desiredImage, nil
}
