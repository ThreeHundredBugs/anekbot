package anekbot

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    LogLevel
		wantErr bool
	}{
		{"trace", LevelTrace, false},
		{"TRACE", LevelTrace, false},
		{"debug", LevelDebug, false},
		{"Debug", LevelDebug, false},
		{"warn", LevelWarn, false},
		{"bogus", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseLogLevel(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseLogLevel(%q): expected an error, got none", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLogLevel(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLogLevelFiltering(t *testing.T) {
	orig := currentLogLevel
	defer SetLogLevel(orig)

	origOutput := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
	}()
	log.SetFlags(0)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	SetLogLevel(LevelWarn)
	logTracef("trace msg")
	logDebugf("debug msg")
	logWarnf("warn msg")
	out := buf.String()
	if strings.Contains(out, "trace msg") || strings.Contains(out, "debug msg") {
		t.Errorf("expected trace/debug to be suppressed at warn level, got %q", out)
	}
	if !strings.Contains(out, "warn msg") {
		t.Errorf("expected the warn message to be logged, got %q", out)
	}

	buf.Reset()
	SetLogLevel(LevelTrace)
	logTracef("trace msg")
	logDebugf("debug msg")
	logWarnf("warn msg")
	out = buf.String()
	if !strings.Contains(out, "trace msg") || !strings.Contains(out, "debug msg") || !strings.Contains(out, "warn msg") {
		t.Errorf("expected all levels to be logged at trace level, got %q", out)
	}
}

// countingValue counts marshals, to prove asJSON defers encoding until logged.
type countingValue struct {
	Field        string `json:"field"`
	marshalCount *int
}

func (c countingValue) MarshalJSON() ([]byte, error) {
	*c.marshalCount++
	return json.Marshal(struct {
		Field string `json:"field"`
	}{Field: c.Field})
}

func TestAsJSON_DefersEncodingUntilLogged(t *testing.T) {
	orig := currentLogLevel
	defer SetLogLevel(orig)

	origOutput := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
	}()
	log.SetFlags(0)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	count := 0
	v := countingValue{Field: "hello", marshalCount: &count}

	SetLogLevel(LevelWarn)
	logTracef("value: %s", asJSON(v))
	if count != 0 {
		t.Errorf("expected asJSON to not encode when trace logging is disabled, got %d encodes", count)
	}

	SetLogLevel(LevelTrace)
	logTracef("value: %s", asJSON(v))
	if count != 1 {
		t.Errorf("expected asJSON to encode exactly once when trace logging is enabled, got %d encodes", count)
	}
	if !strings.Contains(buf.String(), `{"field":"hello"}`) {
		t.Errorf("expected the log line to contain the JSON encoding, got %q", buf.String())
	}
}
