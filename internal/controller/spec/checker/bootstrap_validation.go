package checker

import (
	"fmt"
	"strings"

	postgresV1 "reactive-tech.io/kubegres/api/v1"
)

func (r *SpecChecker) bootstrapSpecError(spec *postgresV1.KubegresSpec) string {
	if spec.Bootstrap == nil {
		return ""
	}
	if spec.ContainerSecurityContext == nil || spec.ContainerSecurityContext.RunAsNonRoot == nil || !*spec.ContainerSecurityContext.RunAsNonRoot || spec.ContainerSecurityContext.RunAsUser == nil {
		return "'spec.containerSecurityContext.runAsNonRoot' and 'runAsUser' are required for logical bootstrap; the bootstrap image must initialize a writable PVC as a non-root PostgreSQL user."
	}
	if spec.Replicas == nil || *spec.Replicas != 1 {
		return "'spec.replicas' must be 1 for logical bootstrap; replication/failover is outside this one-time import contract."
	}
	if spec.Bootstrap.Logical == nil {
		return "'spec.bootstrap.logical' is required; no other bootstrap method is supported."
	}
	logical := spec.Bootstrap.Logical
	source := logical.Source
	for field, value := range map[string]string{
		"spec.bootstrap.logical.source.host":                      source.Host,
		"spec.bootstrap.logical.source.database":                  source.Database,
		"spec.bootstrap.logical.source.username":                  source.Username,
		"spec.bootstrap.logical.database":                         logical.Database,
		"spec.bootstrap.logical.source.passwordSecretKeyRef.name": source.PasswordSecretKeyRef.Name,
		"spec.bootstrap.logical.source.passwordSecretKeyRef.key":  source.PasswordSecretKeyRef.Key,
		"spec.bootstrap.logical.source.tls.caSecretKeyRef.name":   source.TLS.CASecretKeyRef.Name,
		"spec.bootstrap.logical.source.tls.caSecretKeyRef.key":    source.TLS.CASecretKeyRef.Key,
	} {
		if value == "" {
			return "'" + field + "' must be set when logical bootstrap is configured."
		}
	}
	if source.Port < 0 || source.Port > 65535 {
		return "'spec.bootstrap.logical.source.port' must be between 1 and 65535 when set."
	}
	if logical.TimeoutSeconds < 60 || logical.TimeoutSeconds > 86400 {
		return "'spec.bootstrap.logical.timeoutSeconds' must be between 60 and 86400 seconds."
	}
	for field, value := range map[string]string{
		"spec.bootstrap.logical.database":        logical.Database,
		"spec.bootstrap.logical.source.database": source.Database,
		"spec.bootstrap.logical.source.username": source.Username,
	} {
		if !isBootstrapIdentifier(value) {
			return "'" + field + "' must contain only letters, numbers, and underscores, and start with a letter or underscore."
		}
	}
	if logical.Database == "postgres" || logical.Database == "template0" || logical.Database == "template1" {
		return fmt.Sprintf("'spec.bootstrap.logical.database' cannot be the system database %q.", logical.Database)
	}
	if spec.CustomConfig != "" && spec.CustomConfig != "base-kubegres-config" {
		return "'spec.customConfig' cannot be used with logical bootstrap; custom primary initialization scripts are not ordered with the import."
	}
	for _, env := range spec.Env {
		if env.Name == "POSTGRES_USER" || env.Name == "POSTGRES_DB" || env.Name == "PGDATA" || strings.HasPrefix(env.Name, "BOOTSTRAP_") {
			return "'spec.env." + env.Name + "' cannot be overridden when logical bootstrap is configured."
		}
	}
	for _, name := range []string{"POSTGRES_PASSWORD", "POSTGRES_REPLICATION_PASSWORD"} {
		count := 0
		for _, env := range spec.Env {
			if env.Name == name {
				count++
			}
		}
		if count == 0 {
			return "'spec.env." + name + "' is required for logical bootstrap."
		}
		if count > 1 {
			return "'spec.env." + name + "' must not be duplicated."
		}
	}
	return ""
}

func isBootstrapIdentifier(value string) bool {
	if value == "" || strings.ContainsAny(value, "-.'\"\\") {
		return false
	}
	for i, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || character == '_' || (i > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}
