package advslog

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// LevelConfig is the configuration source for per-package log levels: the
// subset of onlineconf-go/v2 shared by *onlineconf.Module and
// *onlineconf.Subtree. Both satisfy it as-is.
type LevelConfig interface {
	GetStringIfExists(path string) (string, bool)
	GetStrings(path string, dfl []string) []string
	SubscribeChanSubtree(path string, ch chan<- struct{}) error
}

// levelNever is above every real level: the floor when no overrides exist.
const levelNever = slog.Level(math.MaxInt32)

// levelAll is below every real level: the sink level while levelHandler owns
// all gating (both slog.HandlerOptions.Level and tint's Options.Level default
// to Info when nil, so the permissive level must be explicit).
const levelAll = slog.Level(math.MinInt32)

// levelSnapshot is an immutable view of the configured overrides, swapped
// atomically on every configuration change.
type levelSnapshot struct {
	levels      map[string]slog.Level // import path -> level
	minOverride slog.Level            // min of levels values, levelNever if none
}

var emptyLevelSnapshot = &levelSnapshot{minOverride: levelNever}

// levelState connects a LevelConfig to the handlers: it owns the snapshot and
// the subscription watcher.
type levelState struct {
	cfg      LevelConfig
	snapshot atomic.Pointer[levelSnapshot]
}

// newLevelState reads the initial configuration, subscribes to changes and
// starts the watcher goroutine (which lives for the process lifetime, like
// the onlineconf-go updater itself).
func newLevelState(cfg LevelConfig) *levelState {
	s := &levelState{cfg: cfg}
	s.snapshot.Store(emptyLevelSnapshot)

	// subscribe before the initial read so no update can fall in between
	ch := make(chan struct{}, 1)
	if err := cfg.SubscribeChanSubtree("", ch); err != nil {
		slog.Warn("advslog: level config subscription failed, levels are frozen at their initial values", Err(err))
	}

	s.rebuild()

	go func() {
		for range ch {
			// notifications are coalescing and carry no payload:
			// re-read everything on every wake-up
			s.rebuild()
		}
	}()

	return s
}

// rebuild re-reads the default level and walks the children lists into a new
// snapshot. It runs once per change notification.
func (s *levelState) rebuild() {
	if str, ok := s.cfg.GetStringIfExists("/level"); ok {
		if lvl, err := ParseLevel(str); err == nil {
			Level.Set(lvl)
		} else {
			slog.Warn("advslog: invalid default log level in config", "value", str, Err(err))
		}
	}

	prev := s.snapshot.Load()
	levels := make(map[string]slog.Level)
	s.walk("", prev, levels)

	minOverride := levelNever
	for _, lvl := range levels {
		if lvl < minOverride {
			minOverride = lvl
		}
	}

	s.snapshot.Store(&levelSnapshot{levels: levels, minOverride: minOverride})
}

// walk descends the configuration tree using OnlineConf children lists: for
// every node, "<path>/" (trailing slash) holds a JSON array of child names,
// readable via the public GetStrings. A "level" child holds the override for
// the import path spelled by its parent node.
func (s *levelState) walk(node string, prev *levelSnapshot, out map[string]slog.Level) {
	for _, child := range s.cfg.GetStrings(node+"/", nil) {
		childPath := node + "/" + child

		if child == "level" && node != "" {
			s.readOverride(node[1:], childPath, prev, out)
		}

		// descend even into "level" nodes: a package path may contain a
		// segment literally named "level"
		s.walk(childPath, prev, out)
	}
}

// readOverride parses one "/<pkg>/level" value into out. An invalid value
// keeps the previous override (and warns); a missing string is skipped.
func (s *levelState) readOverride(pkg, path string, prev *levelSnapshot, out map[string]slog.Level) {
	str, ok := s.cfg.GetStringIfExists(path)
	if !ok {
		return
	}

	lvl, err := ParseLevel(str)
	if err != nil {
		slog.Warn("advslog: invalid log level in config", "package", pkg, "value", str, Err(err))

		if old, ok := prev.levels[pkg]; ok {
			out[pkg] = old
		}

		return
	}

	out[pkg] = lvl
}

// effective returns the level in force for a package.
func (s *levelState) effective(pkg string) slog.Level {
	if pkg != "" {
		if lvl, ok := s.snapshot.Load().levels[pkg]; ok {
			return lvl
		}
	}

	return Level.Level()
}

// levelHandler gates records by the per-package configuration. It owns all
// level filtering while level config is active: the sink runs at levelAll.
type levelHandler struct {
	next  slog.Handler
	state *levelState
}

// Enabled reports whether any package could emit at this level: the floor is
// min(default, lowest override). Enabled receives no caller information, so a
// per-package answer is impossible here; the per-package decision happens in
// Handle. Explicit `if logger.Enabled(...)` guards around expensive argument
// computation therefore keep working at floor granularity.
func (h levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	if level >= Level.Level() {
		return true
	}

	return level >= h.state.snapshot.Load().minOverride
}

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error {
	var pkg string
	if r.PC != 0 {
		pkg = packageForPC(r.PC)
	}

	if r.Level < h.state.effective(pkg) {
		return nil
	}

	return h.next.Handle(ctx, r)
}

// Derivations keep filtering: per-package gating is orthogonal to accumulated
// attributes, and every record carries its own PC.

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{next: h.next.WithAttrs(attrs), state: h.state}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{next: h.next.WithGroup(name), state: h.state}
}

// pcPkgCache maps program counters to import paths. Call sites are finite and
// their PCs never change, so the cache is append-only and survives
// configuration changes.
var pcPkgCache sync.Map // uintptr -> string

// packageForPC resolves the import path of the code at pc, caching the
// symbolization per call site.
func packageForPC(pc uintptr) string {
	if v, ok := pcPkgCache.Load(pc); ok {
		return v.(string) //nolint:revive // the map holds strings only
	}

	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	pkg := packageFromFunc(frame.Function)
	pcPkgCache.Store(pc, pkg)

	return pkg
}

// packageFromFunc extracts the import path from a fully qualified symbol name
// as reported by the runtime: "github.com/user/mod/pkg.Func",
// "github.com/user/mod/pkg.(*Type).Method", "main.main".
//
// The import path is everything up to the first dot after the last slash,
// with one special case: a "vN" token right after that dot which is followed
// by another dot belongs to the path ("gopkg.in/yaml.v3.Marshal" ->
// "gopkg.in/yaml.v3"). Returns "" for an empty (unresolvable) symbol.
func packageFromFunc(fn string) string {
	// generic instantiations append "[...]" type arguments, which may contain
	// dots and slashes of their own; the import path always ends before them
	if i := strings.IndexByte(fn, '['); i >= 0 {
		fn = fn[:i]
	}

	slash := strings.LastIndexByte(fn, '/') // -1 for path-less packages like "main"
	rest := fn[slash+1:]

	dot := strings.IndexByte(rest, '.')
	if dot < 0 {
		return "" // not a qualified symbol
	}

	if n := versionSuffixLen(rest[dot+1:]); n > 0 {
		dot += 1 + n
	}

	return fn[:slash+1+dot]
}

// versionSuffixLen returns the length of a leading "v<digits>" token followed
// by a dot, or 0.
func versionSuffixLen(s string) int {
	if len(s) < 2 || s[0] != 'v' {
		return 0
	}

	i := 1
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	if i == 1 || i >= len(s) || s[i] != '.' {
		return 0
	}

	return i
}
