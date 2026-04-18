package cmd

import (
	"fmt"
	"time"

	"mux/internal/tmux"

	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all tmux sessions (local + Franklin)",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := tmux.ListAll()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}
		fmt.Printf("%-30s %-10s %-20s %s\n", "SESSION", "HOST", "GROUP", "ACTIVITY")
		for _, s := range sessions {
			group := s.Group
			if group == "" {
				group = "-"
			}
			ago := time.Since(s.Activity).Truncate(time.Second)
			fmt.Printf("%-30s %-10s %-20s %s ago\n", s.Name, s.Host, group, ago)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
