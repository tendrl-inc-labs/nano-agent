package socketclient

import (
	"runtime"
	"testing"
)

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if runtime.GOOS == "windows" {
		if path != `C:\ProgramData\tendrl\tendrl_agent.sock` {
			t.Fatalf("unexpected windows socket path: %s", path)
		}
		return
	}
	if path != "/var/lib/tendrl/tendrl_agent.sock" {
		t.Fatalf("unexpected unix socket path: %s", path)
	}
}
