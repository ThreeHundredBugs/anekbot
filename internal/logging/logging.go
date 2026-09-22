package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type Level int

const (
	// LevelTrace logs every update the bot receives
	LevelTrace Level = iota
	// LevelDebug logs the chat ID and username behind each update.
	LevelDebug
	// LevelWarn only logs things that went wrong. This is the default.
	LevelWarn
)

var current = LevelWarn

func SetLevel(level Level) {
	current = level
}

func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(s) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return LevelDebug, nil
	case "warn":
		return LevelWarn, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: must be %q, %q or %q", s, "trace", "debug", "warn")
	}
}

// jsonValue defers marshaling until String is called, so a skipped log call costs nothing.
type jsonValue struct{ v any }

func (j jsonValue) String() string {
	data, err := json.Marshal(j.v)
	if err != nil {
		return fmt.Sprintf("%+v", j.v)
	}
	return string(data)
}

func AsJSON(v any) fmt.Stringer {
	return jsonValue{v}
}

func Tracef(format string, args ...any) {
	if current <= LevelTrace {
		log.Printf("TRACE "+format, args...)
	}
}

func Debugf(format string, args ...any) {
	if current <= LevelDebug {
		log.Printf("DEBUG "+format, args...)
	}
}

func Warnf(format string, args ...any) {
	if current <= LevelWarn {
		log.Printf("WARN "+format, args...)
	}
}
