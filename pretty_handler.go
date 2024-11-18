package logger

import (
	"bytes"
	"context"
	"encoding/json"
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

var (
	tWidth       int
	lWidth       int
	sWidth       int
	jsonBuf      bytes.Buffer
	firstLine    bool
	lastLine     bool
	activeIndent = make(map[int]bool)
)

type attrWithInfo struct {
	attr       slog.Attr
	extraLine  string
	firstChild bool
	innerChild bool
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
}

func NewPrettyHandler(out io.Writer, opts *HandlerOptions) *prettyHandler {
	h := &prettyHandler{out: out, mu: &sync.Mutex{}}
	h.jsonH = NewJSONHandler(&jsonBuf, &HandlerOptions{})
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = LevelDebug
	}
	return h
}

/*--------------------------------HANDLER---------------------------------------------------*/
func (h *prettyHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := make([]byte, 0, 1024)
	var (
		path        string
		timestamp   string
		indentLevel int
	)
	timeExist := !r.Time.IsZero()
	sourceExist := h.opts.AddSource && r.PC != 0

	// Defining width of log width and system info
	tWidth, _, _ = term.GetSize(int(os.Stdout.Fd()))
	if tWidth < 159 {
		lWidth = tWidth
	} else if h.opts.Level.Level() < slog.LevelInfo {
		if 160 <= tWidth && tWidth <= 180 {
			lWidth = 100
			sWidth = 50
		} else if 179 <= tWidth && tWidth <= 200 {
			sWidth = 75
			lWidth = tWidth - sWidth - 5
		} else {
			lWidth = tWidth / 2
			sWidth = 100
		}
	}
	goas := h.goas
	// If the record has no Attrs, remove groups at the end of the list; they are empty.
	if r.NumAttrs() == 0 {
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}
	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		path = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	if timeExist {
		timestamp = fmt.Sprintf("[%s %s]", "🕙", r.Time.Format(time.Stamp))
	}

	// OUTPUT

	firstLine = true
	msg := slog.String(colorizeLevel(r.Level), r.Message)
	if lWidth < len(msg.String())+10+len(timestamp) {
		buf = h.appendAttr(buf, attrWithInfo{msg, "", false, false, false}, indentLevel)
	} else {
		buf = h.appendAttr(buf, attrWithInfo{msg, timestamp, false, false, false}, indentLevel)
	}
	firstLine = false

	for _, goa := range goas {
		if goa.group != "" {
			group := slog.String("GROUP", goa.group)
			buf = h.appendAttr(buf, attrWithInfo{group, "", false, false, false}, indentLevel)
		} else {
			for _, a := range goa.attrs {
				buf = h.appendAttr(buf, attrWithInfo{a, "", false, false, false}, indentLevel)
			}
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		buf = h.appendAttr(buf, attrWithInfo{a, "", false, false, false}, indentLevel)
		return true
	})

	if sourceExist && path != "" {

		lastLine = true
		indentLevel = 0
		if len("SOURCE"+path)+20 > lWidth {
			path = runewidth.TruncateLeft(path, len(path)+20-lWidth+len("SOUECE"), "...")
		}
		source := slog.String("source", path)

		buf = h.appendAttr(buf, attrWithInfo{source, "", false, false, false}, indentLevel)
		lastLine = false

	}

	buf = fmt.Append(buf, "\n")
	if h.opts.Level.Level() < slog.LevelInfo && tWidth > 158 {
		addSysInfo(&buf)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
	return err
}

func (h *prettyHandler) appendAttr(buf []byte, a attrWithInfo, indentLevel int) []byte {
	// Resolve the Attr's value before doing anything else.
	a.attr.Value = a.attr.Value.Resolve()
	// Ignore empty Attrs.
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
		case firstLine:
			str = fmt.Sprintf("[%s: %q]", a.attr.Key, a.attr.Value.String())
			str = alignValues(str, indentLevel, '-', '─', false, a, false, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "╭")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "╮")
		case groupLine:
			str = fmt.Sprintf("[%s %s: %q]", "📂", color.HiWhiteString(a.attr.Key), a.attr.Value.String())
			str = alignValues(str, indentLevel, ' ', '─', false, a, false, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "├")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		case lastLine:
			str = fmt.Sprintf("[%s: %q] ", a.attr.Key, a.attr.Value.String())
			str = alignValues(str, indentLevel, '╴', '─', true, a, false, false)
			buf = fmt.Append(buf, "╰")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "╯")
		case a.firstChild:
			if len(str) > lWidth-35-indentLevel*10 {
				wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = fmt.Sprintf("%s: %q", key, a.attr.Value.String())
				str = alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		case a.innerChild:
			if len(str) > lWidth-35-indentLevel*10 {
				wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		case a.lastChild:
			if len(str) > lWidth-35-indentLevel*10 {
				wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {
				str = alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "│")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		default:
			if len(str) > lWidth-35-indentLevel*10 {
				wrapLongValue(&buf, a, key, a.attr.Value.String(), indentLevel)
			} else {

				str = fmt.Sprintf("[%s: %q] ", key, a.attr.Value.String())
				str = alignValues(str, indentLevel, ' ', '─', false, a, false, false)
				str = appendInRight(str, a.extraLine)
				buf = fmt.Append(buf, "├")
				buf = fmt.Append(buf, str)
				buf = fmt.Append(buf, "│")
			}
		}
		buf = fmt.Append(buf, "\n")
	case slog.KindAny:
		var data map[string]interface{}

		h.jsonH.WithAttrs([]slog.Attr{a.attr}).Handle(context.Background(), slog.Record{})
		jsonData := jsonBuf.Bytes()
		if err := json.Unmarshal(jsonData, &data); err != nil {
			buf = h.appendAttr(buf, attrWithInfo{slog.String(a.attr.Key, a.attr.Value.String()), "", a.firstChild, a.innerChild, a.lastChild}, indentLevel)
		} else {
			delete(data, "level")
			delete(data, "msg")

			if len(data) > 0 {
				for key, value := range data {
					// Удаляем текущий ключ и добавляем новый с именем " "
					delete(data, key)
					data[""] = value
					break // Завершаем после изменения первого ключа
				}
			}
			prettyJSON, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				buf = h.appendAttr(buf, attrWithInfo{slog.String(a.attr.Key, a.attr.Value.String()), "", a.firstChild, a.innerChild, a.lastChild}, indentLevel)
			} else {
				keyL := runewidth.StringWidth(a.attr.Key)
				sameSymbol := centerString("\u2e17", keyL)
				space := keyL - runewidth.StringWidth(sameSymbol)
				if space < 0 {
					space = -1
				}
				key := strings.Repeat(" ", space) + a.attr.Key
				lines := strings.Split(string(prettyJSON), "\n")

				lines = lines[1 : len(lines)-1]
				lines[0] = lines[0][6:]
				lines[len(lines)-1] = "]"

				for i, line := range lines {

					str := fmt.Sprintf("%s:%q", colorizeKey(indentLevel, key), line)
					if i > 0 {
						str = fmt.Sprintf("%s  %q", colorizeKey(indentLevel, sameSymbol), line)
					}
					str = strings.ReplaceAll(str, "\\", "")
					str = removeFirstAndLastQuote(str)

					if i == 0 {
						str = alignValues(str, indentLevel, '╴', '─', false, a, false, false)
					} else {
						str = alignValues(str, indentLevel, ' ', '─', false, a, false, true)
					}

					if i == 0 {
						str = appendInRight(str, fmt.Sprintf("%s %s", "╕", "struct"))
					} else if i == len(lines)-1 {
						str = appendInRight(str, fmt.Sprintf("%s %s", "╛", "      "))
					} else {
						str = appendInRight(str, fmt.Sprintf("%s %s", "│", "      "))
					}
					buf = fmt.Append(buf, "│")
					buf = fmt.Append(buf, str)
					buf = fmt.Append(buf, "│")
					buf = fmt.Append(buf, "\n")
				}
			}
		}

		jsonBuf.Reset()

	case slog.KindGroup:
		attrs := a.attr.Value.Group()
		if len(attrs) == 0 {
			return buf
		}

		str := fmt.Sprintf("%s%s:", "📦 ", colorizeKey(indentLevel, a.attr.Key))

		switch true {
		case a.firstChild:
			str = alignValues(str, indentLevel, ' ', '─', false, a, true, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "│")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		case a.innerChild:
			str = alignValues(str, indentLevel, ' ', '─', false, a, true, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "│")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		case a.lastChild:
			str = alignValues(str, indentLevel, '╴', '─', true, a, true, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "│")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		default:
			str = alignValues(str, indentLevel, ' ', '─', false, a, true, false)
			str = appendInRight(str, a.extraLine)
			buf = fmt.Append(buf, "├")
			buf = fmt.Append(buf, str)
			buf = fmt.Append(buf, "│")
		}
		buf = fmt.Append(buf, "\n")
		indentLevel++

		for i, ga := range attrs {

			if i == 0 {
				if i == len(attrs)-1 {
					buf = h.appendAttr(buf, attrWithInfo{ga, "", true, false, true}, indentLevel)
				} else {
					buf = h.appendAttr(buf, attrWithInfo{ga, "", true, false, false}, indentLevel)
				}
			} else {
				if i == len(attrs)-1 {
					if i > 1 {
						buf = h.appendAttr(buf, attrWithInfo{ga, "", false, true, true}, indentLevel)
					} else {
						buf = h.appendAttr(buf, attrWithInfo{ga, "", false, true, true}, indentLevel)
					}
				} else {
					buf = h.appendAttr(buf, attrWithInfo{ga, "", false, true, false}, indentLevel)
				}
			}
		}
	default:
		buf = h.appendAttr(buf, attrWithInfo{slog.String(a.attr.Key, a.attr.Value.String()), "", a.firstChild, a.innerChild, a.lastChild}, indentLevel)
	}
	return buf
}

