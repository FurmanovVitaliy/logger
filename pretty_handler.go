package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"time"

	stdLog "log"

	"github.com/fatih/color"
)

type PrettyHandler struct {
	opts *slog.HandlerOptions
	slog.Handler
	l     *stdLog.Logger
	attrs []slog.Attr
}

func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	return &PrettyHandler{
		opts:    opts,
		Handler: slog.NewJSONHandler(w, opts),
		l:       stdLog.New(w, "", 0),
	}
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	level := h.colorizeLevel(r.Level)
	file, line := h.getSourceInfo(r)

	fields, err := h.getFormattedFields(r)
	if err != nil {
		return err
	}

	timeStr := h.formatTime(r.Time)
	msg := color.CyanString(r.Message)

	h.printLog(timeStr, level, msg, fields, file, line)
	return nil
}

func (h *PrettyHandler) colorizeLevel(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return color.MagentaString(level.String() + ":")
	case slog.LevelInfo:
		return color.BlueString(level.String() + ":")
	case slog.LevelWarn:
		return color.YellowString(level.String() + ":")
	case slog.LevelError:
		return color.RedString(level.String() + ":")
	default:
		return level.String() + ":"
	}
}

func (h *PrettyHandler) getSourceInfo(r slog.Record) (string, int) {
	if !h.opts.AddSource || r.PC == 0 {
		return "", 0
	}
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()
	return frame.File, frame.Line
}

func (h *PrettyHandler) getFormattedFields(r slog.Record) (string, error) {
	fields := make(map[string]interface{}, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})
	for _, a := range h.attrs {
		fields[a.Key] = a.Value.Any()
	}

	if len(fields) == 0 {
		return "", nil
	}
	b, err := json.MarshalIndent(fields, "", "  ")
	return string(b), err
}

func (h *PrettyHandler) formatTime(t time.Time) string {
	return "[" + t.Format(time.Kitchen) + "]"
}

func (h *PrettyHandler) printLog(timeStr, level, msg, fields, file string, line int) {
	if file != "" {
		h.l.Println(
			timeStr,
			level,
			msg,
			color.WhiteString(fields),

			color.WhiteString(fmt.Sprintf("\n[ORIGIN ] %s:%d\n", file, line)),
		)
	} else {
		h.l.Println(timeStr, level, msg, color.WhiteString(fields))
	}
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler,
		l:       h.l,
		attrs:   attrs,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler.WithGroup(name),
		l:       h.l,
	}
}
