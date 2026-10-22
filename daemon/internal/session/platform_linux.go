//go:build linux

package session

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	displayMu sync.Mutex
	displays  = map[int]bool{}
)

type proc struct {
	cmd  *exec.Cmd
	done chan error
}

type linuxPlatform struct {
	s       *Session
	display string
	number  int
	dir     string
	xauth   string
	xserver *proc
	wm      *proc
	app     *proc
	ffmpeg  *proc
	clip    *exec.Cmd
	closing atomic.Bool
	once    sync.Once
}

func startPlatformOS(ctx context.Context, s *Session, dataDir string, cmd Command) (platform, string, error) {
	xvfb, err := exec.LookPath("Xvfb")
	if err != nil {
		return nil, "", errors.New("Xvfb is not installed")
	}
	xauth, err := exec.LookPath("xauth")
	if err != nil {
		return nil, "", errors.New("xauth is not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, "", errors.New("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, "", errors.New("xdotool is not installed")
	}
	if !commandAvailable(cmd.Path) {
		return nil, "", errors.New("application is not installed: " + cmd.Path)
	}
	number := allocateDisplay()
	if number == 0 {
		return nil, "", errors.New("no free X display available")
	}
	p := &linuxPlatform{s: s, number: number, display: fmt.Sprintf(":%d", number)}
	ok := false
	defer func() {
		if !ok {
			p.releaseDisplay()
		}
	}()
	p.dir = filepath.Join(dataDir, "sessions", s.ID)
	if err := os.MkdirAll(filepath.Join(p.dir, "run"), 0o700); err != nil {
		return nil, "", err
	}
	p.xauth = filepath.Join(p.dir, ".Xauthority")
	if err := makeXauth(xauth, p.xauth, p.display); err != nil {
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	xlog, err := os.OpenFile(filepath.Join(p.dir, "xserver.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	defer xlog.Close()
	xvcmd := exec.Command(xvfb, p.display, "-screen", "0", fmt.Sprintf("%dx%dx24", s.Width, s.Height), "-nolisten", "tcp", "-auth", p.xauth, "-noreset")
	xvcmd.Stdout = xlog
	xvcmd.Stderr = xlog
	xv, err := startProc(xvcmd)
	if err != nil {
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	p.xserver = xv
	go func() {
		<-xv.done
		if !p.closing.Load() {
			s.Close("X server exited")
		}
	}()
	if err := waitForX(p.display, xv.done, 8*time.Second); err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	if wmPath, err := exec.LookPath("openbox"); err == nil {
		wmcmd := exec.Command(wmPath)
		wmcmd.Env = p.env()
		if wm, err := startProc(wmcmd); err == nil {
			p.wm = wm
		}
	} else if wmPath, err := exec.LookPath("fluxbox"); err == nil {
		wmcmd := exec.Command(wmPath)
		wmcmd.Env = p.env()
		if wm, err := startProc(wmcmd); err == nil {
			p.wm = wm
		}
	}
	if xkb, err := exec.LookPath("setxkbmap"); err == nil {
		c := exec.Command(xkb, "-display", p.display, "it")
		c.Env = p.env()
		_ = c.Run()
	}
	args := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		args[i] = strings.ReplaceAll(a, "%SESSION_DIR%", p.dir)
	}
	if os.Geteuid() == 0 && isChromium(cmd.Path) {
		args = append([]string{"--no-sandbox"}, args...)
	}
	appLog, err := os.OpenFile(filepath.Join(p.dir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	defer appLog.Close()
	appcmd := exec.Command(cmd.Path, args...)
	appcmd.Env = p.env()
	appcmd.Stdout = appLog
	appcmd.Stderr = appLog
	app, err := startProc(appcmd)
	if err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	p.app = app
	go func() {
		<-app.done
		if !p.closing.Load() {
			s.Close("application exited")
		}
	}()
	flog, err := os.OpenFile(filepath.Join(p.dir, "ffmpeg.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	defer flog.Close()
	ff := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "warning",
		"-f", "x11grab", "-draw_mouse", "0",
		"-framerate", strconv.Itoa(s.FPS),
		"-video_size", fmt.Sprintf("%dx%d", s.Width, s.Height),
		"-i", p.display+".0+0,0",
		"-c:v", "mjpeg", "-q:v", "7", "-pix_fmt", "yuvj420p",
		"-f", "mjpeg", "pipe:1",
	)
	ff.Env = p.env()
	ff.Stderr = flog
	stdout, err := ff.StdoutPipe()
	if err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	ffp, err := startProc(ff)
	if err != nil {
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", err
	}
	p.ffmpeg = ffp
	first := make(chan struct{})
	firstErr := make(chan error, 1)
	br := bufio.NewReaderSize(stdout, 1<<20)
	go func() {
		jpeg, err := readJPEG(br, 12<<20)
		if err != nil {
			firstErr <- err
			return
		}
		s.broadcastFrame(Frame{At: time.Now(), JPEG: jpeg})
		close(first)
		for {
			jpeg, err := readJPEG(br, 12<<20)
			if err != nil {
				if !p.closing.Load() {
					s.Close("video capture stopped")
				}
				return
			}
			s.broadcastFrame(Frame{At: time.Now(), JPEG: jpeg})
		}
	}()
	select {
	case <-first:
	case err := <-firstErr:
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", fmt.Errorf("video capture: %w", err)
	case <-time.After(8 * time.Second):
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", errors.New("timed out waiting for the first video frame")
	case <-ctx.Done():
		p.Close()
		_ = os.RemoveAll(p.dir)
		return nil, "", ctx.Err()
	}
	ok = true
	return p, p.display, nil
}

func (p *linuxPlatform) env() []string {
	return append(os.Environ(),
		"DISPLAY="+p.display,
		"XAUTHORITY="+p.xauth,
		"XDG_RUNTIME_DIR="+filepath.Join(p.dir, "run"),
		"SESSION_DIR="+p.dir,
		"MAZIMA_SESSION_ID="+p.s.ID,
	)
}

func makeXauth(xauth, file, display string) error {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return err
	}
	cookie := fmt.Sprintf("%x", b)
	cmd := exec.Command(xauth, "-f", file, "add", display, "MIT-MAGIC-COOKIE-1", cookie)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xauth: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Chmod(file, 0o600)
}

func allocateDisplay() int {
	displayMu.Lock()
	defer displayMu.Unlock()
	for n := 90; n < 200; n++ {
		if displays[n] {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X11-unix/X%d", n)); err == nil {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X%d-lock", n)); err == nil {
			continue
		}
		displays[n] = true
		return n
	}
	return 0
}

func (p *linuxPlatform) releaseDisplay() {
	displayMu.Lock()
	delete(displays, p.number)
	displayMu.Unlock()
}

func waitForX(display string, done <-chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	num := strings.TrimPrefix(display, ":")
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/tmp/.X11-unix/X" + num); err == nil {
			return nil
		}
		select {
		case err := <-done:
			return fmt.Errorf("X server failed to start: %v", err)
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timed out waiting for the X server")
}

func startProc(cmd *exec.Cmd) (*proc, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, done: make(chan error, 1)}
	go func() {
		p.done <- cmd.Wait()
	}()
	return p, nil
}

func (p *linuxPlatform) Input(ctx context.Context, ev InputEvent) error {
	if p.closing.Load() {
		return ErrNotFound
	}
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		return err
	}
	switch ev.Type {
	case "pointer":
		x := int(math.Round(ev.X))
		y := int(math.Round(ev.Y))
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= p.s.Width {
			x = p.s.Width - 1
		}
		if y >= p.s.Height {
			y = p.s.Height - 1
		}
		return p.run(ctx, xdotool, "mousemove", "--sync", "--", strconv.Itoa(x), strconv.Itoa(y))
	case "button":
		if ev.Button < 1 || ev.Button > 7 {
			return errors.New("invalid mouse button")
		}
		x, y := int(ev.X), int(ev.Y)
		_ = p.run(ctx, xdotool, "mousemove", "--sync", "--", strconv.Itoa(x), strconv.Itoa(y))
		if ev.Down {
			return p.run(ctx, xdotool, "mousedown", strconv.Itoa(ev.Button))
		}
		return p.run(ctx, xdotool, "mouseup", strconv.Itoa(ev.Button))
	case "wheel":
		steps := int(math.Ceil(math.Abs(ev.DeltaY) / 100))
		if steps < 1 {
			steps = 1
		}
		if steps > 12 {
			steps = 12
		}
		button := "5"
		if ev.DeltaY < 0 {
			button = "4"
		}
		return p.run(ctx, xdotool, "click", "--repeat", strconv.Itoa(steps), "--delay", "15", button)
	case "key":
		if !ev.Down {
			return nil
		}
		if ev.Text != "" && !ev.Ctrl && !ev.Alt && !ev.Meta {
			return p.typeText(ctx, xdotool, ev.Text)
		}
		key := keyName(ev)
		if key == "" {
			return nil
		}
		if key == "BackSpace" || key == "Return" || key == "Tab" || key == "Escape" || key == "Delete" || strings.HasPrefix(key, "Arrow") || strings.HasPrefix(key, "F") {
			if len(key) <= 1 || (len(key) > 1 && !ev.Ctrl && !ev.Alt && !ev.Meta && !ev.Shift) {
				return p.run(ctx, xdotool, "key", "--clearmodifiers", key)
			}
		}
		if !ev.Ctrl && !ev.Alt && !ev.Meta && !ev.Shift && len(ev.Text) == 0 && len([]rune(ev.Key)) == 1 {
			return p.typeText(ctx, xdotool, ev.Key)
		}
		var chord []string
		if ev.Ctrl {
			chord = append(chord, "ctrl")
		}
		if ev.Alt {
			chord = append(chord, "alt")
		}
		if ev.Shift {
			chord = append(chord, "shift")
		}
		if ev.Meta {
			chord = append(chord, "super")
		}
		chord = append(chord, key)
		return p.run(ctx, xdotool, "key", "--clearmodifiers", strings.Join(chord, "+"))
	case "text":
		if ev.Text == "" {
			return nil
		}
		return p.typeText(ctx, xdotool, ev.Text)
	case "clipboard":
		return p.ClipboardSet(ctx, ev.Text)
	default:
		return errors.New("unknown input event")
	}
}

func (p *linuxPlatform) typeText(ctx context.Context, xdotool, text string) error {
	if len(text) > 120000 {
		return errors.New("clipboard text is too large")
	}
	return p.run(ctx, xdotool, "type", "--clearmodifiers", "--delay", "1", "--", text)
}

func (p *linuxPlatform) run(ctx context.Context, bin string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Env = p.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func keyName(ev InputEvent) string {
	if m, ok := keyMap[ev.Code]; ok {
		if ev.Shift && m == "minus" {
			return "underscore"
		}
		if ev.Shift && m == "equal" {
			return "plus"
		}
		return m
	}
	if len(ev.Key) == 1 {
		c := strings.ToLower(ev.Key)
		if c >= "a" && c <= "z" || c >= "0" && c <= "9" {
			return c
		}
	}
	return ""
}

var keyMap = map[string]string{
	"Enter": "Return", "NumpadEnter": "KP_Enter", "Backspace": "BackSpace", "Tab": "Tab", "Escape": "Escape", "Delete": "Delete",
	"ArrowUp": "Up", "ArrowDown": "Down", "ArrowLeft": "Left", "ArrowRight": "Right",
	"Home": "Home", "End": "End", "PageUp": "Prior", "PageDown": "Next", "Insert": "Insert",
	"F1": "F1", "F2": "F2", "F3": "F3", "F4": "F4", "F5": "F5", "F6": "F6",
	"F7": "F7", "F8": "F8", "F9": "F9", "F10": "F10", "F11": "F11", "F12": "F12",
	"Space": "space", "Minus": "minus", "Equal": "equal", "BracketLeft": "bracketleft", "BracketRight": "bracketright",
	"Backslash": "backslash", "Semicolon": "semicolon", "Quote": "apostrophe", "Comma": "comma", "Period": "period", "Slash": "slash", "Backquote": "grave",
	"KeyA": "a", "KeyB": "b", "KeyC": "c", "KeyD": "d", "KeyE": "e", "KeyF": "f", "KeyG": "g", "KeyH": "h", "KeyI": "i", "KeyJ": "j",
	"KeyK": "k", "KeyL": "l", "KeyM": "m", "KeyN": "n", "KeyO": "o", "KeyP": "p", "KeyQ": "q", "KeyR": "r", "KeyS": "s", "KeyT": "t",
	"KeyU": "u", "KeyV": "v", "KeyW": "w", "KeyX": "x", "KeyY": "y", "KeyZ": "z",
	"Digit0": "0", "Digit1": "1", "Digit2": "2", "Digit3": "3", "Digit4": "4", "Digit5": "5", "Digit6": "6", "Digit7": "7", "Digit8": "8", "Digit9": "9",
}

func (p *linuxPlatform) ClipboardGet(ctx context.Context) (string, error) {
	if p.closing.Load() {
		return "", ErrNotFound
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if xclip, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.CommandContext(cctx, xclip, "-selection", "clipboard", "-o")
		cmd.Env = p.env()
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("xclip: %w", err)
		}
		return string(out), nil
	}
	if xsel, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.CommandContext(cctx, xsel, "--clipboard", "--output")
		cmd.Env = p.env()
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("xsel: %w", err)
		}
		return string(out), nil
	}
	return "", errors.New("xclip or xsel is not installed")
}

func (p *linuxPlatform) ClipboardSet(ctx context.Context, text string) error {
	if p.closing.Load() {
		return ErrNotFound
	}
	if len(text) > 120000 {
		return errors.New("clipboard text is too large")
	}
	if p.clip != nil && p.clip.Process != nil {
		_ = p.clip.Process.Kill()
	}
	if xclip, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(xclip, "-selection", "clipboard", "-loops", "1")
		cmd.Env = p.env()
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		p.clip = cmd
		go func() {
			_, _ = io.WriteString(stdin, text)
			_ = stdin.Close()
			_ = cmd.Wait()
		}()
		return nil
	}
	if xsel, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(xsel, "--clipboard", "--input")
		cmd.Env = p.env()
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		p.clip = cmd
		go func() {
			_, _ = io.WriteString(stdin, text)
			_ = stdin.Close()
			_ = cmd.Wait()
		}()
		return nil
	}
	return errors.New("xclip or xsel is not installed")
}

func (p *linuxPlatform) Close() {
	p.once.Do(func() {
		p.closing.Store(true)
		if p.clip != nil && p.clip.Process != nil {
			_ = p.clip.Process.Kill()
		}
		for _, pr := range []*proc{p.ffmpeg, p.app, p.wm, p.xserver} {
			stopProc(pr)
		}
		p.releaseDisplay()
		_ = os.RemoveAll(p.dir)
	})
}

func stopProc(p *proc) {
	if p == nil || p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-p.done:
		case <-time.After(time.Second):
		}
	}
}

func isChromium(path string) bool {
	b := strings.ToLower(filepath.Base(path))
	return strings.Contains(b, "chrom")
}

func readJPEG(r *bufio.Reader, max int) ([]byte, error) {
	var buf bytes.Buffer
	prev := byte(0)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if prev == 0xFF && b == 0xD8 {
			buf.Reset()
			buf.WriteByte(0xFF)
			buf.WriteByte(0xD8)
			break
		}
		prev = b
	}
	prev = 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(b)
		if buf.Len() > max {
			return nil, errors.New("JPEG frame exceeds size limit")
		}
		if prev == 0xFF && b == 0xD9 {
			return buf.Bytes(), nil
		}
		prev = b
	}
}