/*--------------------------------UTILS------------------------------------------------------*/

func alignValues(text string, identLevel int, spacer, ident rune, center bool, a attrWithInfo, key, wraped bool) string {
	prefix := strings.Repeat(" ", identLevel*6)

	if identLevel > 1 {
		prefix = addIndentSpaces(identLevel)
	}

	if !key && a.lastChild && a.innerChild {
		activeIndent[identLevel] = false
	}
	if key && !a.lastChild && a.innerChild {
		activeIndent[identLevel] = true
	}

	if !firstLine && !lastLine {
		if key && !a.innerChild {
			text = "╼ " + text
		} else if key {
			text = "╼ " + text
		} else if wraped {
			text = "  " + text
		} else {
			text = "╼ " + text
		}
	} else if firstLine {
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
	case a.innerChild:
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
	paddingL = (lWidth - textLength) / 2
	if paddingL < 0 {
		paddingL = 0
	}
	padding := strings.Repeat(string(spacer), paddingL)
	if center && !lastLine {

		return padding + text + padding

	}
	if firstLine {
		r := runewidth.FillRight(text, lWidth)
		r = strings.ReplaceAll(r, string(' '), string('-'))
		r = text + r[len(text):]
		return r
	}
	if lastLine {
		text = fmt.Sprintf("[%s: %s]", "SOURCE", a.attr.Value.String())
		text1 := fmt.Sprintf("[%s: %s]", color.HiWhiteString("SOURCE"), a.attr.Value.String())
		r := strings.Repeat(string('-'), lWidth-7)
		center := (len(r) - len(text)) / 2
		result := r[:center] + text1 + r[center+len(text):]
		return result
	}
	return runewidth.FillRight(text, lWidth)
}

func addIndentSpaces(parts int) string {
	result := ""
	for i := 0; i < parts; i++ {
		part := runewidth.FillRight("", 6)
		if activeIndent[i] {
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
func removeFirstAndLastQuote(str string) string {
	firstQuoteIndex := strings.Index(str, `"`)
	lastQuoteIndex := strings.LastIndex(str, `"`)
	if firstQuoteIndex != -1 && lastQuoteIndex != -1 && firstQuoteIndex != lastQuoteIndex {
		return str[:firstQuoteIndex] + str[firstQuoteIndex+1:lastQuoteIndex] + str[lastQuoteIndex+1:]
	}
	return str
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
func wrapLongValue(buf *[]byte, a attrWithInfo, key, value string, lvl int) {

	vMaxL := lWidth - len(a.attr.Key) - 5 - lvl*10 - 20
	if lWidth < 60 {
		vMaxL = lWidth - len(a.attr.Key) - 5 - lvl*10 - 35
	} else {

	}
	if vMaxL < 0 {
		vMaxL = 1
	}
	vals := splitText(value, vMaxL)
	keyL := len(a.attr.Key)
	sameSymbol := centerString("\u2e17", keyL)
	space := keyL - runewidth.StringWidth(sameSymbol)
	if space < 0 {
		space *= -1
	}
	key = strings.Repeat(string(' '), space) + key

	for i, v := range vals {
		var str string
		if i == 0 {
			str = fmt.Sprintf("%s:%q", key, v)
		} else {
			str = fmt.Sprintf("%s:%q", colorizeKey(lvl, sameSymbol), v)
		}
		if i == 0 {
			str = alignValues(str, lvl, ' ', '─', false, a, false, false)

		} else {
			str = alignValues(str, lvl, ' ', '─', false, a, false, true)
		}
		if lWidth > 60 {
			if i == 0 {
				str = appendInRight(str, fmt.Sprintf("%s %s", "╕", "line"))
			} else if i == 1 {
				str = appendInRight(str, fmt.Sprintf("%s %s", "│", "wrap"))
			} else if i == len(vals)-1 && i > 1 {
				str = appendInRight(str, fmt.Sprintf("%s %s", "╛", "    "))
			} else {
				str = appendInRight(str, fmt.Sprintf("%s %s", " │", "    "))
			}
		}

		*buf = fmt.Append(*buf, "│")
		*buf = fmt.Append(*buf, str)
		*buf = fmt.Append(*buf, "│")
		if i < len(vals)-1 {
			*buf = fmt.Append(*buf, "\n")
		}
	}
}

func splitText(text string, maxLength int) []string {
	result := []string{}
	index := 0
	str := []byte(text)
	for {
		i := cutString(str, maxLength, index)
		result = append(result, (string(str[index:i])))
		if i >= len(str) {
			break
		}
		index = i
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
	spaceIndex := bytes.LastIndexByte(buf[index:end], ' ')
	slashIndex := bytes.LastIndexByte(buf[index:end], '/')
	dotIndex := bytes.LastIndexByte(buf[index:end], '.')
	comaIndex := bytes.LastIndexByte(buf[index:end], ',')
	enterIndex := bytes.LastIndexByte(buf[index:end], '\n')
	tempIndex := []int{spaceIndex, slashIndex, dotIndex, comaIndex, enterIndex}
	t := -1
	for _, i := range tempIndex {
		if i > t {
			t = i
		}
	}
	if t == -1 {
		return end
	}
	return index + t + 1
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
