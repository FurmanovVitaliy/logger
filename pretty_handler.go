package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var keyColors = []func(string) string{
	func(s string) string { return color.HiGreenString(s) },
	func(s string) string { return color.HiYellowString(s) },
	func(s string) string { return color.HiCyanString(s) },
	func(s string) string { return color.HiMagentaString(s) },
	func(s string) string { return color.HiRedString(s) },
}

type attrWithInfo struct {
	attr       slog.Attr
	extraLine  string
	firstChild bool
	neasted    bool
	lastChild  bool
}

type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}
type prettyHandler struct {
	jsonH slog.Handler
	opts  HandlerOptions
	goas  []groupOrAttrs
	mu    *sync.Mutex
	out   io.Writer

	jsonBuf             bytes.Buffer
	tWidth              int
	lWidth              int
	firstLine           bool
	lastLine            bool
	disableActiveIndent bool
	activeIndent        map[int]bool
}

func NewPrettyHandler(out io.Writer, opts *HandlerOptions) *prettyHandler {
	if opts == nil {
		opts = &HandlerOptions{}
	}

	h := &prettyHandler{
		out:          out,
		mu:           &sync.Mutex{},
		activeIndent: make(map[int]bool),
		opts:         *opts,
	}

	if h.opts.Level == nil {
		h.opts.Level = slog.LevelDebug
	}

	h.jsonH = slog.NewJSONHandler(&h.jsonBuf, &slog.HandlerOptions{
		Level: h.opts.Level,
	})

	return h
}

