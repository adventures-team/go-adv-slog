// Package leveltest integration-tests the advslog per-package level
// configuration against the real onlineconf-go library. CDB module files are
// produced by the yaml2cdb tool (a go.mod tool dependency, run via `go tool`)
// and configuration updates are delivered by moving a new file over the old
// one — exactly how the OnlineConf updater deploys changes, and what
// onlineconf-go's fsnotify watcher reacts to.
//
// The package is a separate Go module on purpose: onlineconf-go and yaml2cdb
// stay out of the library's go.mod. Run with `go test ./...` from this
// directory; the main module's tests do not include it.
package leveltest

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	advslog "github.com/adventures-team/go-adv-slog"
	onlineconf "github.com/onlineconf/onlineconf-go/v2"
)

// The fixtures configure an override for this test's own import path
// (github.com/adventures-team/go-adv-slog/leveltest) — records logged by the
// test functions resolve to it.
//
// The children lists ("<path>/" keys holding JSON arrays of child names) are
// spelled explicitly as ""-keyed YAML entries: yaml2cdb joins map keys with
// "/", so a "" key produces the trailing-slash path, and a YAML sequence is
// written as a 'j'-typed JSON value — the exact child_lists format
// onlineconf-go requires for subtree subscriptions and that the level walk
// enumerates.
const subtreeYAMLv1 = `
"": ["log"]
log:
  "": ["level", "github.com"]
  level: info
  github.com:
    "": ["adventures-team"]
    adventures-team:
      "": ["go-adv-slog"]
      go-adv-slog:
        "": ["leveltest"]
        leveltest:
          "": ["level"]
          level: trace
`

// v2: the default tightens to warn and the per-package override disappears.
const subtreeYAMLv2 = `
"": ["log"]
log:
  "": ["level"]
  level: warn
`

const moduleYAMLv1 = `
"": ["level", "github.com"]
level: info
github.com:
  "": ["adventures-team"]
  adventures-team:
    "": ["go-adv-slog"]
    go-adv-slog:
      "": ["leveltest"]
      leveltest:
        "": ["level"]
        level: debug
`

const moduleYAMLv2 = `
"": ["level"]
level: error
`

// recorder implements advslog.TestingLog, collecting the log lines.
type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Log(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, a := range args {
		if s, ok := a.(string); ok {
			r.lines = append(r.lines, s)
		}
	}
}

func (r *recorder) contains(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, l := range r.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}

	return false
}

// buildCDB converts inline YAML to a CDB file using the yaml2cdb tool from
// this module's go.mod.
func buildCDB(t *testing.T, dir, name, yamlText string) string {
	t.Helper()

	yamlPath := filepath.Join(dir, name+".yaml")
	cdbPath := filepath.Join(dir, name+".cdb")

	if err := os.WriteFile(yamlPath, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("go", "tool", "yaml2cdb", "-in", yamlPath, "-out", cdbPath).CombinedOutput()
	if err != nil {
		t.Fatalf("yaml2cdb: %v\n%s", err, out)
	}

	return cdbPath
}

// deploy moves a freshly built CDB over the module file, the way the
// OnlineConf updater deploys configuration updates.
func deploy(t *testing.T, cdbPath, modulePath string) {
	t.Helper()

	if err := os.Rename(cdbPath, modulePath); err != nil {
		t.Fatal(err)
	}
}

// moduleDir returns a temp dir with symlinks resolved: onlineconf-go matches
// fsnotify event paths against the opened file name byte-for-byte.
func moduleDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

func saveGlobals(t *testing.T) {
	t.Helper()

	level := advslog.Level.Level()
	prev := slog.Default()
	t.Cleanup(func() {
		advslog.Level.Set(level)
		slog.SetDefault(prev)
	})
}

// waitFor polls until cond holds or the deadline expires: file watching,
// reopening and the subscription watcher are all asynchronous.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// otherPkgPC returns a PC from package strings, for records "logged by" a
// package without an override.
func otherPkgPC() uintptr {
	return reflect.ValueOf(strings.ToUpper).Pointer()
}

// TestSubtreeLiveUpdate drives the recommended setup: a Subtree scoped to the
// logging section, whose change detection depends on the child_lists written
// by the fixtures.
func TestSubtreeLiveUpdate(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	saveGlobals(t)

	dir := moduleDir(t)
	module := filepath.Join(dir, "leveltest-subtree.cdb")
	deploy(t, buildCDB(t, dir, "v1", subtreeYAMLv1), module)

	mod, err := onlineconf.OpenModule(module)
	if err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	logger := advslog.InitTest(rec, advslog.WithLevelConfig(mod.Subtree("/log")))

	if advslog.Level.Level() != slog.LevelInfo {
		t.Fatalf("Level = %v, expected INFO from /log/level", advslog.Level.Level())
	}

	// this package is boosted to trace by the override
	logger.Debug("boosted debug v1")

	if !rec.contains("boosted debug v1") {
		t.Errorf("boosted debug record missing: %q", rec.lines)
	}

	// a debug record from a package without an override obeys the default
	r := slog.NewRecord(time.Now(), slog.LevelDebug, "foreign debug v1", otherPkgPC())
	if err := logger.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if rec.contains("foreign debug v1") {
		t.Errorf("foreign debug record not dropped at the info default: %q", rec.lines)
	}

	// deploy v2: default warn, override gone
	deploy(t, buildCDB(t, dir, "v2", subtreeYAMLv2), module)
	waitFor(t, "the v2 default level", func() bool { return advslog.Level.Level() == slog.LevelWarn })

	logger.Debug("stale debug v2")
	logger.Info("stale info v2")
	logger.Warn("visible warn v2")

	if rec.contains("stale debug v2") || rec.contains("stale info v2") {
		t.Errorf("records below the v2 default not dropped: %q", rec.lines)
	}

	if !rec.contains("visible warn v2") {
		t.Errorf("warn record missing after the v2 update: %q", rec.lines)
	}
}

// TestModuleRootConfig drives the whole-Module flavor, whose subscription
// sits at the OnlineConf root.
func TestModuleRootConfig(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	saveGlobals(t)

	dir := moduleDir(t)
	module := filepath.Join(dir, "leveltest-module.cdb")
	deploy(t, buildCDB(t, dir, "v1", moduleYAMLv1), module)

	mod, err := onlineconf.OpenModule(module)
	if err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	logger := advslog.InitTest(rec, advslog.WithLevelConfig(mod))

	logger.Debug("boosted debug v1")

	if !rec.contains("boosted debug v1") {
		t.Errorf("debug record missing despite the debug override: %q", rec.lines)
	}

	deploy(t, buildCDB(t, dir, "v2", moduleYAMLv2), module)
	waitFor(t, "the v2 default level", func() bool { return advslog.Level.Level() == slog.LevelError })

	logger.Debug("stale debug v2")
	logger.Warn("stale warn v2")
	logger.Error("visible error v2")

	if rec.contains("stale debug v2") || rec.contains("stale warn v2") {
		t.Errorf("records below the v2 default not dropped: %q", rec.lines)
	}

	if !rec.contains("visible error v2") {
		t.Errorf("error record missing after the v2 update: %q", rec.lines)
	}
}
