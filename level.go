package advslog

import (
	"log/slog"
	"strings"
)

// Additional levels complementing the standard [slog.Level] values, matching
// the zerolog level set. Handlers created by [Init] and [InitTest] render them
// by name ("TRACE", "FATAL", "PANIC") instead of slog's default offset
// spelling ("DEBUG-4", "ERROR+4", "ERROR+8").
//
// LevelPanic exists for level parsing and rendering only: unlike zerolog,
// there is no helper that logs and panics.
const (
	LevelTrace = slog.Level(-8)
	LevelFatal = slog.Level(12)
	LevelPanic = slog.Level(16)
)

// Level is the dynamic logging level used by all handlers created by [Init]
// and [InitTest]. Adjust it at runtime with Level.Set — the replacement for
// zerolog.SetGlobalLevel.
//
// It defaults to [LevelTrace]: like zerolog, everything is logged until a
// level is set explicitly.
var Level = defaultLevel()

func defaultLevel() *slog.LevelVar {
	v := new(slog.LevelVar)
	v.Set(LevelTrace)

	return v
}

// Names of the custom levels as they appear in output. levelNameWarning is
// never rendered (warning is a standard level) but, uppercased like the rest,
// it lets [ParseLevel] match every name with a single ToUpper.
const (
	levelNameTrace   = "TRACE"
	levelNameFatal   = "FATAL"
	levelNamePanic   = "PANIC"
	levelNameWarning = "WARNING"
)

// ParseLevel converts a level name to a [slog.Level]. On top of the names and
// offsets understood by [slog.Level.UnmarshalText] (case-insensitive "debug",
// "INFO", "ERROR+4", ...), it accepts the zerolog names: "trace", "warning",
// "fatal" and "panic".
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(s) {
	case levelNameTrace:
		return LevelTrace, nil
	case levelNameWarning:
		return slog.LevelWarn, nil
	case levelNameFatal:
		return LevelFatal, nil
	case levelNamePanic:
		return LevelPanic, nil
	}

	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, err
	}

	return l, nil
}

var levelNames = map[slog.Level]string{
	LevelTrace: levelNameTrace,
	LevelFatal: levelNameFatal,
	LevelPanic: levelNamePanic,
}

// replaceLevelName is a HandlerOptions.ReplaceAttr function naming the custom
// levels in handler output. Standard levels are left to the handler so that it
// can apply its own formatting (e.g. tint's colors).
func replaceLevelName(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.LevelKey {
		if lvl, ok := a.Value.Any().(slog.Level); ok {
			if name, ok := levelNames[lvl]; ok {
				a.Value = slog.StringValue(name)
			}
		}
	}

	return a
}
