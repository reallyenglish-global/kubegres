package util

import (
	"strings"
	"testing"

	"reactive-tech.io/kubegres/internal/test/resourceConfigs"
)

func TestCloseWithoutConnectionDoesNotPanic(t *testing.T) {
	dbUtil := DbConnectionDbUtil{LogLabel: "Primary test"}

	dbUtil.Close()
}

func TestConnectionDescriptionOmitsPassword(t *testing.T) {
	dbUtil := DbConnectionDbUtil{Port: 30001}

	description := dbUtil.connectionDescription()
	if strings.Contains(description, resourceConfigs.DbPassword) {
		t.Fatalf("connection description %q contains the DB password", description)
	}
	if !strings.Contains(description, "30001") {
		t.Fatalf("connection description %q does not identify the port", description)
	}
}
