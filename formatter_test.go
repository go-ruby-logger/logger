// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"strings"
	"testing"
	"time"
)

func TestFormatterDatetimeOverride(t *testing.T) {
	f := &Formatter{DatetimeFormat: "%Y-%m-%d %H:%M:%S"}
	got := f.Format("WARN", fixedTime, fixedPid, "app", "warned")
	want := "W, [2025-06-29 15:58:32 #4242]  WARN -- app: warned\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMsg2strExceptionWithBacktrace(t *testing.T) {
	f := &Formatter{}
	exc := Exception{
		Message:   "bad arg",
		Class:     "ArgumentError",
		Backtrace: []string{"file.rb:10:in `foo`", "file.rb:20:in `bar`"},
	}
	got := f.Format("ERROR", fixedTime, fixedPid, "app", exc)
	want := "E, [2025-06-29T15:58:32.123456 #4242] ERROR -- app: bad arg (ArgumentError)\n" +
		"file.rb:10:in `foo`\nfile.rb:20:in `bar`\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestMsg2strExceptionNoBacktrace(t *testing.T) {
	f := &Formatter{}
	exc := Exception{Message: "oops", Class: "RuntimeError"} // nil backtrace
	got := f.msg2str(exc)
	want := "oops (RuntimeError)\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMsg2strInspectorWired(t *testing.T) {
	f := &Formatter{Inspect: func(any) string { return "[1, 2, 3]" }}
	got := f.Format("INFO", fixedTime, fixedPid, "app", []int{1, 2, 3})
	want := "I, [2025-06-29T15:58:32.123456 #4242]  INFO -- app: [1, 2, 3]\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMsg2strDefaultInspect(t *testing.T) {
	f := &Formatter{} // no Inspector
	if got := f.msg2str(42); got != "42" {
		t.Errorf("default inspect of int = %q, want 42", got)
	}
	if got := f.msg2str(nil); got != "nil" {
		t.Errorf("default inspect of nil = %q, want nil", got)
	}
}

func TestFirst1AndPad5EdgeCases(t *testing.T) {
	if got := first1(""); got != "" {
		t.Errorf("first1(\"\") = %q", got)
	}
	// An empty label exercises the %.1s-of-empty branch through Format.
	f := &Formatter{}
	got := f.Format("", fixedTime, fixedPid, "app", "m")
	if !strings.HasPrefix(got, ", [") {
		t.Errorf("empty label line = %q", got)
	}
	// A label longer than five chars is left intact by %5s.
	if got := pad5("SEVERE"); got != "SEVERE" {
		t.Errorf("pad5(SEVERE) = %q", got)
	}
}

func TestNilFormatterDefault(t *testing.T) {
	// A Logger with a nil Formatter falls back to a zero-value default.
	var out []string
	l := &Logger{Level: DEBUG, Formatter: nil, Sink: func(s string) { out = append(out, s) }, Now: fixedClock, Pid: fixedPidFn}
	l.Info("m", "app")
	want := "I, [2025-06-29T15:58:32.123456 #4242]  INFO -- app: m\n"
	if out[0] != want {
		t.Errorf("got %q want %q", out[0], want)
	}
}

func TestStrftimeDirectives(t *testing.T) {
	// 2025-06-29 is a Sunday; 15:58:32.123456 UTC.
	tm := fixedTime
	cases := []struct{ fmt, want string }{
		{"%Y-%m-%dT%H:%M:%S.%6N", "2025-06-29T15:58:32.123456"},
		{"%y", "25"},
		{"%3N", "123"},
		{"%9N", "123456000"},
		{"%N", "123456000"},
		{"%L", "123"},
		{"%I %p %P", "03 PM pm"},
		{"%A %a", "Sunday Sun"},
		{"%B %b %h", "June Jun Jun"},
		{"%j", "180"},
		{"%e", "29"},
		{"%Z", "UTC"},
		{"%z", "+0000"},
		{"100%%done", "100%done"},
		{"%Q", "%Q"},                 // unknown directive emitted verbatim
		{"trailing %", "trailing %"}, // lone trailing percent
		{"%6", "%6"},                 // trailing percent + width, no directive
	}
	for _, c := range cases {
		if got := strftime(tm, c.fmt); got != c.want {
			t.Errorf("strftime(%q) = %q, want %q", c.fmt, got, c.want)
		}
	}
}

func TestStrftimeFractionalEdges(t *testing.T) {
	// %12N pads with trailing zeros beyond nanosecond precision; %0N is empty.
	tm := time.Date(2025, 1, 1, 0, 0, 0, 123456000, time.UTC)
	if got := strftime(tm, "%12N"); got != "123456000000" {
		t.Errorf("%%12N = %q", got)
	}
	if got := strftime(tm, "%0N"); got != "123456000" {
		t.Errorf("%%0N = %q, want 123456000", got)
	}
}

func TestStrftimeZoneOffsetNegative(t *testing.T) {
	loc := time.FixedZone("WEST", -7*3600-30*60) // -07:30
	tm := time.Date(2025, 6, 29, 12, 0, 0, 0, loc)
	if got := strftime(tm, "%z"); got != "-0730" {
		t.Errorf("%%z = %q, want -0730", got)
	}
	if got := zoneOffset(tm, true); got != "-07:30" {
		t.Errorf("zoneOffset colon = %q", got)
	}
}

func TestStrftimeSmallDayAndYearDay(t *testing.T) {
	// 2025-01-05 is day-of-year 5 and a single-digit day: %e space-pads, %j
	// zero-pads to three digits.
	tm := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	if got := strftime(tm, "%e"); got != " 5" {
		t.Errorf("%%e single-digit = %q, want \" 5\"", got)
	}
	if got := strftime(tm, "%j"); got != "005" {
		t.Errorf("%%j day 5 = %q, want 005", got)
	}
	// A two-digit yearday still zero-pads to three.
	feb := time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC) // day 41
	if got := strftime(feb, "%j"); got != "041" {
		t.Errorf("%%j day 41 = %q, want 041", got)
	}
}

func TestStrftimeNoonAMPM(t *testing.T) {
	noon := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if got := strftime(noon, "%I %p"); got != "12 PM" {
		t.Errorf("noon %%I %%p = %q", got)
	}
	midnightT := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := strftime(midnightT, "%I %p"); got != "12 AM" {
		t.Errorf("midnight %%I %%p = %q", got)
	}
	// Morning exercises the lowercase %P "am" branch.
	if got := strftime(midnightT, "%P"); got != "am" {
		t.Errorf("midnight %%P = %q, want am", got)
	}
}

func TestStrftimeSubMicrosecondNanos(t *testing.T) {
	// 123 ns left-pads to "000000123" before truncation, exercising the
	// nanosecond zero-fill path.
	tm := time.Date(2025, 1, 1, 0, 0, 0, 123, time.UTC)
	if got := strftime(tm, "%9N"); got != "000000123" {
		t.Errorf("%%9N of 123ns = %q, want 000000123", got)
	}
	if got := strftime(tm, "%6N"); got != "000000" {
		t.Errorf("%%6N of 123ns = %q, want 000000", got)
	}
}
