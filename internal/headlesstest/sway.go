// sway IPC access for the compositor-in-the-loop tests: some scenarios
// need the COMPOSITOR to act on a client - a close request delivered
// while the window is fullscreen, say - and the synthetic seat cannot
// express that. The recipe boots sway with its IPC socket in the
// session's private runtime dir; this file speaks the small i3/sway
// wire protocol (magic, length, type, payload) so tests run swaymsg
// commands against the live compositor.
package headlesstest

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// ipcMagic fronts every i3/sway IPC message and reply.
const ipcMagic = "i3-ipc"

// ipcRunCommand is the i3/sway IPC message type that executes a sway
// command string.
const ipcRunCommand = 0

// ipcTimeout bounds one IPC roundtrip; the compositor answers in
// microseconds, so anything past this is a broken session.
const ipcTimeout = 5 * time.Second

// swayCompositor names the artifacts the test-env recipe creates and
// speaks sway's i3 IPC for compositor-driven window actions.
type swayCompositor struct{}

func (swayCompositor) Name() string       { return "sway" }
func (swayCompositor) ConfigName() string { return "sway.cfg" }
func (swayCompositor) PIDName() string    { return "sway.pid" }
func (swayCompositor) LogName() string    { return "sway.log" }

// CanRestart is true: the recipe's boot line is all a restart needs.
func (swayCompositor) CanRestart() bool { return true }

// Restart boots sway again on the recipe's config with the recipe's
// headless environment (sway from PATH, else through the dev shell,
// as the recipe does), records its pid, and waits until its display
// socket accepts connections. A killed sway leaves its socket name's
// lock free, so the new one binds the same wayland-N.
func (swayCompositor) Restart(dir string) error {
	cfg := filepath.Join(dir, "sway.cfg")
	var cmd *exec.Cmd
	if bin, err := exec.LookPath("sway"); err == nil {
		cmd = exec.Command(bin, "-c", cfg) //nolint:gosec // sway from PATH on the recipe's config
	} else {
		cmd = exec.Command("nix", "develop", "-c", "sway", "-c", cfg) //nolint:gosec // the recipe's own boot line
	}
	cmd.Env = privateEnv("XDG_RUNTIME_DIR="+dir, "WLR_BACKENDS=headless", "WLR_LIBINPUT_NO_DEVICES=1", "WLR_RENDERER=pixman")
	log, err := os.OpenFile(filepath.Join(dir, "sway.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the recipe's private runtime dir
	if err != nil {
		return err
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart sway: %w", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "sway.pid"), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600)
	_ = cmd.Process.Release()
	display := os.Getenv("WAYLAND_DISPLAY")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if c, err := net.Dial("unix", filepath.Join(dir, display)); err == nil { //nolint:gosec // the session's own display socket
			_ = c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("restarted sway never served %s", display)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CloseWindow kills the client window with the given app_id.
func (swayCompositor) CloseWindow(dir, appID string) error {
	return swayIPCCommand(dir, `[app_id="`+appID+`"] kill`)
}

// swayIPCCommand runs one sway command (swaymsg's language) against
// the session's compositor and fails when the command itself failed
// (the reply carries one JSON object per command, each with a success
// flag and an error string).
func swayIPCCommand(dir, command string) error {
	sock, err := swayIPCSocket(dir)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", sock, ipcTimeout)
	if err != nil {
		return fmt.Errorf("headlesstest: sway IPC %s: %w", sock, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(ipcTimeout))

	head := make([]byte, 0, 14+len(command))
	head = append(head, ipcMagic...)
	n, err := intconv.CheckedUint32(len(command))
	if err != nil {
		return fmt.Errorf("headlesstest: sway IPC command length: %w", err)
	}
	var sizes [8]byte
	binary.LittleEndian.PutUint32(sizes[0:4], n)
	binary.LittleEndian.PutUint32(sizes[4:8], ipcRunCommand)
	head = append(head, sizes[:]...)
	head = append(head, command...)
	if _, err := conn.Write(head); err != nil {
		return fmt.Errorf("headlesstest: sway IPC write: %w", err)
	}

	reply := make([]byte, 14)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply header: %w", err)
	}
	if string(reply[0:6]) != ipcMagic {
		return fmt.Errorf("headlesstest: sway IPC reply magic %q", reply[0:6])
	}
	payload := make([]byte, binary.LittleEndian.Uint32(reply[6:10]))
	if _, err := io.ReadFull(conn, payload); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply body: %w", err)
	}
	var results []struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(payload, &results); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply %s: %w", payload, err)
	}
	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("headlesstest: sway rejected %q: %s", command, r.Error)
		}
	}
	return nil
}

// swayIPCSocket finds the sway IPC socket in the session's private
// runtime dir; sway names it sway-ipc.<uid>.<pid>.sock.
func swayIPCSocket(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "sway-ipc.*.sock"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("headlesstest: no sway IPC socket in " + dir + "; the test-env recipe's compositor must keep its default IPC enabled")
	}
	return matches[0], nil
}
