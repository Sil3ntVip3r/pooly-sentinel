package systemdnotify

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type EnvFunc func(string) string

type Client struct {
	Socket string
	PID    int
	Env    EnvFunc
}

func NewFromEnv(env EnvFunc) Client {
	if env == nil {
		env = os.Getenv
	}
	return Client{
		Socket: env("NOTIFY_SOCKET"),
		PID:    os.Getpid(),
		Env:    env,
	}
}

func (c Client) Ready(ctx context.Context) error {
	return c.Send(ctx, "READY=1")
}

func (c Client) Stopping(ctx context.Context) error {
	return c.Send(ctx, "STOPPING=1")
}

func (c Client) Watchdog(ctx context.Context) error {
	return c.Send(ctx, "WATCHDOG=1")
}

func (c Client) Send(ctx context.Context, message string) error {
	if ctx == nil {
		return fmt.Errorf("systemd notify context is nil")
	}
	if err := ctx.Err(); err != nil {
		return redaction.Error(err)
	}
	if c.Socket == "" {
		return nil
	}
	if message == "" {
		return fmt.Errorf("systemd notify message is required")
	}
	addr, err := notifyAddr(c.Socket)
	if err != nil {
		return err
	}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return fmt.Errorf("systemd notify dial: %w", redaction.Error(err))
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	} else {
		_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	}
	if err := ctx.Err(); err != nil {
		return redaction.Error(err)
	}
	if _, err := conn.Write([]byte(message)); err != nil {
		return fmt.Errorf("systemd notify write: %w", redaction.Error(err))
	}
	return nil
}

func (c Client) WatchdogEnabled() bool {
	if c.Env == nil {
		c.Env = os.Getenv
	}
	raw := c.Env("WATCHDOG_USEC")
	if raw == "" {
		return false
	}
	usec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || usec <= 0 {
		return false
	}
	pidRaw := c.Env("WATCHDOG_PID")
	if pidRaw == "" {
		return true
	}
	pid, err := strconv.Atoi(pidRaw)
	if err != nil {
		return false
	}
	if c.PID == 0 {
		c.PID = os.Getpid()
	}
	return pid == c.PID
}

func (c Client) WatchdogInterval(fallback time.Duration) time.Duration {
	if c.Env == nil {
		c.Env = os.Getenv
	}
	raw := c.Env("WATCHDOG_USEC")
	usec, err := strconv.ParseInt(raw, 10, 64)
	if err == nil && usec > 0 {
		interval := time.Duration(usec) * time.Microsecond / 2
		if interval > 0 {
			return interval
		}
	}
	if fallback > 0 {
		return fallback
	}
	return 30 * time.Second
}

func notifyAddr(socket string) (*net.UnixAddr, error) {
	if socket == "" {
		return nil, fmt.Errorf("systemd notify socket is empty")
	}
	name := socket
	if socket[0] == '@' {
		name = "\x00" + socket[1:]
	}
	return &net.UnixAddr{Name: name, Net: "unixgram"}, nil
}
