package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

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

type logger struct {
	debugLogger *log.Logger
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	debug       bool
	mu          sync.RWMutex
}

func New() Logger {
	debug := os.Getenv("DEBUG") == "true" || os.Getenv("LOG_LEVEL") == "debug"
	
	return &logger{
		debugLogger: log.New(os.Stdout, "[DEBUG] ", log.LstdFlags|log.Lshortfile),
		infoLogger:  log.New(os.Stdout, "[INFO] ", log.LstdFlags),
		warnLogger:  log.New(os.Stdout, "[WARN] ", log.LstdFlags),
		errorLogger: log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile),
		debug:       debug,
	}
}

func (l *logger) Debug(v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.debug {
		l.debugLogger.Output(2, fmt.Sprint(v...))
	}
}

func (l *logger) Debugf(format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.debug {
		l.debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *logger) Info(v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.infoLogger.Output(2, fmt.Sprint(v...))
}

func (l *logger) Infof(format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.infoLogger.Output(2, fmt.Sprintf(format, v...))
}

func (l *logger) Warn(v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.warnLogger.Output(2, fmt.Sprint(v...))
}

func (l *logger) Warnf(format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.warnLogger.Output(2, fmt.Sprintf(format, v...))
}

func (l *logger) Error(v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.errorLogger.Output(2, fmt.Sprint(v...))
}

func (l *logger) Errorf(format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.errorLogger.Output(2, fmt.Sprintf(format, v...))
}

func (l *logger) Fatal(v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.errorLogger.Output(2, fmt.Sprint(v...))
	os.Exit(1)
}

func (l *logger) Fatalf(format string, v ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.errorLogger.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}