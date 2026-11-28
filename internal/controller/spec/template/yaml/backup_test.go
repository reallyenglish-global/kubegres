package yaml

import (
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestBackupPublication(t *testing.T) {
	var cm struct{ Data map[string]string }
	if err := yaml.Unmarshal([]byte(BaseConfigMapTemplate), &cm); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"dump-error", "gzip-error", "success"} {
		for _, previous := range []bool{false, true} {
			name := mode
			if previous {
				name += "-with-existing"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				dest := filepath.Join(dir, "volume")
				if mode == "success" {
					dest = filepath.Join(dir, "backup volume")
				}
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
				final := filepath.Join(dest, "fixture-backup-stamp.gz")
				write := func(path, body string) {
					t.Helper()
					if err := os.WriteFile(path, []byte(body), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if previous {
					write(final, "previous archive")
				}
				write(filepath.Join(dir, "date"), "#!/bin/sh\necho stamp\n")
				write(filepath.Join(dir, "pg_dumpall"), `#!/bin/sh
printf 'first row\n'
[ "$MODE" != dump-error ] || exit 42
printf 'last row\n'
`)
				if mode == "gzip-error" {
					write(filepath.Join(dir, "gzip"), "#!/bin/sh\ncat >/dev/null\nprintf truncated\nexit 7\n")
				}
				// Observe the file immediately before the real rename. Both paths must
				// share the destination directory, even when it contains whitespace.
				write(filepath.Join(dir, "mv"), `#!/bin/sh
set -eu
[ "$1" != -T ] || shift
[ "$1" != -- ] || shift
[ "$#" = 2 ] && [ "$1" != "$2" ] && [ "${1%/*}" = "$BACKUP_DESTINATION_FOLDER" ] && [ "$2" = "$FINAL" ] || exit 81
[ "$(stat -c %a "$1")" = 600 ] || exit 82
if [ "$PREVIOUS" = yes ]; then [ "$(cat "$2")" = 'previous archive' ]; else [ ! -e "$2" ]; fi
/usr/bin/mv -T -- "$1" "$2"
echo renamed > "$MARKER"
`)
				prev := "no"
				if previous {
					prev = "yes"
				}
				cmd := exec.Command("/bin/bash", "-s")
				cmd.Stdin = strings.NewReader(cm.Data["backup_database.sh"])
				cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "MODE=" + mode, "BACKUP_DESTINATION_FOLDER=" + dest, "KUBEGRES_RESOURCE_NAME=fixture", "BACKUP_SOURCE_DB_HOST_NAME=fixture.invalid", "FINAL=" + final, "PREVIOUS=" + prev, "MARKER=" + filepath.Join(dir, "marker")}
				output, err := cmd.CombinedOutput()
				success := mode == "success"
				if (err == nil) != success || strings.Contains(string(output), "DB backup completed") != success {
					t.Errorf("status %v; output %s", err, output)
				}
				entries, err := os.ReadDir(dest)
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				if previous || success {
					count = 1
				}
				if len(entries) != count {
					t.Fatalf("unexpected destination contents: %v", entries)
				}
				if success {
					if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
						t.Fatal("archive was not published by rename:", err)
					}
					f, err := os.Open(final)
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					r, err := gzip.NewReader(f)
					if err != nil {
						t.Fatal(err)
					}
					defer r.Close()
					data, err := io.ReadAll(r)
					if err != nil || string(data) != "first row\nlast row\n" {
						t.Fatalf("decompressed %q, %v", data, err)
					}
				} else if previous {
					data, err := os.ReadFile(final)
					if err != nil || string(data) != "previous archive" {
						t.Fatalf("previous backup was lost: %q, %v", data, err)
					}
				}
			})
		}
	}
}
