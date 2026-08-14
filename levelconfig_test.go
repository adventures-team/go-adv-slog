package advslog

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPkg = "github.com/adventures-team/go-adv-slog" // this test's package

func TestPackageFromFunc(t *testing.T) {
	tests := []struct{ fn, expected string }{
		{"github.com/user/mod/pkg.Func", "github.com/user/mod/pkg"},
		{"github.com/user/mod/pkg.(*Type).Method", "github.com/user/mod/pkg"},
		{"github.com/user/mod/pkg.Type.Method", "github.com/user/mod/pkg"},
		{"github.com/user/mod/pkg.Func.func1", "github.com/user/mod/pkg"},
		{"github.com/user/mod/pkg.F[go.shape.int]", "github.com/user/mod/pkg"},
		{"github.com/user/mod/pkg.F[github.com/other/mod.T]", "github.com/user/mod/pkg"},
		{"main.main", "main"},
		{"main.(*T).M", "main"},
		{"runtime.goexit", "runtime"},
		{"github.com/onlineconf/onlineconf-go/v2.OpenModule", "github.com/onlineconf/onlineconf-go/v2"},
		{"gopkg.in/yaml.v3.Marshal", "gopkg.in/yaml.v3"},
		{"gopkg.in/yaml.v3.(*Decoder).Decode", "gopkg.in/yaml.v3"},
		{"some/pkg.v3", "some/pkg"}, // a function named v3, not a version suffix
		{"", ""},
	}

	for _, tt := range tests {
		if got := packageFromFunc(tt.fn); got != tt.expected {
			t.Errorf("packageFromFunc(%q) = %q, expected %q", tt.fn, got, tt.expected)
		}
	}
}

// fakeLevelConfig implements LevelConfig over a plain map, deriving children
// lists from the stored key paths the way the OnlineConf updater does.
type fakeLevelConfig struct {
	mu     sync.Mutex
	values map[string]string
	ch     chan<- struct{}
}

func newFakeLevelConfig(values map[string]string) *fakeLevelConfig {
	return &fakeLevelConfig{values: values}
}

func (f *fakeLevelConfig) GetStringIfExists(path string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	v, ok := f.values[path]

	return v, ok
}

func (f *fakeLevelConfig) GetStrings(path string, dfl []string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	seen := make(map[string]bool)

	var out []string

	for k := range f.values {
		if len(k) <= len(path) || !strings.HasPrefix(k, path) {
			continue
		}

		name, _, _ := strings.Cut(k[len(path):], "/")
		if !seen[name] {
			seen[name] = true

			out = append(out, name)
		}
	}

	if out == nil {
		return dfl
	}

	sort.Strings(out)

	return out
}

func (f *fakeLevelConfig) SubscribeChanSubtree(_ string, ch chan<- struct{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ch = ch

	return nil
}

func (f *fakeLevelConfig) set(path, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.values[path] = value
}

func (f *fakeLevelConfig) fire() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ch != nil {
		select {
		case f.ch <- struct{}{}:
		default:
		}
	}
}

// newTestLevelState builds a levelState without the watcher goroutine so that
// tests drive rebuild() synchronously.
func newTestLevelState(t *testing.T, values map[string]string) (*levelState, *fakeLevelConfig) {
	t.Helper()
	saveLevel(t)

	f := newFakeLevelConfig(values)
	s := &levelState{cfg: f}
	s.snapshot.Store(emptyLevelSnapshot)
	s.rebuild()

	return s, f
}

func saveLevel(t *testing.T) {
	t.Helper()

	old := Level.Level()
	t.Cleanup(func() { Level.Set(old) })
}

// otherPkgPC returns a PC belonging to package strings, for records
// "logged by" a foreign package.
func otherPkgPC() uintptr {
	return reflect.ValueOf(strings.ToUpper).Pointer()
}

