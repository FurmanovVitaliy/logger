package logger

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
)

var cursorIndex, linieIndex, lines, wWidth, rWidth, next, row int
var addNext bool

func addSysInfo(buf *[]byte, sWidth int) {
	wWidth = sWidth
	lines = len(strings.Split(string(*buf), "\n")) - 2
	if lines < 5 {
		return
	}

	if wWidth > 80 {
		row = 3
		writeNext(buf, logMemoryStats())
		newNext(buf)
		writeNext(buf, logGoroutineCount()+"           ")
		writeNext(buf, "\n"+logHeapObjects())
		writeNext(buf, "\n"+logGCPauses())
		newNext(buf)
		writeNext(buf, logCPUUsage())

	} else if wWidth > 74 {
		row = 2
		if lines > 9 {
			writeNext(buf, logGoroutineCount()+"           ")
			writeNext(buf, "\n"+logGCPauses())
			writeNext(buf, "\n"+logCPUUsage())
			newNext(buf)
			writeNext(buf, logMemoryStats())
			writeNext(buf, "\n"+logHeapObjects())
		} else {
			writeNext(buf, logGoroutineCount()+"           ")
			writeNext(buf, logGCPauses())
			writeNext(buf, logCPUUsage())
			newNext(buf)
			writeNext(buf, logMemoryStats())
			writeNext(buf, logHeapObjects())
		}

	} else if wWidth > 49 {
		row = 1
		if lines > 19 {
			writeNext(buf, logGoroutineCount())
			writeNext(buf, "\n"+logGCPauses())
			writeNext(buf, "\n"+logCPUUsage())
			writeNext(buf, "\n"+logMemoryStats())
			writeNext(buf, "\n"+logHeapObjects())
		} else {
			writeNext(buf, logGoroutineCount())
			writeNext(buf, logHeapObjects())
			writeNext(buf, logGCPauses())
			writeNext(buf, logCPUUsage())
			writeNext(buf, logMemoryStats())
		}

	}
	// Clear
	newNext(buf)
	cursorIndex, linieIndex, lines, lWidth, wWidth, sWidth, rWidth, next = 0, 0, 0, 0, 0, 0, 0, 0
	addNext = false
}

// SYSINFO
func logMemoryStats() string {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return fmt.Sprintf(
		"💾 *Memory Stats*\n  - 🟢 Current Allocated Memory: %v MiB\n  - 📊 Total Allocated Memory: %v MiB\n  - 🛠️  System Memory Used: %v MiB\n  - ♻️ Garbage Collections: %v",
		memStats.Alloc/1024/1024,
		memStats.TotalAlloc/1024/1024,
		memStats.Sys/1024/1024,
		memStats.NumGC,
	)
}

func logGoroutineCount() string {
	goroutines := runtime.NumGoroutine()
	return fmt.Sprintf("🌀 *Goroutine Stats*\n  - Active Goroutines: %v", goroutines)
}

func logHeapObjects() string {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return fmt.Sprintf("📦 *Heap Objects*\n  - Total Objects Allocated: %v", memStats.HeapObjects)
}

func logGCPauses() string {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return fmt.Sprintf("🕙 *Garbage Collection Pauses*\n  - Total GC Pause Time: %v ms", memStats.PauseTotalNs/1e6)
}

func logCPUUsage() string {
	return fmt.Sprintf(
		"🧠 *CPU Usage*\n  - Available CPUs: %v\n  - GOMAXPROCS: %v",
		runtime.NumCPU(),
		runtime.GOMAXPROCS(0),
	)
}

// WriteNext writes the next line to the buffer
func writeNext(buf *[]byte, newText string) {
	if newText == "" {
		newText = " "
	}
	if linieIndex > lines-1 {
		return
	}
	if linieIndex == 0 {
		sWidth = sWidth - rWidth
		rWidth = 0
		next++

		frame := ("╭" + strings.Repeat("─", wWidth) + "╮")

		insertFrame(buf, frame)
		linieIndex++
		writeNext(buf, newText)
		return
	} else if linieIndex == lines-1 {
		if next > 1 {
			return
		}
		addNext = false
		frame := "╰" + strings.Repeat("─", wWidth) + "╯"
		insertFrame(buf, frame)
		linieIndex++
		return
	} else {

	}

	tl := strings.Split(newText, "\n")
	for _, t := range tl {

		if runewidth.StringWidth(t) > sWidth {
			rWidth = sWidth
		}
		if runewidth.StringWidth(t) > rWidth {
			rWidth = runewidth.StringWidth(t)
		}
	}

	if len(tl) > 1 {
		for _, t := range tl {
			writeNext(buf, t)
		}

		return
	}

	if sWidth > 5 {

		if runewidth.StringWidth(newText) > sWidth {
			newText = newText[:sWidth-6] + "... "
			if next > 3 {
				newText = newText[:sWidth-4] + "... "
			}
		}
		if newText[len(newText)-1] != '\n' {
			r := runewidth.StringWidth(newText)

			if strings.Contains(newText, "\uFE0F") {
				if os.Getenv("TERM_PROGRAM") != "vscode" {
					r = r + 1
				}
			}

			switch row {
			case 1:
				newText = "│ " + newText
				newText = newText + strings.Repeat(" ", sWidth-r-1) + "│" + "\n"
			case 2:
				if next == 1 {
					newText = "│ " + newText
					newText = newText + strings.Repeat(" ", rWidth-r) + "\n"
				} else {
					newText = newText + strings.Repeat(" ", sWidth-r-1) + "│\n"
				}
			case 3:
				if next == 1 {
					newText = "│ " + newText
					newText = newText + strings.Repeat(" ", rWidth-r) + "   \n"
				} else if next == 3 {
					newText = newText + strings.Repeat(" ", sWidth-r-4) + "│\n"
				} else {
					newText = newText + strings.Repeat(" ", rWidth-r) + "\n"
				}
			}

		}

	} else if sWidth > 0 {
		newText = "\n"
	} else {
		return
	}
	endIndex := bytes.IndexByte((*buf)[cursorIndex:], '\n')
	if endIndex == -1 {
		endIndex = len(*buf)
	} else {
		endIndex += cursorIndex
	}
	separator := strings.Repeat(" ", 5)

	if next > 1 {
		separator = ""
	}
	newText = separator + newText

	cursorIndex = endIndex
	insertIntoSlice(buf, []byte(newText), cursorIndex)
	cursorIndex += len(newText)

	linieIndex++
}

func insertFrame(buf *[]byte, frame string) {

	if addNext {
		endIndex := bytes.IndexByte((*buf)[cursorIndex:], '\n')
		cursorIndex = endIndex + 1
		return
	}
	endIndex := bytes.IndexByte((*buf)[cursorIndex:], '\n')
	if endIndex == -1 {
		endIndex = len(*buf)
	} else {
		endIndex += cursorIndex
	}

	frame = strings.Repeat(" ", 5) + frame + "\n"
	cursorIndex = endIndex
	insertIntoSlice(buf, []byte(frame), cursorIndex)
	cursorIndex += len(frame)
	addNext = true
}

func insertIntoSlice(buf *[]byte, insert []byte, index int) {
	*buf = append((*buf)[:index], append(insert, (*buf)[index+1:]...)...)
}
func newNext(buf *[]byte) {

	freeSpace := lines - linieIndex
	for i := 0; i < freeSpace; i++ {
		writeNext(buf, "")
	}
	cursorIndex = 0
	linieIndex = 0
}
