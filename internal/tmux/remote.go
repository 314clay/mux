package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var sshKey = os.Getenv("HOME") + "/.ssh/id_ed25519_homelab"

const franklinHost = "franklin"

// ListFranklin returns all tmux sessions on Franklin via SSH.
func ListFranklin() ([]Session, error) {
	cmd := fmt.Sprintf("tmux list-sessions -F '%s' 2>/dev/null", listFmt)
	out, err := sshRun(cmd)
	if err != nil {
		return nil, nil // Franklin unreachable or no sessions — not fatal
	}
	return parseSessions(out, "franklin"), nil
}

// SendFranklin sends text to a tmux session on Franklin.
func SendFranklin(session, text string) error {
	escaped := strings.ReplaceAll(text, "'", "'\\''")
	cmd := fmt.Sprintf("tmux send-keys -t %s '%s' Enter", session, escaped)
	_, err := sshRun(cmd)
	if err != nil {
		return fmt.Errorf("sending to franklin:%s: %w", session, err)
	}
	return nil
}

// ListAll returns sessions from both local and Franklin, merging results.
func ListAll() ([]Session, error) {
	local, err := ListLocal()
	if err != nil {
		return nil, err
	}
	remote, _ := ListFranklin()
	return append(local, remote...), nil
}

// Send dispatches to local or franklin based on session host.
func Send(s Session, text string) error {
	if s.Host == "franklin" {
		return SendFranklin(s.Name, text)
	}
	return SendLocal(s.Name, text)
}

func sshRun(cmd string) (string, error) {
	args := []string{
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=3",
		franklinHost,
		cmd,
	}
	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
