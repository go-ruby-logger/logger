// Copyright (c) the go-ruby-logger/logger authors
//
// SPDX-License-Identifier: BSD-3-Clause

package logger

import (
	"fmt"
	"strings"
)

// Severity is a log level, matching MRI's Logger::Severity numeric model.
//
//	DEBUG=0 INFO=1 WARN=2 ERROR=3 FATAL=4 UNKNOWN=5
//
// A Severity is just an int; the named constants below mirror the Ruby ones so
// a host can pass MRI's integers straight through.
type Severity int

// The MRI Logger::Severity constants (logger/severity.rb).
const (
	DEBUG   Severity = 0 // Low-level information, mostly for developers.
	INFO    Severity = 1 // Generic (useful) information about system operation.
	WARN    Severity = 2 // A warning.
	ERROR   Severity = 3 // A handleable error condition.
	FATAL   Severity = 4 // An unhandleable error that results in a program crash.
	UNKNOWN Severity = 5 // An unknown message that should always be logged.
)

// sevLabels mirrors MRI's private SEV_LABEL = %w(DEBUG INFO WARN ERROR FATAL ANY).
// Index 5 (UNKNOWN) and any out-of-range severity map to "ANY".
var sevLabels = [...]string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL", "ANY"}

// SeverityLabel returns the textual label MRI uses for a severity, matching
// Logger#format_severity: SEV_LABEL[severity] || 'ANY'. Any value outside
// 0..5 yields "ANY".
func SeverityLabel(s Severity) string {
	if s >= 0 && int(s) < len(sevLabels) {
		return sevLabels[s]
	}
	return "ANY"
}

// levels mirrors MRI's private Severity::LEVELS map (logger/severity.rb).
var levels = map[string]Severity{
	"debug":   DEBUG,
	"info":    INFO,
	"warn":    WARN,
	"error":   ERROR,
	"fatal":   FATAL,
	"unknown": UNKNOWN,
}

// CoerceSeverity mirrors Logger::Severity.coerce: an Integer passes through
// unchanged; a string (case-insensitively) maps via LEVELS, and anything else
// is an error ("invalid log level: ..."). It accepts an int or a string, the
// two forms a host level= would hand through.
func CoerceSeverity(v any) (Severity, error) {
	switch x := v.(type) {
	case Severity:
		return x, nil
	case int:
		return Severity(x), nil
	case string:
		if s, ok := levels[strings.ToLower(x)]; ok {
			return s, nil
		}
		return 0, fmt.Errorf("invalid log level: %s", x)
	default:
		return 0, fmt.Errorf("invalid log level: %v", v)
	}
}
