package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    Level
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
		got, err := ParseLevel(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseLevel(%q): expected an error, got none", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLevel(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	orig := current
	defer SetLevel(orig)

	origOutput := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
	}()
	log.SetFlags(0)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	SetLevel(LevelWarn)
	Tracef("trace msg")
	Debugf("debug msg")
	Warnf("warn msg")
	out := buf.String()
	if strings.Contains(out, "trace msg") || strings.Contains(out, "debug msg") {
		t.Errorf("expected trace/debug to be suppressed at warn level, got %q", out)
	}
	if !strings.Contains(out, "warn msg") {
		t.Errorf("expected the warn message to be logged, got %q", out)
	}

	buf.Reset()
	SetLevel(LevelTrace)
	Tracef("trace msg")
	Debugf("debug msg")
	Warnf("warn msg")
	out = buf.String()
	if !strings.Contains(out, "trace msg") || !strings.Contains(out, "debug msg") || !strings.Contains(out, "warn msg") {
		t.Errorf("expected all levels to be logged at trace level, got %q", out)
	}
}

// countingValue counts marshals, to prove AsJSON defers encoding until logged.
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
	orig := current
	defer SetLevel(orig)

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

	SetLevel(LevelWarn)
	Tracef("value: %s", AsJSON(v))
	if count != 0 {
		t.Errorf("expected AsJSON to not encode when trace logging is disabled, got %d encodes", count)
	}

	SetLevel(LevelTrace)
	Tracef("value: %s", AsJSON(v))
	if count != 1 {
		t.Errorf("expected AsJSON to encode exactly once when trace logging is enabled, got %d encodes", count)
	}
	if !strings.Contains(buf.String(), `{"field":"hello"}`) {
		t.Errorf("expected the log line to contain the JSON encoding, got %q", buf.String())
	}
}
