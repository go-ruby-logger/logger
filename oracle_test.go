// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// rubyBin locates a usable `ruby` once. The oracle tests skip themselves when it
// is absent (the qemu cross-arch lanes and the Windows lane), so the
// deterministic suite alone drives the 100% gate there.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	return path
}

// rubyEval runs a Ruby script and returns its stdout. The script $stdout.binmode's
// itself so Windows text-mode never pollutes the bytes (the go-ruby-erb lesson);
// the shared preamble also stubs Process.pid to the fixed pid, so MRI's lines
// match the deterministic ones byte-for-byte.
func rubyEval(t *testing.T, bin, script string) string {
	t.Helper()
	preamble := fmt.Sprintf("require 'logger'\n$stdout.binmode\nmodule Process; def self.pid; %d; end; end\n", fixedPid)
	cmd := exec.Command(bin, "-e", preamble+script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return string(out)
}

// fixedTimeScript is the Ruby expression for the same fixed instant the Go tests
// use (2025-06-29T15:58:32.123456 UTC), so MRI's formatter stamps identical bytes.
const fixedTimeScript = "t = Time.at(1751212712, 123456).utc"

// TestOracleDefaultFormat checks our default-format line is byte-identical to
// MRI's Logger::Formatter#call across severities, prognames and message kinds.
func TestOracleDefaultFormat(t *testing.T) {
	bin := rubyBin(t)
	f := &Formatter{}

	cases := []struct {
		sev      Severity
		progname string
		msg      any
		rubyMsg  string // the Ruby literal for the message
	}{
		{INFO, "myapp", "hello world", `"hello world"`},
		{DEBUG, "", "msg2", `"msg2"`},
		{ERROR, "prog", "boom", `"boom"`},
		{WARN, "app", "warned", `"warned"`},
		{FATAL, "app", "fatal!", `"fatal!"`},
		{UNKNOWN, "app", "unknown msg", `"unknown msg"`},
	}
	for _, c := range cases {
		got := f.Format(SeverityLabel(c.sev), fixedTime, fixedPid, c.progname, c.msg)
		script := fmt.Sprintf(`%s
fmt = Logger::Formatter.new
print fmt.call(%q, t, %q, %s)`, fixedTimeScript, SeverityLabel(c.sev), c.progname, c.rubyMsg)
		want := rubyEval(t, bin, script)
		if got != want {
			t.Errorf("sev=%s progname=%q:\n got %q\nwant %q", SeverityLabel(c.sev), c.progname, got, want)
		}
	}
}

// TestOracleDatetimeFormat checks an overridden datetime_format matches MRI.
func TestOracleDatetimeFormat(t *testing.T) {
	bin := rubyBin(t)
	formats := []string{"%Y-%m-%d %H:%M:%S", "%H:%M:%S.%3N", "%Y/%m/%d", "%A %B %d, %Y"}
	for _, df := range formats {
		f := &Formatter{DatetimeFormat: df}
		got := f.Format("INFO", fixedTime, fixedPid, "app", "m")
		script := fmt.Sprintf(`%s
fmt = Logger::Formatter.new
fmt.datetime_format = %q
print fmt.call("INFO", t, "app", "m")`, fixedTimeScript, df)
		want := rubyEval(t, bin, script)
		if got != want {
			t.Errorf("datetime_format %q:\n got %q\nwant %q", df, got, want)
		}
	}
}

// TestOracleExceptionMsg2str checks the Exception coercion (message + class +
// backtrace) is byte-identical to MRI's msg2str.
func TestOracleExceptionMsg2str(t *testing.T) {
	bin := rubyBin(t)
	f := &Formatter{}
	exc := Exception{
		Message:   "bad arg",
		Class:     "ArgumentError",
		Backtrace: []string{"file.rb:10:in `foo`", "file.rb:20:in `bar`"},
	}
	got := f.Format("ERROR", fixedTime, fixedPid, "app", exc)
	script := fmt.Sprintf(`%s
fmt = Logger::Formatter.new
begin
  raise ArgumentError, "bad arg"
rescue => e
  e.set_backtrace([%q, %q])
  print fmt.call("ERROR", t, "app", e)
end`, fixedTimeScript, "file.rb:10:in `foo`", "file.rb:20:in `bar`")
	want := rubyEval(t, bin, script)
	if got != want {
		t.Errorf("exception:\n got %q\nwant %q", got, want)
	}
}

// TestOracleLevelGating checks that, end to end, a Logger driven through Add
// emits exactly the lines MRI's Logger emits at the same level — same gating,
// same bytes — by comparing against a real Logger writing to a StringIO.
func TestOracleLevelGating(t *testing.T) {
	bin := rubyBin(t)

	var out []string
	l := captured(&out)
	l.Level = WARN
	l.Debug("d", "app")
	l.Info("i", "app")
	l.Warn("w", "app")
	l.Error("e", "app")
	got := strings.Join(out, "")

	// MRI: same level, same calls, fixed clock via a frozen Time, fixed pid.
	script := fmt.Sprintf(`%s
require 'stringio'
io = StringIO.new
log = Logger.new(io)
log.level = Logger::WARN
# Freeze Time.now to the fixed instant so the timestamps match.
class << Time; alias_method :__now, :now; def now; Time.at(1751212712, 123456).utc; end; end
log.debug("app") { "d" }
log.info("app")  { "i" }
log.warn("app")  { "w" }
log.error("app") { "e" }
print io.string`, fixedTimeScript)
	want := rubyEval(t, bin, script)
	if got != want {
		t.Errorf("level gating:\n got %q\nwant %q", got, want)
	}
}

// TestOraclePeriodPolicy checks our rotation-period decision (next-rotate /
// previous-period-end, and thus the rotated filename suffix) matches MRI's
// Logger::Period across daily/weekly/monthly for several dates, computed in UTC
// so the result is timezone-stable on every CI lane.
func TestOraclePeriodPolicy(t *testing.T) {
	bin := rubyBin(t)
	dates := [][]int{
		{2026, 6, 24, 13, 30, 0},
		{2026, 1, 1, 0, 0, 1},
		{2026, 12, 31, 23, 59, 59},
		{2024, 2, 29, 8, 15, 0},
		{2026, 6, 28, 0, 0, 0},
	}
	periods := []Period{Daily, Weekly, Monthly}
	const layout = "2006-01-02T15:04:05"

	for _, d := range dates {
		now := time.Date(d[0], time.Month(d[1]), d[2], d[3], d[4], d[5], 0, time.UTC)
		for _, p := range periods {
			nr, err := NextRotateTime(now, p)
			if err != nil {
				t.Fatal(err)
			}
			pe, err := PreviousPeriodEnd(now, p)
			if err != nil {
				t.Fatal(err)
			}
			suffix := strftime(pe, DefaultPeriodSuffix)
			got := fmt.Sprintf("%s|%s|%s", nr.Format(layout), pe.Format(layout), suffix)

			script := fmt.Sprintf(`include Logger::Period
n = Time.utc(%d,%d,%d,%d,%d,%d)
nr = next_rotate_time(n, %q)
pe = previous_period_end(n, %q)
print "%%s|%%s|%%s" %% [nr.strftime("%%Y-%%m-%%dT%%H:%%M:%%S"), pe.strftime("%%Y-%%m-%%dT%%H:%%M:%%S"), pe.strftime("%%Y%%m%%d")]`,
				d[0], d[1], d[2], d[3], d[4], d[5], string(p), string(p))
			want := rubyEvalUTC(t, bin, script)
			if got != want {
				t.Errorf("date=%v period=%s:\n got %q\nwant %q", d, p, got, want)
			}
		}
	}
}

// rubyEvalUTC runs a Ruby script with TZ=UTC so Time.utc-based period arithmetic
// matches our UTC computation regardless of the runner's local zone.
func rubyEvalUTC(t *testing.T, bin, script string) string {
	t.Helper()
	cmd := exec.Command(bin, "-e", "require 'logger'\n$stdout.binmode\n"+script)
	cmd.Env = append(cmd.Environ(), "TZ=UTC")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return string(out)
}

// TestOracleSeverityLabel checks our SeverityLabel matches MRI's private
// format_severity for the full 0..6 range (UNKNOWN and beyond fold to "ANY").
func TestOracleSeverityLabel(t *testing.T) {
	bin := rubyBin(t)
	script := `labels = []
(0..6).each do |n|
  # Drive the private format_severity through a Logger via send.
  log = Logger.new(nil)
  labels << log.send(:format_severity, n)
end
print labels.join(",")`
	want := rubyEval(t, bin, script)
	var got []string
	for n := Severity(0); n <= 6; n++ {
		got = append(got, SeverityLabel(n))
	}
	if g := strings.Join(got, ","); g != want {
		t.Errorf("severity labels: got %q want %q", g, want)
	}
}
