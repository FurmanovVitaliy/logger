package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

type prettyHandler struct {
	opts slog.HandlerOptions
	goas []groupOrAttrs
	mu   *sync.Mutex
	out  io.Writer
}

func NewPrettyHandler(out io.Writer, opts *slog.HandlerOptions) *prettyHandler {
	h := &prettyHandler{out: out, mu: &sync.Mutex{}}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = slog.LevelInfo
	}
	return h
}

func (h *prettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *prettyHandler) Handle(ctx context.Context, r slog.Record) error {
	maxKeyL := h.maxKeyLength(r)

	isTime := r.Time.IsZero()
	group := len(h.goas) > 0 && r.NumAttrs() > 0
	args := r.NumAttrs() > 0
	source := h.opts.AddSource && r.PC != 0
	path := ""
	if source {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		path = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	sl := len(path) + 13

	buf := make([]byte, 0, 1024)
	if group || args || source {
		buf = fmt.Append(buf, "╭")
	} else {
		buf = fmt.Append(buf, "  ")
	}

	if !isTime {
		buf = fmt.Appendf(buf, "[ %s ", color.HiWhiteString(r.Time.Format(time.Kitchen)))
		buf = fmt.Appendf(buf, "%s", colorizeLevel(r.Level))

	} else {
		buf = fmt.Appendf(buf, "[%s", colorizeLevel(r.Level))
	}
	buf = fmt.Appendf(buf, " %s ]", color.HiWhiteString(r.Message))

	addLeftLine(&buf, sl, 31, "╴", "╮")
	buf = fmt.Append(buf, "\n")

	indentLevel := 0
	firstAttr := true

	// Handle state from WithGroup and WithAttrs.
	goas := h.goas
	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list; they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}

	for i := 0; i < len(goas); i++ {
		if goas[i].group != "" {
			buf = fmt.Appendf(buf, "├[ %*s%s ", indentLevel*4, color.HiWhiteString(addKeySpace("GROUP", maxKeyL)+":"), goas[i].group)
			addLeftLine(&buf, sl, 14, " ", "╷")
			buf = fmt.Append(buf, "\n")
		} else if len(goas[i].attrs) == 1 {
			firstAttr = true
			indentLevel = 1
			buf = h.appendAttr(buf, goas[i].attrs[0], indentLevel, &firstAttr, sl, maxKeyL)
			buf = buf[:len(buf)-1]

			addLeftLine(&buf, sl, 14, " ", "╷")
			buf = append(buf, '\n')

		} else {
			for _, a := range goas[i].attrs {
				buf = h.appendAttr(buf, a, indentLevel, &firstAttr, sl, maxKeyL)
			}
		}

	}

	r.Attrs(func(a slog.Attr) bool {
		firstAttr = true
		buf = h.appendAttr(buf, a, indentLevel, &firstAttr, sl, maxKeyL)
		buf = buf[:len(buf)-1]
		addLeftLine(&buf, sl, 14, " ", "╷")
		buf = append(buf, '\n')
		return true

	})
	if source {
		maxKeyL = 0
		buf = fmt.Append(buf, "╰[")
		t := slog.String(color.HiWhiteString(" SOURCE"), path)
		buf = h.appendAttr(buf, t, 0, &firstAttr, sl, maxKeyL)
		buf = buf[:len(buf)-1]
		buf = fmt.Append(buf, " ]╯")
		buf = fmt.Append(buf, "\n")
	}
	buf = fmt.Append(buf, "\n")
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
	return err

}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}