func TestLevelConfigDefault(t *testing.T) {
	newTestLevelState(t, map[string]string{"/level": "warn"})

	if Level.Level() != slog.LevelWarn {
		t.Errorf("Level = %v, expected WARN from /level", Level.Level())
	}
}

func TestLevelConfigDefaultAbsent(t *testing.T) {
	saveLevel(t)
	Level.Set(slog.LevelError)

	newTestLevelState(t, map[string]string{})

	if Level.Level() != slog.LevelError {
		t.Errorf("Level = %v, expected the pre-set ERROR to survive an absent /level", Level.Level())
	}
}

func TestLevelConfigOverrides(t *testing.T) {
	st, _ := newTestLevelState(t, map[string]string{
		"/level":                  "info",
		"/" + testPkg + "/level":  "trace",
		"/some/other/pkg/level":   "error",
		"/strings/level":          "warn",
		"/" + testPkg + "/level/": `["ignored"]`, // a stray children list must not break the walk
	})

	var buf bytes.Buffer
	logger := slog.New(levelHandler{next: jsonHandler(&buf, false, levelAll), state: st})

	// this package is boosted to trace: debug passes
	logger.Debug("boosted debug")

	if !strings.Contains(buf.String(), `"msg":"boosted debug"`) {
		t.Errorf("boosted debug record missing: %s", buf.String())
	}

	// a debug record from package strings (override warn) is dropped
	buf.Reset()

	h := levelHandler{next: jsonHandler(&buf, false, levelAll), state: st}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, "foreign debug", otherPkgPC())

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 0 {
		t.Errorf("foreign debug record not dropped: %s", buf.String())
	}

	// a warn record from package strings passes
	r = slog.NewRecord(time.Now(), slog.LevelWarn, "foreign warn", otherPkgPC())

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), `"msg":"foreign warn"`) {
		t.Errorf("foreign warn record missing: %s", buf.String())
	}
}

func TestLevelConfigFloor(t *testing.T) {
	st, _ := newTestLevelState(t, map[string]string{
		"/level":                 "info",
		"/" + testPkg + "/level": "debug",
	})

	h := levelHandler{next: slog.DiscardHandler, state: st}
	ctx := context.Background()

	if !h.Enabled(ctx, slog.LevelDebug) {
		t.Error("Enabled(debug) = false despite a debug override (floor too high)")
	}

	if h.Enabled(ctx, LevelTrace) {
		t.Error("Enabled(trace) = true below the floor")
	}

	// without overrides the floor is the default level
	st2, _ := newTestLevelState(t, map[string]string{"/level": "info"})
	h2 := levelHandler{next: slog.DiscardHandler, state: st2}

	if h2.Enabled(ctx, slog.LevelDebug) {
		t.Error("Enabled(debug) = true with no override below the default")
	}

	if !h2.Enabled(ctx, slog.LevelInfo) {
		t.Error("Enabled(info) = false at the default level")
	}
}

func TestLevelConfigOnTheFly(t *testing.T) {
	st, f := newTestLevelState(t, map[string]string{"/level": "info"})

	var buf bytes.Buffer
	logger := slog.New(levelHandler{next: jsonHandler(&buf, false, levelAll), state: st})

	logger.Debug("before change")

	if buf.Len() != 0 {
		t.Fatalf("debug record not suppressed at info default: %s", buf.String())
	}

	f.set("/"+testPkg+"/level", "debug")
	st.rebuild()

	logger.Debug("after change")

	if !strings.Contains(buf.String(), `"msg":"after change"`) {
		t.Errorf("debug record missing after the on-the-fly boost: %s", buf.String())
	}
}

