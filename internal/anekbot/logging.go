package anekbot

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type LogLevel int

const (
	// LevelTrace logs every update the bot receives
	LevelTrace LogLevel = iota
	// LevelDebug logs the chat ID and username behind each update.
	LevelDebug
	// LevelWarn only logs things that went wrong. This is the default.
	LevelWarn
)

var currentLogLevel = LevelWarn

func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

func ParseLogLevel(s string) (LogLevel, error) {
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

func asJSON(v any) fmt.Stringer {
	return jsonValue{v}
}

func logTracef(format string, args ...any) {
	if currentLogLevel <= LevelTrace {
		log.Printf("TRACE "+format, args...)
	}
}

func logDebugf(format string, args ...any) {
	if currentLogLevel <= LevelDebug {
		log.Printf("DEBUG "+format, args...)
	}
}

func logWarnf(format string, args ...any) {
	if currentLogLevel <= LevelWarn {
		log.Printf("WARN "+format, args...)
	}
}
