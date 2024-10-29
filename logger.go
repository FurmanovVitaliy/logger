package logger

import (
	"context"
	"log/slog"
	"os"
)

const (
	defaultLevel     = LevelInfo
	defaultAddSource = true
	defaultAsJSON    = true
	defaultIsDefault = true
	defaultPrettyOut = false
)

func NewLogger(opts ...LoggerOption) *Logger {
	config := &LoggerOptions{
		Level:       defaultLevel,
		AddSource:   defaultAddSource,
		AsJSON:      defaultAsJSON,
		IsDefault:   defaultIsDefault,
		IsPrettyOut: defaultPrettyOut,
	}

	for _, opt := range opts {
		opt(config)
	}

	options := &HandlerOptions{
		AddSource: config.AddSource,
		Level:     config.Level,
	}

	var h Handler = NewTextHandler(os.Stdout, options)

	if config.IsPrettyOut {
		h = NewPrettyHandler(os.Stdout, options)
	}

	if config.AsJSON {
		h = NewJSONHandler(os.Stdout, options)
	}

	logger := New(h)

	if config.IsDefault {
		SetDefault(logger)
	}

	return logger
}

type LoggerOptions struct {
	Level       Level
	AddSource   bool
	AsJSON      bool
	IsDefault   bool
	IsPrettyOut bool
}

type LoggerOption func(*LoggerOptions)

// WithLevel logger option sets the log level, if not set, the default level is Info.
func WithLevel(level string) LoggerOption {
	return func(o *LoggerOptions) {
		var l Level
		if err := l.UnmarshalText([]byte(level)); err != nil {
			l = LevelInfo
		}

		o.Level = l
	}
}

// WithSource logger option sets the add source option, which will add source file and line number to the log record.
func WithSource(addSource bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.AddSource = addSource
	}
}

// IsJSON logger option sets the is json option, which will set JSON format for the log record.
func IsJSON(isJSON bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.AsJSON = isJSON
	}
}

// AsDefault logger option sets the set default option, which will set the created logger as default logger.
func AsDefault(setDefault bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.IsDefault = setDefault
	}
}

// IsPrettyOut logger option sets the pretty out option, which will set pretty output for the log record.
func IsPrettyOut(isPretty bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.IsPrettyOut = isPretty
	}
}

// WithAttrs returns logger with attributes.
func WithAttrs(ctx context.Context, attrs ...Attr) *Logger {
	logger := ExtractLogger(ctx)
	for _, attr := range attrs {
		logger = logger.With(attr)
	}

	return logger
}

// WithDefaultAttrs returns logger with default attributes.
func WithDefaultAttrs(logger *Logger, attrs ...Attr) *Logger {
	for _, attr := range attrs {
		logger = logger.With(attr)
	}

	return logger
}

func ExtractLogger(ctx context.Context) *Logger {
	return loggerFromContext(ctx)
}

func Default() *Logger {
	return slog.Default()
}