func (h *prettyHandler) appendAttr(buf []byte, a slog.Attr, indentLevel int, firstAttr *bool, sl, max int) []byte {
	if *firstAttr && !strings.EqualFold(a.Key, color.HiWhiteString(" SOURCE")) {
		buf = fmt.Append(buf, "├{ ")
		*firstAttr = false

	} else if !strings.EqualFold(a.Key, color.HiWhiteString(" SOURCE")) {

		buf = fmt.Append(buf, "│ ")
	}
	a.Value = a.Value.Resolve()

	// Ignore empty Attrs.
	if a.Equal(slog.Attr{}) {
		return buf
	}
	// Indent 4 spaces per level.
	if indentLevel > 1 {
		buf = fmt.Appendf(buf, "%*s", indentLevel*4, "")
	} else {
		buf = fmt.Appendf(buf, "%*s", indentLevel*0, "")
	}

	switch a.Value.Kind() {
	case slog.KindAny:
		buf = fmt.Appendf(buf, "%s: %q\n", colorizeKey(indentLevel, addKeySpace(a.Key, max)), a.Value.String())
	case slog.KindString:
		// Quote string values, to make them easy to parse.
		buf = fmt.Appendf(buf, "%s: %q", colorizeKey(indentLevel, addKeySpace(a.Key, max)), a.Value.String())
		buf = fmt.Append(buf, "\n")
	case slog.KindGroup:
		attrs := a.Value.Group()
		// Ignore empty groups.
		if len(attrs) == 0 {
			return buf
		}

		// If the key is non-empty, write it out and indent the rest of the attrs.
		// Otherwise, inline the attrs.
		if a.Key != "" {
			buf = fmt.Appendf(buf, "%s:", colorizeKey(indentLevel, addKeySpace(a.Key, max)))
			addLeftLine(&buf, sl, 14, " ", "╷")
			buf = fmt.Append(buf, "\n")
			indentLevel++
			max = 0
			for _, a := range attrs {

				if max < len(a.Key) {

					max = len(a.Key)
				}

			}

		}
		for i, ga := range attrs {
			buf = h.appendAttr(buf, ga, indentLevel, firstAttr, sl, max)
			if i < len(attrs)-1 {
				buf = buf[:len(buf)-1]
				addLeftLine(&buf, sl, 14, " ", "╷")
				buf = append(buf, '\n')
			}
		}
	default:
		buf = fmt.Appendf(buf, "%s: %s", colorizeKey(indentLevel, addKeySpace(a.Key, max)), a.Value)

		buf = fmt.Append(buf, "\n")

	}
	return buf
}

func (h *prettyHandler) withGroupOrAttrs(goa groupOrAttrs) *prettyHandler {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}

func colorizeLevel(level slog.Level) string {
	colorMap := map[slog.Level]string{
		slog.LevelDebug: color.HiMagentaString(level.String() + ":"),
		slog.LevelInfo:  color.HiBlueString(level.String() + ":"),
		slog.LevelWarn:  color.HiYellowString(level.String() + ":"),
		slog.LevelError: color.HiRedString(level.String() + ":"),
	}
	return colorMap[level] // Default case returns white
}

func colorizeKey(identlevel int, key string) string {
	switch identlevel {
	case 0:
		return color.HiGreenString(key)
	case 1:
		return color.HiGreenString(key)
	case 2:
		return color.HiYellowString(key)
	case 3:
		return color.HiCyanString(key)
	case 4:
		return color.HiMagentaString(key)
	case 5:
		return color.HiCyanString(key)
	case 6:
		return color.HiRedString(key)
	case 7:
		return color.HiBlackString(key)
	default:
		return color.HiWhiteString(key)

	}
}

func charsAfterLastNewline(buf []byte) int {
	lastNewline := -1
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] == '\n' {
			lastNewline = i
			break
		}
	}
	// Если найден '\n', возвращаем количество символов после него
	if lastNewline != -1 {
		return len(buf) - lastNewline
	}
	// Если '\n' не найден, возвращаем длину всего буфера
	return len(buf)
}

func addLeftLine(buf *[]byte, sl, ofset int, spacer, edge string) {
	i := sl - charsAfterLastNewline(*buf) + ofset
	if i > 0 {
		*buf = append(*buf, strings.Repeat(spacer, i)...)
		*buf = fmt.Append(*buf, edge)
	}

}

func (h *prettyHandler) maxKeyLength(r slog.Record) int {
	maxKeyL := 0
	r.Attrs(func(a slog.Attr) bool {

		if maxKeyL < len(a.Key) {
			maxKeyL = len(a.Key)
		}
		return true
	})
	for _, goa := range h.goas {
		for _, a := range goa.attrs {
			if maxKeyL < len(a.Key) {
				maxKeyL = len(a.Key)
			}
		}
	}
	return maxKeyL
}

func addKeySpace(key string, max int) string {
	if len(key) < max {
		if max-len(key) > 5 {
			return key + strings.Repeat(".", max-len(key))
		} else {
			return key + strings.Repeat(".", max-len(key))
		}
	}
	return key
}
