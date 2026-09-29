// Package app holds the terminal UI primitives: raw-mode key reading, full-screen frames,
// the Menu (↑↓ + Enter list) and the Picker (scrollable table with filter).
// It knows nothing about Claude sessions — tools/ feed it plain strings.
package app

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// KeyCode classifies a keypress.
type KeyCode int

const (
	KeyUnknown KeyCode = iota
	KeyText            // printable text in Key.Text (can be several runes on paste)
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyEnter
	KeyEsc
	KeyBackspace
	KeyTab
	KeyCtrlC
	KeyCtrlU
)

// Key is one decoded keypress.
type Key struct {
	Code KeyCode
	Text string
}

var (
	stdinFd  = int(os.Stdin.Fd())
	stdoutFd = int(os.Stdout.Fd())
	rawState *term.State
	inAlt    bool
)

// IsTTY reports whether stdin and stdout are both terminals (interactive mode is possible).
func IsTTY() bool { return term.IsTerminal(stdinFd) && term.IsTerminal(stdoutFd) }

// Start switches to the alternate screen, so the session's output does not pollute scrollback.
func Start() {
	fmt.Print("\033[?1049h\033[H\033[2J")
	inAlt = true
}

// Stop restores the terminal: cooked mode, visible cursor, main screen.
func Stop() {
	Cooked()
	ShowCursor()
	if inAlt {
		fmt.Print("\033[?1049l")
		inAlt = false
	}
}

// Exit restores the terminal and terminates the process (Ctrl+C anywhere).
func Exit() {
	Stop()
	os.Exit(0)
}

// Raw puts stdin into raw mode (idempotent).
func Raw() error {
	if rawState != nil {
		return nil
	}
	st, err := term.MakeRaw(stdinFd)
	if err != nil {
		return err
	}
	rawState = st
	return nil
}

// Cooked restores the terminal mode saved by Raw (idempotent).
func Cooked() {
	if rawState != nil {
		_ = term.Restore(stdinFd, rawState)
		rawState = nil
	}
}

func HideCursor() { fmt.Print("\033[?25l") }
func ShowCursor() { fmt.Print("\033[?25h") }

// Clear wipes the screen and homes the cursor.
func Clear() { fmt.Print("\033[H\033[2J") }

// Size returns the terminal width and height, with sane defaults when unknown.
func Size() (w, h int) {
	w, h, err := term.GetSize(stdoutFd)
	if err != nil || w <= 0 || h <= 0 {
		return 120, 40
	}
	return w, h
}

// Frame redraws the whole screen from the top with the given lines (raw-mode safe).
func Frame(lines []string) {
	var b strings.Builder
	b.WriteString("\033[H")
	for _, l := range lines {
		b.WriteString("\r\033[K")
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	b.WriteString("\033[J")
	fmt.Print(b.String())
}

// ReadKey blocks for one keypress. Stdin must be in raw mode.
func ReadKey() Key {
	buf := make([]byte, 64)
	n, err := os.Stdin.Read(buf)
	if err != nil || n == 0 {
		return Key{Code: KeyEsc}
	}
	b := buf[:n]

	if b[0] == 27 {
		if n == 1 {
			return Key{Code: KeyEsc}
		}
		if n >= 3 && (b[1] == '[' || b[1] == 'O') {
			switch string(b[2:]) {
			case "A":
				return Key{Code: KeyUp}
			case "B":
				return Key{Code: KeyDown}
			case "C":
				return Key{Code: KeyRight}
			case "D":
				return Key{Code: KeyLeft}
			case "H", "1~", "7~":
				return Key{Code: KeyHome}
			case "F", "4~", "8~":
				return Key{Code: KeyEnd}
			case "5~":
				return Key{Code: KeyPgUp}
			case "6~":
				return Key{Code: KeyPgDn}
			}
		}
		return Key{Code: KeyUnknown}
	}

	switch b[0] {
	case '\r', '\n':
		return Key{Code: KeyEnter}
	case 127, 8:
		return Key{Code: KeyBackspace}
	case '\t':
		return Key{Code: KeyTab}
	case 3:
		return Key{Code: KeyCtrlC}
	case 21:
		return Key{Code: KeyCtrlU}
	}
	if b[0] < 32 || !utf8.Valid(b) {
		return Key{Code: KeyUnknown}
	}
	// Drop control chars that may ride along in a paste (newlines etc).
	text := strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, string(b))
	return Key{Code: KeyText, Text: text}
}

// WaitKey shows msg and waits for any key; returns the key pressed.
func WaitKey(msg string) Key {
	fmt.Print("\n  " + msg)
	if err := Raw(); err != nil {
		fmt.Scanln()
		return Key{Code: KeyEnter}
	}
	defer Cooked()
	HideCursor()
	defer ShowCursor()
	k := ReadKey()
	if k.Code == KeyCtrlC {
		Exit()
	}
	fmt.Print("\r\n")
	return k
}
