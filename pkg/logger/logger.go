// Package logger provides a simple logging interface and implementation
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Logger defines the logging interface
type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
	Info(v ...interface{})
	Infof(format string, v ...interface{})
	Warn(v ...interface{})
	Warnf(format string, v ...interface{})
	Error(v ...interface{})
	Errorf(format string, v ...interface{})
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
}

// Level represents logging levels
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// logger implements the Logger interface
type logger struct {
	level   Level
	loggers map[Level]*log.Logger
	mu      sync.RWMutex
}

// New creates a new logger instance
func New() Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	
	return &logger{
		level: level,
		loggers: map[Level]*log.Logger{
			LevelDebug: log.New(os.Stdout, "[DEBUG] ", log.LstdFlags|log.Lshortfile),
			LevelInfo:  log.New(os.Stdout, "[INFO] ", log.LstdFlags),
			LevelWarn:  log.New(os.Stdout, "[WARN] ", log.LstdFlags),
			LevelError: log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile),
		},
	}
}

// parseLevel converts string log level to Level type
func parseLevel(levelStr string) Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// shouldLog checks if a message should be logged at given level
func (l *logger) shouldLog(level Level) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// output logs a message at the specified level
func (l *logger) output(level Level, v ...interface{}) {
	if !l.shouldLog(level) {
		return
	}
	
	l.mu.RLock()
	logger := l.loggers[level]
	l.mu.RUnlock()
	
	logger.Output(3, fmt.Sprint(v...))
}

// outputf logs a formatted message at the specified level
func (l *logger) outputf(level Level, format string, v ...interface{}) {
	if !l.shouldLog(level) {
		return
	}
	
	l.mu.RLock()
	logger := l.loggers[level]
	l.mu.RUnlock()
	
	logger.Output(3, fmt.Sprintf(format, v...))
}

// Debug logs a debug message
func (l *logger) Debug(v ...interface{}) {
	l.output(LevelDebug, v...)
}

// Debugf logs a formatted debug message
func (l *logger) Debugf(format string, v ...interface{}) {
	l.outputf(LevelDebug, format, v...)
}

// Info logs an info message
func (l *logger) Info(v ...interface{}) {
	l.output(LevelInfo, v...)
}

// Infof logs a formatted info message
func (l *logger) Infof(format string, v ...interface{}) {
	l.outputf(LevelInfo, format, v...)
}

// Warn logs a warning message
func (l *logger) Warn(v ...interface{}) {
	l.output(LevelWarn, v...)
}

// Warnf logs a formatted warning message
func (l *logger) Warnf(format string, v ...interface{}) {
	l.outputf(LevelWarn, format, v...)
}

// Error logs an error message
func (l *logger) Error(v ...interface{}) {
	l.output(LevelError, v...)
}

// Errorf logs a formatted error message
func (l *logger) Errorf(format string, v ...interface{}) {
	l.outputf(LevelError, format, v...)
}

// Fatal logs an error message and exits
func (l *logger) Fatal(v ...interface{}) {
	l.output(LevelError, v...)
	os.Exit(1)
}

// Fatalf logs a formatted error message and exits
func (l *logger) Fatalf(format string, v ...interface{}) {
	l.outputf(LevelError, format, v...)
	os.Exit(1)
}