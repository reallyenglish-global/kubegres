package template

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogicalBootstrapScriptHappyPathAndCommandFailure(t *testing.T) {
	tests := []struct {
		name       string
		fail       string
		wantErr    string
		wantMarker bool
	}{
		{name: "happy path", wantMarker: true},
		{name: "pg dump failure is deterministic", fail: "pg_dump", wantErr: "bootstrap failed: source export failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			fake := `#!/bin/sh
set -eu
case "$0" in
*/timeout) shift 2; exec "$@";;
*/pg_dump) [ "${FAIL_CMD:-}" = pg_dump ] && exit 42; : > "$(printf '%s\n' "$@" | sed -n 's/^--file=//p')";;
*/initdb) while [ "$#" -gt 0 ]; do [ "$1" = -D ] && d=$2; shift; done; mkdir -p "$d"; : > "$d/PG_VERSION";;
*/pg_ctl|*/psql|*/pg_restore) exit 0;;
esac
`
			for _, name := range []string{"timeout", "pg_dump", "initdb", "pg_ctl", "psql", "pg_restore"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(fake), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			parent := filepath.Join(root, "volume")
			if err := os.Mkdir(parent, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", "-ceu", logicalBootstrapScript)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"), "PGDATA="+filepath.Join(parent, "pgdata"),
				"POSTGRES_PASSWORD=dest", "POSTGRES_REPLICATION_PASSWORD=repl", "POSTGRES_USER=postgres",
				"BOOTSTRAP_SOURCE_PASSWORD=src", "BOOTSTRAP_SOURCE_HOST=source", "BOOTSTRAP_SOURCE_PORT=5432",
				"BOOTSTRAP_SOURCE_DATABASE=source_db", "BOOTSTRAP_SOURCE_USERNAME=importer",
				"BOOTSTRAP_DESTINATION_DATABASE=app_db", "BOOTSTRAP_CA_FILE="+filepath.Join(root, "ca.crt"),
				"BOOTSTRAP_TIMEOUT_SECONDS=60", "BOOTSTRAP_CR_UID=cr-uid", "BOOTSTRAP_PVC_NAME=postgres-db",
			)
			if tc.fail != "" {
				cmd.Env = append(cmd.Env, "FAIL_CMD="+tc.fail)
			}
			out, err := cmd.CombinedOutput()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(string(out), tc.wantErr) {
					t.Fatalf("error=%v output=%s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("bootstrap failed: %v\n%s", err, out)
			}
			marker := filepath.Join(parent, "pgdata", ".kubegres-bootstrap-complete")
			if tc.wantMarker {
				data, err := os.ReadFile(marker)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "cr_uid=cr-uid") || !strings.Contains(string(data), "pvc=postgres-db") {
					t.Fatalf("identity marker=%q", data)
				}
			}
		})
	}
}
