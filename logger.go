package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger

func InitializeLogger() {
	Logger = logrus.New()

	// Get log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	fmt.Printf("[LOGGER INIT] LOG_LEVEL env = \"%s\", using logLevel = \"%s\"\n", 
		os.Getenv("LOG_LEVEL"), strings.ToLower(logLevel))

	// Set log level
	switch strings.ToLower(logLevel) {
	case "debug":
		Logger.SetLevel(logrus.DebugLevel)
	case "info":
		Logger.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		Logger.SetLevel(logrus.WarnLevel)
	case "error":
		Logger.SetLevel(logrus.ErrorLevel)
	default:
		Logger.SetLevel(logrus.InfoLevel)
	}

	// Set formatter
	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	// Set output to stdout
	Logger.SetOutput(os.Stdout)
}