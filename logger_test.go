// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"testing"
	"time"
)

// fixedClock is the instant every deterministic test stamps lines with:
// 2025-06-29T15:58:32.123456 UTC (Unix 1751212712, 123456000 ns). Pairing it
// with the fixed pid below makes the formatted bytes byte-stable, so the
// ruby-free suite can assert exact lines and hold coverage at 100% on the
// no-ruby (Windows / qemu) lanes.
var fixedTime = time.Date(2025, 6, 29, 15, 58, 32, 123456000, time.UTC)

const fixedPid = 4242

func fixedClock() time.Time { return fixedTime }
func fixedPidFn() int       { return fixedPid }

// captured returns a Logger whose sink appends to *out, with the fixed
// clock/pid wired, so every line is deterministic.
func captured(out *[]string) *Logger {
	l := New(func(s string) { *out = append(*out, s) })
	l.Now = fixedClock
	l.Pid = fixedPidFn
	return l
}

func TestSeverityLabel(t *testing.T) {
	cases := map[Severity]string{
		DEBUG: "DEBUG", INFO: "INFO", WARN: "WARN", ERROR: "ERROR",
		FATAL: "FATAL", UNKNOWN: "ANY", 6: "ANY", -1: "ANY", 99: "ANY",
	}
	for s, want := range cases {
		if got := SeverityLabel(s); got != want {
			t.Errorf("SeverityLabel(%d) = %q, want %q", s, got, want)
		}
	}
}

func TestCoerceSeverity(t *testing.T) {
	cases := []struct {
		in      any
		want    Severity
		wantErr bool
	}{
		{INFO, INFO, false},
		{3, ERROR, false},
		{"warn", WARN, false},
		{"FATAL", FATAL, false},
		{"Unknown", UNKNOWN, false},
		{"nope", 0, true},
		{3.5, 0, true},
	}
	for _, c := range cases {
		got, err := CoerceSeverity(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("CoerceSeverity(%v) err=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("CoerceSeverity(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestDefaultFormatLines asserts the byte-exact default-format lines captured
// from MRI logger-1.7.0 with the fixed clock/pid above.
func TestDefaultFormatLines(t *testing.T) {
	var out []string
	l := captured(&out)
	l.Info("hello world", "myapp")
	l.Debug("msg2", "")
	l.Error("boom", "prog")
	l.Unknown("unknown msg", "app")

	want := []string{
		"I, [2025-06-29T15:58:32.123456 #4242]  INFO -- myapp: hello world\n",
		"D, [2025-06-29T15:58:32.123456 #4242] DEBUG -- : msg2\n",
		"E, [2025-06-29T15:58:32.123456 #4242] ERROR -- prog: boom\n",
		"A, [2025-06-29T15:58:32.123456 #4242]   ANY -- app: unknown msg\n",
	}
	if len(out) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(out), len(want), out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, out[i], want[i])
		}
	}
}

func TestWarnFatalHelpers(t *testing.T) {
	var out []string
	l := captured(&out)
	l.Warn("w", "app")
	l.Fatal("f", "app")
	want := []string{
		"W, [2025-06-29T15:58:32.123456 #4242]  WARN -- app: w\n",
		"F, [2025-06-29T15:58:32.123456 #4242] FATAL -- app: f\n",
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, out[i], want[i])
		}
	}
}

// TestLevelGating checks Add returns true but writes nothing below the level,
// and writes at or above it.
func TestLevelGating(t *testing.T) {
	var out []string
	l := captured(&out)
	l.Level = WARN

	if !l.Debug("dropped", "app") {
		t.Error("Add should always return true")
	}
	if !l.Info("dropped", "app") {
		t.Error("Add should always return true")
	}
	if len(out) != 0 {
		t.Fatalf("below-level messages leaked: %q", out)
	}
	l.Warn("kept", "app")
	l.Error("kept", "app")
	if len(out) != 2 {
		t.Fatalf("want 2 lines at/above level, got %d: %q", len(out), out)
	}
}

func TestNoSink(t *testing.T) {
	l := &Logger{Level: DEBUG, Now: fixedClock, Pid: fixedPidFn}
	if !l.Add(INFO, "x", "app") {
		t.Error("Add with no sink must return true")
	}
	if n := l.Write("raw"); n != -1 {
		t.Errorf("Write with no sink = %d, want -1", n)
	}
}

func TestWriteRaw(t *testing.T) {
	var out []string
	l := captured(&out)
	n := l.Write("raw bytes")
	if n != len("raw bytes") {
		t.Errorf("Write returned %d, want %d", n, len("raw bytes"))
	}
	if len(out) != 1 || out[0] != "raw bytes" {
		t.Errorf("raw write = %q", out)
	}
}

func TestDefaultPrognameFallback(t *testing.T) {
	var out []string
	l := captured(&out)
	l.Progname = "globalprog"
	l.Info("m", "") // empty progname picks up the default
	want := "I, [2025-06-29T15:58:32.123456 #4242]  INFO -- globalprog: m\n"
	if out[0] != want {
		t.Errorf("got %q want %q", out[0], want)
	}
}

func TestNegativeSeverityIsUnknown(t *testing.T) {
	var out []string
	l := captured(&out)
	l.Add(-1, "m", "app") // MRI's severity ||= UNKNOWN
	want := "A, [2025-06-29T15:58:32.123456 #4242]   ANY -- app: m\n"
	if out[0] != want {
		t.Errorf("got %q want %q", out[0], want)
	}
}

func TestLogAlias(t *testing.T) {
	var out []string
	l := captured(&out)
	if !l.Log(INFO, "m", "app") {
		t.Error("Log should return true")
	}
	if len(out) != 1 {
		t.Errorf("Log wrote %d lines", len(out))
	}
}

func TestPredicates(t *testing.T) {
	l := &Logger{}
	l.Level = WARN
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"debug?", l.DebugQ(), false},
		{"info?", l.InfoQ(), false},
		{"warn?", l.WarnQ(), true},
		{"error?", l.ErrorQ(), true},
		{"fatal?", l.FatalQ(), true},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v at WARN", c.name, c.got, c.want)
		}
	}
	l.Level = DEBUG
	if !l.DebugQ() || !l.InfoQ() {
		t.Error("at DEBUG, debug? and info? must be true")
	}
}

func TestSetLevel(t *testing.T) {
	l := &Logger{}
	if err := l.SetLevel("error"); err != nil || l.Level != ERROR {
		t.Errorf("SetLevel(error) -> level=%d err=%v", l.Level, err)
	}
	if err := l.SetLevel(2); err != nil || l.Level != WARN {
		t.Errorf("SetLevel(2) -> level=%d err=%v", l.Level, err)
	}
	if err := l.SetLevel("bogus"); err == nil {
		t.Error("SetLevel(bogus) should error")
	}
}

// TestDefaultClockAndPid exercises the real-clock/real-pid fallbacks so the
// seams are covered even though their output is non-deterministic.
func TestDefaultClockAndPid(t *testing.T) {
	var out []string
	l := New(func(s string) { out = append(out, s) }) // real Now/Pid
	l.Info("real", "app")
	if len(out) != 1 {
		t.Fatalf("expected one line, got %d", len(out))
	}
	// A nil-clock/nil-pid logger must also fall back, not panic.
	l2 := &Logger{Level: DEBUG, Formatter: nil, Sink: func(string) {}}
	if !l2.Add(INFO, "m", "p") {
		t.Error("nil clock/pid/formatter must still work")
	}
	if defaultPid() <= 0 {
		t.Error("defaultPid should be positive")
	}
}
