package tmux

import "time"

// Session represents a tmux session on any host.
type Session struct {
	Name     string
	Group    string
	Host     string // "local" or "franklin"
	Windows  int
	Activity time.Time
}
