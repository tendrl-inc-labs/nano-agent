package socketclient

import (
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"time"
)

const maxResponse = 10 << 20

// DefaultSocketPath returns the platform-default agent socket path.
func DefaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\tendrl\tendrl_agent.sock`
	}
	return "/var/lib/tendrl/tendrl_agent.sock"
}

// Ping checks whether the agent socket accepts connections.
func Ping(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", socketPath, err)
	}
	return conn.Close()
}

// Send encodes msg as JSON, sends it to the agent, and optionally reads a response.
func Send(socketPath string, msg any, waitResponse bool) ([]byte, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", socketPath, err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	if !waitResponse {
		return nil, nil
	}

	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	buf := make([]byte, maxResponse)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return buf[:n], nil
}
