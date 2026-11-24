package yaml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"
)

func TestPromotionHook(t *testing.T) {
	var cm struct{ Data map[string]string }
	if err := yaml.Unmarshal([]byte(BaseConfigMapTemplate), &cm); err != nil {
		t.Fatal(err)
	}
	hook := cm.Data["promote_replica_to_primary.sh"]
	for _, tc := range []struct {
		name                       string
		standby                    bool
		readyAfter, ctlStatus      int
		wantFailure, wantPromotion bool
	}{
		{"already primary", false, 0, 0, false, false},
		{"ready after retry", true, 1, 0, false, true},
		{"never ready", true, 100, 0, true, false},
		{"promotion fails", true, 0, 7, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.standby {
				write("standby.signal", "")
			}
			write("date", "#!/bin/sh\necho timestamp\n")
			write("sleep", "#!/bin/sh\necho wait >> waits\n")
			write("pg_isready", `#!/bin/bash
[[ $* == '-U postgres' ]] || exit 2
probes=0
[[ ! -e probes ]] || read -r probes < probes
echo $((probes + 1)) > probes
(( probes >= READY_AFTER ))
`)
			write("su", `#!/bin/bash
[[ $# == 3 && $1 == -c && $3 == postgres ]] || exit 2
exec /bin/bash -c "$2"
`)
			write("pg_ctl", `#!/bin/bash
[[ $# == 3 && $1 == promote && $2 == -D && $3 == "$PGDATA" ]] || exit 2
echo promote >> promotions
exit "$CTL_STATUS"
`)
			// Relocate only the image-specific absolute paths. No real PG utilities,
			// credentials, sleeps or host log files are involved.
			write("hook", strings.NewReplacer("/usr/share/promotion.log", "promotion.log", "/usr/lib/postgresql/$PG_MAJOR/bin/pg_ctl", "pg_ctl").Replace(hook))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/bash", "hook")
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + dir, "PGDATA=" + dir, fmt.Sprintf("READY_AFTER=%d", tc.readyAfter), fmt.Sprintf("CTL_STATUS=%d", tc.ctlStatus)}
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if ctx.Err() != nil || (err != nil && !errors.As(err, &exit)) || (err != nil) != tc.wantFailure {
				t.Errorf("hook returned %v, want failure=%v: %s", err, tc.wantFailure, output)
			}
			read := func(name string) string {
				b, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				return string(b)
			}
			want := ""
			if tc.wantPromotion {
				want = "promote\n"
			}
			if got := read("promotions"); got != want {
				t.Errorf("pg_ctl invocations %q, want %q", got, want)
			}
			if tc.readyAfter > 0 && read("waits") == "" {
				t.Error("hook did not wait for readiness")
			}
			if !tc.standby && read("probes") != "" {
				t.Error("unexpected readiness probe on primary")
			}
		})
	}
}
