package systemdnotify

import (
	"context"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestReadyNoopsWhenNotifySocketAbsent(t *testing.T) {
	client := Client{}
	if err := client.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
}

func TestReadySendsUnixDatagram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix datagram sockets are not available")
	}
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	addr := &net.UnixAddr{Name: socketPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Skipf("unix datagram socket unavailable: %v", err)
	}
	defer conn.Close()
	client := Client{Socket: socketPath}
	if err := client.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	buf := make([]byte, 64)
	n, _, err := conn.ReadFromUnix(buf)
	if err != nil {
		t.Fatalf("read datagram: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("datagram = %q", got)
	}
}

func TestInvalidNotifySocketReturnsError(t *testing.T) {
	client := Client{Socket: filepath.Join(t.TempDir(), "missing.sock")}
	if err := client.Ready(context.Background()); err == nil {
		t.Fatal("Ready() error = nil, want invalid socket error")
	}
}

func TestWatchdogEnabledAndInterval(t *testing.T) {
	env := map[string]string{
		"WATCHDOG_USEC": "60000000",
		"WATCHDOG_PID":  strconv.Itoa(42),
	}
	client := Client{PID: 42, Env: func(key string) string { return env[key] }}
	if !client.WatchdogEnabled() {
		t.Fatal("WatchdogEnabled() = false, want true")
	}
	if got := client.WatchdogInterval(30 * time.Second); got != 30*time.Second {
		t.Fatalf("WatchdogInterval() = %s, want 30s", got)
	}
	env["WATCHDOG_PID"] = "99"
	if client.WatchdogEnabled() {
		t.Fatal("WatchdogEnabled() = true for another pid")
	}
}

func TestSendHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := Client{Socket: filepath.Join(t.TempDir(), "missing.sock")}
	if err := client.Ready(ctx); err == nil {
		t.Fatal("Ready() error = nil, want cancellation")
	}
}
