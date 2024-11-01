package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"time"

	stdLog "log"

	"github.com/fatih/color"
)

type PrettyHandler struct {
	opts *slog.HandlerOptions
	slog.Handler
	l      *stdLog.Logger
	attrs  []slog.Attr
	groups []string
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
	colorMap := map[slog.Level]string{
		slog.LevelDebug: color.HiMagentaString(level.String() + ":"),
		slog.LevelInfo:  color.HiBlueString(level.String() + ":"),
		slog.LevelWarn:  color.HiYellowString(level.String() + ":"),
		slog.LevelError: color.HiRedString(level.String() + ":"),
	}
	return colorMap[level] // Default case returns white
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
	fields := make(map[string]interface{})

	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = extractValue(a.Value)
		return true
	})

	for _, a := range h.attrs {
		fields[a.Key] = extractValue(a.Value)
	}

	if len(fields) == 0 {
		return "", nil
	}
	b, err := json.MarshalIndent(fields, "", "  ")
	return string(b), err
}

func extractValue(val slog.Value) interface{} {
	switch v := val.Any().(type) {
	case string, int, int64, float64, bool:
		return v
	case fmt.Stringer:
		return v.String()
	case []slog.Attr:
		return extractSubAttrs(v)
	default:
		return val.Any()
	}
}

func extractSubAttrs(attrs []slog.Attr) map[string]interface{} {
	mapValue := make(map[string]interface{})
	for _, subAttr := range attrs {
		mapValue[subAttr.Key] = extractValue(subAttr.Value)
	}
	return mapValue
}

func (h *PrettyHandler) formatTime(t time.Time) string {
	return "┌╴[" + t.Format(time.Kitchen) + "]"
}

func colorizeJson(data map[string]interface{}, level int) string {
	var buffer bytes.Buffer
	colorFunc := getColorFunc(level)

	indent := "   "
	for key, value := range data {
		buffer.WriteString(fmt.Sprintf("%s%s", repeat(indent, level), colorFunc(key)))
		if nestedMap, ok := value.(map[string]interface{}); ok {
			buffer.WriteString("{\n")
			buffer.WriteString(colorizeJson(nestedMap, level+1))
			buffer.WriteString(fmt.Sprintf("%s}\n", repeat(indent, level)))
		} else {
			valStr, _ := json.Marshal(value)
			buffer.WriteString(color.HiWhiteString(":%s\n", valStr))
		}
	}
	return buffer.String()
}

func getColorFunc(level int) func(string, ...interface{}) string {
	switch level {
	case 0:
		return color.HiMagentaString
	case 1:
		return color.HiRedString
	case 2:
		return color.HiYellowString
	case 3:
		return color.HiGreenString
	case 4:
		return color.HiBlueString
	case 5:
		return color.HiCyanString
	default:
		return color.WhiteString
	}
}

func repeat(s string, count int) string {
	return fmt.Sprintf("│%s", strings.Repeat(s, count))
}

func (h *PrettyHandler) printLog(timeStr, level, msg, fields, file string, line int) {
	var jsonData map[string]interface{}
	json.Unmarshal([]byte(fields), &jsonData)
	json := colorizeJson(jsonData, 0)

	if file != "" {
		h.l.Println(timeStr, level, msg, h.formatOrigin(json, file, line))
	} else {
		h.l.Println(timeStr, level, msg, fields)
	}
}

func (h *PrettyHandler) formatOrigin(json string, file string, line int) string {
	if len(json) > 0 {
		return fmt.Sprintf("\n├╴[%s ", json[3:]) + color.WhiteString(fmt.Sprintf("\r└╴[ORIGIN] %s:%d\n", file, line))
	}
	return color.WhiteString(fmt.Sprintf("\n└╴[ORIGIN ] %s:%d\n", file, line))
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler,
		l:       h.l,
		attrs:   append(h.attrs, attrs...),
		groups:  h.groups,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{
		Handler: h.Handler.WithGroup(name),
		l:       h.l,
		attrs:   h.attrs,
		groups:  append(h.groups, name),
	}
}
