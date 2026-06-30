// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package logger is a pure-Go (CGO-free) port of the deterministic core of
// MRI 4.0.5's stdlib Logger (the logger-1.7.0 gem): the severity model, the
// default Formatter, level gating, and the LogDevice rotation *policy*.
//
// It is the compute core that go-embedded-ruby's rbgo binds: rbgo wires the IO
// sink (the $stdout / File that receives the formatted bytes), the wall clock,
// and the process id; everything that is a pure decision — which bytes a log
// line is, whether a message is gated out by the level, and whether (and to what
// filename) a log file should rotate — lives here, with no IO and no Ruby
// runtime, so it is testable byte-for-byte against the `ruby` binary.
package logger

import "time"

// Clock supplies the current instant. The default is time.Now; tests inject a
// fixed instant so the formatted lines are deterministic.
type Clock func() time.Time

// Logger is a pure port of MRI's Logger class minus the IO device. It decides
// what bytes each call would emit and whether a call is gated by the level, then
// hands the formatted line to the host Sink (rbgo's IO device). It never writes
// to a file or stream itself.
type Logger struct {
	// Level is the threshold below which Add drops a message (MRI's @level).
	Level Severity
	// Progname is the default program name (MRI's @progname).
	Progname string
	// Formatter renders each line; if nil, a zero-value default Formatter is used
	// (MRI's @formatter || @default_formatter).
	Formatter *Formatter
	// Sink receives each formatted line and each raw << write — this is the host
	// IO device. A nil Sink models MRI's @logdev.nil? (Add returns true and
	// writes nothing).
	Sink func(string)
	// Now supplies the timestamp for each line; defaults to time.Now.
	Now Clock
	// Pid supplies the process id stamped into each line; defaults to os.Getpid
	// via PidFunc. Modeled as a function so tests inject a fixed pid.
	Pid func() int
}

// New returns a Logger writing formatted lines to sink, with the default
// Formatter, DEBUG level, and the real clock/pid wired. Pass a nil sink to model
// a logger with no device.
func New(sink func(string)) *Logger {
	return &Logger{
		Level:     DEBUG,
		Formatter: &Formatter{},
		Sink:      sink,
		Now:       time.Now,
		Pid:       defaultPid,
	}
}

func (l *Logger) clock() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Logger) pid() int {
	if l.Pid != nil {
		return l.Pid()
	}
	return defaultPid()
}

func (l *Logger) formatter() *Formatter {
	if l.Formatter != nil {
		return l.Formatter
	}
	return &Formatter{}
}

// Add mirrors Logger#add. severity defaults to UNKNOWN when negative (MRI's
// `severity ||= UNKNOWN`, modeled here with the sentinel -1); when there is no
// sink (MRI's @logdev.nil?) or the severity is below the level, it formats
// nothing and returns true. Otherwise it formats the line — picking up the
// default progname when progname is empty — and writes it to the sink, then
// returns true. Add always returns true, exactly as MRI does.
func (l *Logger) Add(severity Severity, message any, progname string) bool {
	if severity < 0 {
		severity = UNKNOWN
	}
	if l.Sink == nil || severity < l.Level {
		return true
	}
	if progname == "" {
		progname = l.Progname
	}
	line := l.formatter().Format(SeverityLabel(severity), l.clock(), l.pid(), progname, message)
	l.Sink(line)
	return true
}

// Log is MRI's alias for Add (`alias log add`).
func (l *Logger) Log(severity Severity, message any, progname string) bool {
	return l.Add(severity, message, progname)
}

// Write mirrors Logger#<<: it writes msg to the sink with no formatting and
// returns the number of bytes written, or -1 when there is no sink (MRI returns
// nil; -1 is the Go-idiomatic "no device" signal).
func (l *Logger) Write(msg string) int {
	if l.Sink == nil {
		return -1
	}
	l.Sink(msg)
	return len(msg)
}

// Debug logs message at DEBUG (Logger#debug). The remaining severity helpers
// mirror their MRI counterparts.
func (l *Logger) Debug(message any, progname string) bool {
	return l.Add(DEBUG, message, progname)
}

// Info logs message at INFO (Logger#info).
func (l *Logger) Info(message any, progname string) bool {
	return l.Add(INFO, message, progname)
}

// Warn logs message at WARN (Logger#warn).
func (l *Logger) Warn(message any, progname string) bool {
	return l.Add(WARN, message, progname)
}

// Error logs message at ERROR (Logger#error).
func (l *Logger) Error(message any, progname string) bool {
	return l.Add(ERROR, message, progname)
}

// Fatal logs message at FATAL (Logger#fatal).
func (l *Logger) Fatal(message any, progname string) bool {
	return l.Add(FATAL, message, progname)
}

// Unknown logs message at UNKNOWN (Logger#unknown).
func (l *Logger) Unknown(message any, progname string) bool {
	return l.Add(UNKNOWN, message, progname)
}

// DebugQ mirrors Logger#debug?: level <= DEBUG. The predicates report whether a
// message at that severity would currently be emitted.
func (l *Logger) DebugQ() bool { return l.Level <= DEBUG }

// InfoQ mirrors Logger#info?: level <= INFO.
func (l *Logger) InfoQ() bool { return l.Level <= INFO }

// WarnQ mirrors Logger#warn?: level <= WARN.
func (l *Logger) WarnQ() bool { return l.Level <= WARN }

// ErrorQ mirrors Logger#error?: level <= ERROR.
func (l *Logger) ErrorQ() bool { return l.Level <= ERROR }

// FatalQ mirrors Logger#fatal?: level <= FATAL.
func (l *Logger) FatalQ() bool { return l.Level <= FATAL }

// SetLevel mirrors Logger#level=, coercing a string or integer like MRI's
// Severity.coerce before assigning.
func (l *Logger) SetLevel(v any) error {
	s, err := CoerceSeverity(v)
	if err != nil {
		return err
	}
	l.Level = s
	return nil
}