func TestLevelConfigInvalidValue(t *testing.T) {
	st, f := newTestLevelState(t, map[string]string{
		"/level":                 "info",
		"/" + testPkg + "/level": "debug",
	})

	// capture the WARN emitted by rebuild
	var warnBuf bytes.Buffer

	prev := slog.Default()
	slog.SetDefault(slog.New(jsonHandler(&warnBuf, false, Level)))

	defer slog.SetDefault(prev)

	f.set("/"+testPkg+"/level", "verbose")
	st.rebuild()

	if lvl := st.effective(testPkg); lvl != slog.LevelDebug {
		t.Errorf("effective level = %v, expected the previous DEBUG to survive an invalid value", lvl)
	}

	if !strings.Contains(warnBuf.String(), "invalid log level") {
		t.Errorf("no warning about the invalid value: %s", warnBuf.String())
	}

	// an invalid default keeps the current Level
	f.set("/level", "loud")
	st.rebuild()

	if Level.Level() != slog.LevelInfo {
		t.Errorf("Level = %v, expected INFO to survive an invalid /level", Level.Level())
	}
}

func TestLevelConfigWatcher(t *testing.T) {
	saveLevel(t)

	f := newFakeLevelConfig(map[string]string{"/level": "info"})
	st := newLevelState(f)

	f.set("/"+testPkg+"/level", "trace")
	f.fire()

	deadline := time.Now().Add(2 * time.Second)
	for st.effective(testPkg) != LevelTrace {
		if time.Now().After(deadline) {
			t.Fatal("the watcher did not apply the change")
		}

		time.Sleep(time.Millisecond)
	}
}

// lazyValue implements slog.LogValuer and records whether it was resolved.
type lazyValue struct{ resolved *bool }

func (v lazyValue) LogValue() slog.Value {
	*v.resolved = true

	return slog.StringValue("expensive")
}

func TestLevelConfigHeavyArgsContract(t *testing.T) {
	st, f := newTestLevelState(t, map[string]string{"/level": "info"})

	var buf bytes.Buffer
	logger := slog.New(levelHandler{next: jsonHandler(&buf, false, levelAll), state: st})
	ctx := context.Background()

	// nothing enables debug: the guard skips the expensive computation
	evaluated := false
	if logger.Enabled(ctx, slog.LevelDebug) {
		evaluated = true

		logger.Debug("guarded")
	}

	if evaluated {
		t.Error("Enabled guard passed below the floor")
	}

	// another package boosted to debug: the guard passes (floor moved), the
	// record from this unboosted package is still dropped in Handle
	f.set("/strings/level", "debug")
	st.rebuild()

	if !logger.Enabled(ctx, slog.LevelDebug) {
		t.Fatal("Enabled(debug) = false despite a foreign debug override")
	}

	logger.Debug("dropped per package")

	if buf.Len() != 0 {
		t.Errorf("record from an unboosted package not dropped: %s", buf.String())
	}

	// a LogValuer on a dropped record is never resolved
	resolved := false
	logger.LogAttrs(ctx, slog.LevelDebug, "lazy", slog.Any("v", lazyValue{&resolved}))

	if resolved {
		t.Error("LogValuer resolved on a dropped record")
	}
}

func TestLevelConfigDerivedLogger(t *testing.T) {
	st, _ := newTestLevelState(t, map[string]string{
		"/level":                 "info",
		"/" + testPkg + "/level": "debug",
	})

	var buf bytes.Buffer
	logger := slog.New(levelHandler{next: jsonHandler(&buf, false, levelAll), state: st}).With("request_id", "req-9")

	logger.Debug("derived keeps filtering and attrs")

	out := buf.String()
	if !strings.Contains(out, `"request_id":"req-9"`) || !strings.Contains(out, "derived keeps") {
		t.Errorf("derived logger record wrong: %s", out)
	}

	// trace is still below this package's debug override: dropped in the derived logger too
	buf.Reset()
	logger.Log(context.Background(), LevelTrace, "too verbose")

	if buf.Len() != 0 {
		t.Errorf("trace record not dropped by the derived handler: %s", buf.String())
	}
}

