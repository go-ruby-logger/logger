// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"fmt"
	"strings"
	"time"
)

// DefaultDatetimeFormat is MRI's Logger::Formatter::DatetimeFormat, the strftime
// pattern used when datetime_format is unset.
const DefaultDatetimeFormat = "%Y-%m-%dT%H:%M:%S.%6N"

// Inspector turns a non-String, non-Exception message into the text MRI's
// msg.inspect would produce. The host wires this from its own object model
// (Array#inspect, Hash#inspect, Integer#to_s, ...); the library never reaches
// into a Ruby object itself. A nil Inspector falls back to Go's %#v / fmt.Sprint
// only for the Go primitive forms a ruby-free test feeds it.
type Inspector func(msg any) string

// Exception is the message shape Logger::Formatter#msg2str special-cases. A host
// passes one of these when the logged object is a Ruby exception; the formatter
// renders "<message> (<class>)\n<backtrace>" exactly as MRI does.
type Exception struct {
	Message   string   // exception.message
	Class     string   // exception.class.to_s
	Backtrace []string // exception.backtrace (nil if the exception has none)
}

// Formatter is a pure port of MRI's Logger::Formatter. It holds only the
// optional datetime_format override and the message Inspector; the clock, pid,
// severity-label, progname and message are all supplied per call, so Format is a
// deterministic function of its inputs.
type Formatter struct {
	// DatetimeFormat overrides the strftime pattern for the timestamp; empty
	// means use DefaultDatetimeFormat (MRI's @datetime_format || DatetimeFormat).
	DatetimeFormat string
	// Inspect renders an arbitrary message value as msg.inspect would; see
	// Inspector. May be nil.
	Inspect Inspector
}

// formatString is MRI's Logger::Formatter::Format. %.1s takes the first byte of
// the severity label, %5s right-justifies the full label in five columns.
const formatString = "%.1s, [%s #%d] %5s -- %s: %s\n"

// Format renders one log line byte-for-byte as MRI's Logger::Formatter#call
// would, given the severity label, the (injected) timestamp, the (injected)
// pid, the progname and the message:
//
//	sprintf("%.1s, [%s #%d] %5s -- %s: %s\n",
//	        severity, format_datetime(time), pid, severity, progname, msg2str(msg))
func (f *Formatter) Format(severityLabel string, t time.Time, pid int, progname string, msg any) string {
	return fmt.Sprintf(formatString,
		first1(severityLabel),
		f.formatDatetime(t),
		pid,
		pad5(severityLabel),
		progname,
		f.msg2str(msg),
	)
}

// formatDatetime mirrors Logger::Formatter#format_datetime.
func (f *Formatter) formatDatetime(t time.Time) string {
	df := f.DatetimeFormat
	if df == "" {
		df = DefaultDatetimeFormat
	}
	return strftime(t, df)
}

// msg2str mirrors Logger::Formatter#msg2str: a String passes through, an
// Exception becomes "message (Class)\nbacktrace", and anything else is
// inspected.
func (f *Formatter) msg2str(msg any) string {
	switch m := msg.(type) {
	case string:
		return m
	case Exception:
		bt := ""
		if m.Backtrace != nil {
			bt = strings.Join(m.Backtrace, "\n")
		}
		return fmt.Sprintf("%s (%s)\n%s", m.Message, m.Class, bt)
	default:
		if f.Inspect != nil {
			return f.Inspect(msg)
		}
		return defaultInspect(msg)
	}
}

// defaultInspect is the ruby-free fallback used when no host Inspector is wired.
// It renders the Go primitive forms the deterministic tests feed in a way that
// matches Ruby's inspect for those values (strings are not reached here).
func defaultInspect(msg any) string {
	if msg == nil {
		return "nil"
	}
	return fmt.Sprint(msg)
}

// first1 returns the first byte of s as MRI's %.1s does ("" for an empty label).
func first1(s string) string {
	if s == "" {
		return ""
	}
	return s[:1]
}

// pad5 right-justifies s in five columns, matching sprintf("%5s", s). Labels
// longer than five characters are left intact (Ruby's %5s is a minimum width).
func pad5(s string) string {
	if len(s) >= 5 {
		return s
	}
	return strings.Repeat(" ", 5-len(s)) + s
}
