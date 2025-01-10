package vod

import (
	"fmt"
	"log"
)

// Logger is an interface for logging messages.
type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})

	Info(args ...interface{})
	Infof(format string, args ...interface{})

	Error(args ...interface{})
	Errorf(format string, args ...interface{})

	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
}

type stdlog string

func (s stdlog) Debug(args ...interface{}) {
	log.Println("[DEBUG]", fmt.Sprint(args...))
}

func (s stdlog) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

func (s stdlog) Info(args ...interface{}) {
	log.Println("[INFO]", fmt.Sprint(args...))
}

func (s stdlog) Infof(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (s stdlog) Error(args ...interface{}) {
	log.Println("[ERROR]", fmt.Sprint(args...))
}

func (s stdlog) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func (s stdlog) Warn(args ...interface{}) {
	log.Println("[WARN]", fmt.Sprint(args...))
}

func (s stdlog) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

type emptyLogger string

func (e emptyLogger) Debug(args ...interface{}) {
}

func (e emptyLogger) Debugf(format string, args ...interface{}) {
}

func (e emptyLogger) Info(args ...interface{}) {
}

func (e emptyLogger) Infof(format string, args ...interface{}) {
}

func (e emptyLogger) Error(args ...interface{}) {
}

func (e emptyLogger) Errorf(format string, args ...interface{}) {
}

func (e emptyLogger) Warn(args ...interface{}) {
}

func (e emptyLogger) Warnf(format string, args ...interface{}) {
}

// NewSTDLogger NewLogger returns a new logger with default std log.
func NewSTDLogger() Logger {
	return stdlog("stdlog")
}

// NewEmptyLogger returns a new logger with empty log.
func NewEmptyLogger() Logger {
	return emptyLogger("empty")
}