func TestLevelConfigWithRouting(t *testing.T) {
	st, _ := newTestLevelState(t, map[string]string{"/level": "warn"})

	var buf bytes.Buffer
	chain := routingHandler{next: levelHandler{next: jsonHandler(&buf, false, levelAll), state: st}}

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(chain))

	ctx := NewContext(context.Background(), slog.Default().With("request_id", "req-7"))

	// foreign code, suppressed level: routed, then dropped per package config
	slog.InfoContext(ctx, "foreign info")

	if buf.Len() != 0 {
		t.Fatalf("info record not dropped at warn default: %s", buf.String())
	}

	// foreign code, allowed level: routed and logged with the ctx attributes
	slog.WarnContext(ctx, "foreign warn")

	out := buf.String()
	if !strings.Contains(out, `"msg":"foreign warn"`) || !strings.Contains(out, `"request_id":"req-7"`) {
		t.Errorf("routed warn record wrong: %s", out)
	}
}

func TestLevelConfigPCZero(t *testing.T) {
	st, _ := newTestLevelState(t, map[string]string{"/level": "info"})

	var buf bytes.Buffer
	h := levelHandler{next: jsonHandler(&buf, false, levelAll), state: st}

	r := slog.NewRecord(time.Now(), slog.LevelDebug, "no pc", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 0 {
		t.Errorf("PC-less debug record not gated by the default: %s", buf.String())
	}

	r = slog.NewRecord(time.Now(), slog.LevelInfo, "no pc info", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), `"msg":"no pc info"`) {
		t.Errorf("PC-less info record missing: %s", buf.String())
	}
}

func TestLevelConfigConcurrentSwaps(t *testing.T) {
	st, f := newTestLevelState(t, map[string]string{"/level": "info"})

	logger := slog.New(levelHandler{next: slog.DiscardHandler, state: st})

	var wg sync.WaitGroup

	stop := make(chan struct{})

	for range 4 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					logger.Debug("spin", "i", 1)
					logger.Info("spin")
				}
			}
		}()
	}

	levels := []string{"trace", "debug", "warn", "error"}
	for i := range 200 {
		f.set("/"+testPkg+"/level", levels[i%len(levels)])
		st.rebuild()
	}

	close(stop)
	wg.Wait()
}

func TestWithLevelConfigPlumbing(t *testing.T) {
	saveLevel(t)

	var buf bytes.Buffer

	f := newFakeLevelConfig(map[string]string{
		"/level":                 "info",
		"/" + testPkg + "/level": "trace",
	})

	logger := slog.New(buildHandler(&buf, false, options{levelConfig: f}))

	// trace passes only if the sink is permissive and the override applies:
	// proves both the levelAll sink and the levelHandler wiring
	logger.Log(context.Background(), LevelTrace, "trace through the chain")

	if !strings.Contains(buf.String(), `"level":"TRACE"`) {
		t.Errorf("trace record missing from the built chain: %s", buf.String())
	}

	// routing wraps outermost when both options are given
	h := buildHandler(&buf, false, options{levelConfig: f, ctxRouting: true})
	rh, ok := h.(routingHandler)

	if !ok {
		t.Fatalf("outermost handler is %T, expected routingHandler", h)
	}

	if _, ok := rh.next.(levelHandler); !ok {
		t.Errorf("handler under routing is %T, expected levelHandler", rh.next)
	}
}

func TestInitTestWithLevelConfig(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	saveLevel(t)

	prev := slog.Default()
	defer slog.SetDefault(prev)

	f := newFakeLevelConfig(map[string]string{"/level": "warn"})
	rec := &tbRecorder{}
	logger := InitTest(rec, WithLevelConfig(f))

	logger.Info("suppressed info")
	logger.Warn("visible warn")

	if len(rec.lines) != 1 || !strings.Contains(rec.lines[0], "visible warn") {
		t.Errorf("expected exactly the warn line, got %q", rec.lines)
	}
}