/*--------------------------------HANDLER---------------------------------------------------*/
func (h *prettyHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	buf := make([]byte, 0, 1024)
	var path, timestamp string
	indentLevel := 0

	// terminal width logic
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 99
	}

	h.lWidth = width + 5

	if !r.Time.IsZero() {
		timestamp = fmt.Sprintf("[%s %s]", "🕙", r.Time.Format(time.Stamp))
	}

	h.firstLine = true
	msg := slog.String(colorizeLevel(r.Level), r.Message)
	if h.lWidth < len(msg.String())+10+len(timestamp) {
		buf = h.appendAttr(buf, attrWithInfo{msg, "", false, false, false}, indentLevel)
	} else {
		buf = h.appendAttr(buf, attrWithInfo{msg, timestamp, false, false, false}, indentLevel)
	}
	h.firstLine = false

	for _, goa := range h.goas {
		if len(goa.attrs) > 0 {
			for _, a := range goa.attrs {
				buf = h.appendAttr(buf, attrWithInfo{a, "", false, false, false}, indentLevel)
			}
		}
		if goa.group != "" {
			group := slog.String("GROUP", goa.group)
			buf = h.appendAttr(buf, attrWithInfo{group, "", false, false, false}, indentLevel)
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		buf = h.appendAttr(buf, attrWithInfo{a, "", false, false, false}, indentLevel)
		return true
	})

	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		path = fmt.Sprintf("%s:%d", f.File, f.Line)
		h.lastLine = true
		source := slog.String("source", path)
		buf = h.appendAttr(buf, attrWithInfo{source, "", false, false, false}, indentLevel)
		h.lastLine = false
	}
	/*
		buf = fmt.Append(buf, "\n")
		if h.opts.Level.Level() < slog.LevelInfo && h.tWidth > 158 {
			addSysInfo(&buf)
		}
	*/
	_, err = h.out.Write(buf)
	return err
}
func (h *prettyHandler) appendAttr(buf []byte, a attrWithInfo, indentLevel int) []byte {
	a.attr.Value = a.attr.Value.Resolve()
	if a.attr.Equal(slog.Attr{}) {
		return buf
	}

	switch a.attr.Value.Kind() {
	case slog.KindString:
		groupLine := false
		if a.attr.Key == "GROUP" {
			groupLine = true
		}

		key := colorizeKey(indentLevel, a.attr.Key)
		str := fmt.Sprintf("%s: %q", key, a.attr.Value.String())

		switch true {
		case h.firstLine:
			str = fmt.Sprintf("[%s: %q]", a.attr.Key, a.attr.Value.String())
			str = h.alignValues(str, indentLevel, '-', '─', false, a, false, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "╭")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "╮")
		case groupLine:
			str = fmt.Sprintf("[%s %s: %q]", "📂", color.HiWhiteString(a.attr.Key), a.attr.Value.String())
			str = h.alignValues(str, indentLevel, ' ', '─', false, a, false, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "├")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		case h.lastLine:

			str = fmt.Sprintf("[%s: %q] ", a.attr.Key, a.attr.Value.String())
			str = h.alignValues(str, indentLevel, '╴', '─', true, a, false, false)
			buf = fmt.Append(buf, "╰")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "╯")
		case a.firstChild:
			if h.isLongValue(str, indentLevel) {
				h.wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = fmt.Sprintf("%s: %q", key, a.attr.Value.String())
				str = h.alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		case a.neasted:

			if h.isLongValue(str, indentLevel) {
				h.wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = h.alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		case a.lastChild:
			if h.isLongValue(str, indentLevel) {
				h.wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = h.alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		default:
			if h.isLongValue(str, indentLevel) {
				h.wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {

				str = fmt.Sprintf("[%s: %q] ", key, a.attr.Value.String())
				str = h.alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "├")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		}
		buf = fmt.Append(buf, "\n")
	case slog.KindGroup:
		attrs := a.attr.Value.Group()
		if len(attrs) == 0 {
			return buf
		}
		str := fmt.Sprintf("%s%s:", "📦 ", colorizeKey(indentLevel, a.attr.Key))
		str = h.alignValues(str, indentLevel, ' ', '─', false, a, true, false)
		str = appendInRight(str, a.extraLine)
		buf = fmt.Append(buf, "│", str, "│\n")
		indentLevel++
		for i, ga := range attrs {
			isFirst := i == 0
			isLast := i == len(attrs)-1
			buf = h.appendAttr(buf, attrWithInfo{ga, "", isFirst, true, isLast}, indentLevel)
		}
	default:
		buf = h.appendAttr(buf, attrWithInfo{slog.String(a.attr.Key, a.attr.Value.String()), "", a.firstChild, a.neasted, a.lastChild}, indentLevel)
	}
	return buf
}

func (h *prettyHandler) isLongValue(str string, identLevel int) bool {
	return len(str) > h.lWidth-identLevel*6
}

/*--------------------------------UTILS------------------------------------------------------*/

func (h *prettyHandler) alignValues(text string, identLevel int, spacer, ident rune, center bool, a attrWithInfo, key, wraped bool) string {
	prefix := strings.Repeat(" ", identLevel*6)

	if identLevel == 1 && a.lastChild {
		h.disableActiveIndent = true
	}
	if identLevel == 1 && a.firstChild {
		h.disableActiveIndent = false
	}
	if identLevel > 1 {
		prefix = h.addIndentSpaces(identLevel)
	}

	if !key && a.lastChild && a.neasted {
		h.activeIndent[identLevel] = false
	}
	if key && !a.lastChild && a.neasted {
		h.activeIndent[identLevel] = true
	}

	if !h.firstLine && !h.lastLine {
		if key && !a.neasted {
			text = "╼ " + text
		} else if key {
			text = "╼ " + text
		} else if wraped {
			text = "  " + text
		} else {
			text = "╼ " + text
		}
	} else if h.firstLine {
		text = string(ident) + string(ident) + text
	}

	switch true {
	case wraped && identLevel > 0 && a.lastChild:
		prefix = prefix + "    "
		text = prefix + text
	case wraped && identLevel > 0 && !a.lastChild:
		prefix = prefix + "┃   "
		text = prefix + text
	case a.firstChild && a.lastChild:
		prefix = prefix + "┗━━━"
		text = prefix + text
	case a.firstChild:
		prefix = prefix + "┣━━━"
		text = prefix + text
	case a.lastChild:
		prefix = prefix + "┗━━━"
		text = prefix + text
	case a.neasted:
		prefix = prefix + "┣━━━"
		text = prefix + text
	default:
		if identLevel > 1 {
			prefix = runewidth.FillLeft("┣━━━", identLevel*6)
			text = prefix + text
		} else {
			text = strings.Repeat(string(ident), identLevel) + text
		}
	}
	if identLevel > 0 {
		text = text[4:]
	}

	paddingL := 0
	textLength := runewidth.StringWidth(text)

	if textLength%2 != 0 {
		text = text + "\u202f"
	}
	paddingL = (h.lWidth - textLength) / 2
	if paddingL < 0 {
		paddingL = 0
	}
	padding := strings.Repeat(string(spacer), paddingL)
	if center && !h.lastLine {

		return padding + text + padding

	}
	if h.firstLine {
		r := runewidth.FillRight(text, h.lWidth)
		r = strings.ReplaceAll(r, string(' '), string('-'))
		r = text + r[len(text):]
		return r
	}
	if h.lastLine {
		text = fmt.Sprintf("[%s: %s]", "SOURCE", a.attr.Value.String())

		if h.lWidth-7 > runewidth.StringWidth(text) {
			text1 := fmt.Sprintf("[%s: %s]", color.HiWhiteString("SOURCE"), a.attr.Value.String())
			r := strings.Repeat("-", h.lWidth-7)
			center := (len(r) - runewidth.StringWidth(text)) / 2
			result := r[:center] + text1 + r[center+runewidth.StringWidth(text):]
			return result
		} else {
			return a.attr.Value.String()
		}
	}
	return runewidth.FillRight(text, h.lWidth)
}

func (h *prettyHandler) addIndentSpaces(parts int) string {
	result := ""
	for i := range parts {
		part := runewidth.FillRight("", 6)
		if h.activeIndent[i] && !h.disableActiveIndent {
			part = runewidth.FillRight("┃", 6)
		}
		result += part
	}
	return result
}

func appendInRight(target, text string) string {
	tl := runewidth.StringWidth(text)
	if len(target) < len(text)+5 || tl == 0 {
		return target
	}
	modifiablePart := target[:len(target)-tl-5]
	lastFive := target[len(target)-5:]
	return modifiablePart + text + lastFive
}
func centerString(str string, width int) string {
	strLen := runewidth.StringWidth(str)
	if strLen >= width {
		return str
	}
	empty := width - strLen
	space := empty / 2
	remaider := empty % 2
	if remaider == 0 {
		return strings.Repeat(string(' '), space) + str + strings.Repeat(string(' '), space)
	}
	return strings.Repeat(string(' '), space) + str + strings.Repeat(string(' '), space+1)
}
func (h *prettyHandler) wrapLongValue(buf *[]byte, a attrWithInfo, key, value string, lvl int) {
	vMaxL := h.lWidth - len(a.attr.Key) - 16 - lvl*6
	if vMaxL%2 != 0 {
		vMaxL = vMaxL - 1
	}

	vals := splitText(value, vMaxL)
	keyL := len(a.attr.Key)
	sameSymbol := centerString("\u2e17", keyL)
	space := max(keyL-runewidth.StringWidth(sameSymbol), 0)
	key = strings.Repeat(" ", space) + key

	for i, v := range vals {
		var str string
		if i == 0 {
			str = fmt.Sprintf("%s:%q", key, v)
		} else {
			str = fmt.Sprintf("%s:%q", colorizeKey(lvl, sameSymbol), v)
		}

		str = h.alignValues(str, lvl, ' ', '─', false, a, false, i > 0)

		*buf = fmt.Append(*buf, "│", str, "│")
		if i < len(vals)-1 {
			*buf = fmt.Append(*buf, "\n")
		}
	}
}

func splitText(text string, maxLength int) []string {
	var result []string
	index := 0
	str := []byte(text)

	for index < len(str) {
		next := cutString(str, maxLength, index)
		result = append(result, string(str[index:next]))
		index = next
	}

	return result
}

func cutString(buf []byte, max, index int) int {
	if max <= 0 || index >= len(buf) {
		return len(buf)
	}
	end := index + max
	if end >= len(buf) {
		return len(buf)
	}

	delimiters := []byte{' ', '/', '.', ',', '\n'}
	bestIndex := -1
	for _, delim := range delimiters {
		if idx := bytes.LastIndexByte(buf[index:end], delim); idx > bestIndex {
			bestIndex = idx
		}
	}

	if bestIndex == -1 {
		return end
	}
	return index + bestIndex + 1
}
func colorizeLevel(level slog.Level) string {
	colorMap := map[slog.Level]string{
		slog.LevelDebug: color.HiMagentaString("🔧 " + level.String()),
		slog.LevelInfo:  color.HiBlueString("🌐 " + level.String()),
		slog.LevelWarn:  color.HiYellowString("⚠️  " + level.String()),
		slog.LevelError: color.HiRedString("🛑 " + level.String()),
	}
	return colorMap[level]
}

func colorizeKey(indentLevel int, key string) string {
	idx := indentLevel % len(keyColors)
	return keyColors[idx](key)
}

/*--------------------------------slog methods-----------------------------------------------*/

func (h *prettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
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

func (h *prettyHandler) withGroupOrAttrs(goa groupOrAttrs) *prettyHandler {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}
