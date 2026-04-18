package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const listFmt = "#{session_name}\t#{session_group}\t#{session_windows}\t#{session_activity}"

// ListLocal returns all tmux sessions on the local machine.
func ListLocal() ([]Session, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", listFmt).Output()
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing local sessions: %w", err)
	}
	return parseSessions(string(out), "local"), nil
}

// SendLocal sends text to a local tmux session followed by Enter.
func SendLocal(session, text string) error {
	return exec.Command("tmux", "send-keys", "-t", session, text, "Enter").Run()
}

func parseSessions(raw, host string) []Session {
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		wins, _ := strconv.Atoi(parts[2])
		actUnix, _ := strconv.ParseInt(parts[3], 10, 64)
		sessions = append(sessions, Session{
			Name:     parts[0],
			Group:    parts[1],
			Host:     host,
			Windows:  wins,
			Activity: time.Unix(actUnix, 0),
		})
	}
	return sessions
}
