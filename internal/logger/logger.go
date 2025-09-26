package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// LogLevel represents the logging level
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// New creates a new logger instance with the specified level
func New(level LogLevel) *Logger {
	var slogLevel slog.Level
	
	switch strings.ToUpper(string(level)) {
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "INFO":
		slogLevel = slog.LevelInfo
	case "WARN":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelDebug // Default to debug in development
	}

	opts := &slog.HandlerOptions{
		Level: slogLevel,
	}

	// Send logs to stderr so TUI rendering on stdout is not disrupted
	handler := slog.NewTextHandler(os.Stderr, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}
}

// NewWithFile creates a logger that can write to a file when quiet mode is enabled
func NewWithFile(level LogLevel, quiet bool, logFile string) (*Logger, error) {
	var slogLevel slog.Level
	
	switch strings.ToUpper(string(level)) {
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "INFO":
		slogLevel = slog.LevelInfo
	case "WARN":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: slogLevel,
	}

	var writer io.Writer
	var logFileName string
	
	if quiet {
		// Generate log file name if not provided
		if logFile == "" {
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			logFileName = fmt.Sprintf("localchat_%s.log", timestamp)
		} else {
			logFileName = logFile
		}
		
		// Ensure log directory exists
		if dir := filepath.Dir(logFileName); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create log directory: %w", err)
			}
		}
		
		// Open log file
		file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", logFileName, err)
		}
		
		writer = file
		fmt.Printf("📝 Logs will be written to: %s\n", logFileName)
	} else {
		// Send logs to stderr so TUI rendering on stdout is not disrupted
		writer = os.Stderr
	}

	handler := slog.NewTextHandler(writer, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}, nil
}

// NewFromEnv creates a logger using the LOG_LEVEL environment variable
func NewFromEnv() *Logger {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "DEBUG" // Default to debug mode
	}
	return New(LogLevel(level))
}

// WithComponent adds a component field to all log messages
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

// WithUser adds a user field to all log messages
func (l *Logger) WithUser(userID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("user", userID),
	}
}

// WithPeer adds a peer field to all log messages
func (l *Logger) WithPeer(peerID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("peer", peerID),
	}
}
