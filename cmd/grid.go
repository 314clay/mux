package cmd

import (
	"fmt"
	"strconv"

	"mux/internal/grid"

	"github.com/spf13/cobra"
)

var gridURL string

var gridCmd = &cobra.Command{
	Use:   "grid",
	Short: "Show Grid Server layout (all monitors + cells)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		layouts, err := c.GetLayout()
		if err != nil {
			return fmt.Errorf("fetching layout: %w", err)
		}
		for _, l := range layouts {
			label := l.Label
			if label == "" {
				label = "Monitor " + l.Monitor.String()
			}
			fmt.Printf("\n%s [%s] (mode: %s)\n", label, l.Monitor, l.Mode)
			for _, p := range l.Panes {
				url := p.URL
				if len(url) > 60 {
					url = url[:57] + "..."
				}
				fmt.Printf("  %-4s %s\n", p.ID, url)
			}
		}
		fmt.Println()
		return nil
	},
}

var gridSetRaw bool

// resolveTarget accepts either a combined "<monitor><cell>" target
// (e.g. "1a1" → monitor 1, cell A1) as a single arg, or the legacy
// two-arg "<monitor> <cell>" form. Returns the remaining args.
func resolveTarget(args []string) (monitor, cell string, rest []string, err error) {
	if len(args) == 0 {
		return "", "", nil, fmt.Errorf("missing target (e.g. 1a1)")
	}
	if m, c, perr := grid.ParseTarget(args[0]); perr == nil {
		return m, c, args[1:], nil
	} else if len(args) < 2 {
		return "", "", nil, perr
	}
	col, row, cerr := grid.ParseCellID(args[1])
	if cerr != nil {
		return "", "", nil, cerr
	}
	return args[0], fmt.Sprintf("%s%d", col, row), args[2:], nil
}

var gridSetCmd = &cobra.Command{
	Use:   "set <target> <session-or-url>",
	Short: "Assign a tmux session or URL to a grid or bsp cell",
	Long: `Assign content to a cell on a grid- or bsp-mode monitor.

Targets combine monitor and cell. For grid-mode monitors the cell is
letter+row ("1a1" = monitor 1, cell A1). For bsp-mode monitors the
cell is a single letter naming the position in the snake order
("3a" = monitor 3, first pane; "1c" = monitor 1, third pane).

The legacy two-arg form ("<monitor> <cell>") is still accepted.

By default, the value is treated as a tmux session name and wrapped in
a ttyd URL. Use --raw to pass a URL directly.

Examples:
  mux grid set 1a1 fizz                          # grid: tmux session via ttyd
  mux grid set 1a1 --raw http://localhost:9090/  # grid: direct URL
  mux grid set 3c --raw http://localhost:3000/   # bsp: third pane on monitor 3
  mux grid set 0 A1 fizz                         # legacy 2-arg form`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell, rest, err := resolveTarget(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return fmt.Errorf("expected one value (session or URL) after target")
		}
		value := rest[0]
		c := grid.NewClient(gridURL)

		url := value
		if !gridSetRaw {
			url = grid.TtydURL(value)
		}

		if err := c.SetCell(monitor, cell, url); err != nil {
			return err
		}
		fmt.Printf("Assigned %s/%s → %s\n", monitor, cell, url)
		return nil
	},
}

var gridOpenCmd = &cobra.Command{
	Use:   "open <target> <url>",
	Short: "Open a URL in a grid cell",
	Long: `Shorthand for 'mux grid set --raw'. Opens any URL in a grid cell.

Targets combine monitor and cell: "1a1" = monitor 1, cell A1.
The legacy two-arg form ("<monitor> <cell> <url>") is still accepted.`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell, rest, err := resolveTarget(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return fmt.Errorf("expected URL after target")
		}
		url := rest[0]
		c := grid.NewClient(gridURL)
		if err := c.SetCell(monitor, cell, url); err != nil {
			return err
		}
		fmt.Printf("Opened %s/%s → %s\n", monitor, cell, url)
		return nil
	},
}

var gridClearCmd = &cobra.Command{
	Use:   "clear <target>",
	Short: "Clear a grid cell (set to about:blank)",
	Long: `Clear a grid cell. Targets combine monitor and cell ("1a1" =
monitor 1, cell A1). The legacy two-arg form is still accepted.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		monitor, cell, rest, err := resolveTarget(args)
		if err != nil {
			return err
		}
		if len(rest) != 0 {
			return fmt.Errorf("unexpected extra arguments after target")
		}
		c := grid.NewClient(gridURL)
		if err := c.ClearCell(monitor, cell); err != nil {
			return err
		}
		fmt.Printf("Cleared %s/%s\n", monitor, cell)
		return nil
	},
}

// --- BSP subcommands ---

var gridBspCmd = &cobra.Command{
	Use:   "bsp",
	Short: "BSP-mode monitor operations (snake-ordered page list)",
}

var gridBspListCmd = &cobra.Command{
	Use:   "list <monitor>",
	Short: "List pages on a BSP monitor (with letter ids and indices)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		info, err := c.GetBSP(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Monitor %s (bsp, %d pages)\n", info.Monitor, info.Count)
		for _, p := range info.Pages {
			fmt.Printf("  %s [%d] %s\n", grid.IndexToBSPCell(p.Index), p.Index, p.URL)
		}
		return nil
	},
}

var gridBspAppendRaw bool

var gridBspAppendCmd = &cobra.Command{
	Use:   "append <monitor> <session-or-url>",
	Short: "Append a page to the end of a BSP monitor",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		url := args[1]
		if !gridBspAppendRaw {
			url = grid.TtydURL(url)
		}
		if err := c.BSPAppend(args[0], url); err != nil {
			return err
		}
		fmt.Printf("Appended to monitor %s → %s\n", args[0], url)
		return nil
	},
}

var gridBspPrependRaw bool

var gridBspPrependCmd = &cobra.Command{
	Use:   "prepend <monitor> <session-or-url>",
	Short: "Prepend a page to the front of a BSP monitor",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		url := args[1]
		if !gridBspPrependRaw {
			url = grid.TtydURL(url)
		}
		if err := c.BSPPrepend(args[0], url); err != nil {
			return err
		}
		fmt.Printf("Prepended to monitor %s → %s\n", args[0], url)
		return nil
	},
}

var gridBspRemoveCmd = &cobra.Command{
	Use:   "remove <target-or-monitor> [index|url]",
	Short: "Remove a BSP page by target (e.g. 1c), by index, or by URL",
	Long: `Remove a page from a BSP monitor.

Three forms are supported:
  mux grid bsp remove 1c              # combined target (monitor 1, position C)
  mux grid bsp remove 1 2             # monitor 1, index 2
  mux grid bsp remove 1 http://...    # monitor 1, by URL`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		// Combined target form: "1c"
		if len(args) == 1 {
			monitor, cell, err := grid.ParseTarget(args[0])
			if err != nil {
				return err
			}
			idx, err := grid.ParseBSPCell(cell)
			if err != nil {
				return err
			}
			if err := c.BSPRemoveByIndex(monitor, idx); err != nil {
				return err
			}
			fmt.Printf("Removed monitor %s position %s (index %d)\n", monitor, cell, idx)
			return nil
		}
		monitor, second := args[0], args[1]
		// Numeric → by index
		if n, err := strconv.Atoi(second); err == nil {
			if err := c.BSPRemoveByIndex(monitor, n); err != nil {
				return err
			}
			fmt.Printf("Removed monitor %s index %d\n", monitor, n)
			return nil
		}
		// Otherwise treat as URL
		if err := c.BSPRemoveByURL(monitor, second); err != nil {
			return err
		}
		fmt.Printf("Removed from monitor %s: %s\n", monitor, second)
		return nil
	},
}

var gridBspReorderCmd = &cobra.Command{
	Use:   "reorder <monitor> <url1> [url2 ...]",
	Short: "Replace a BSP monitor's page list with the given URLs (in order)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		if err := c.BSPReorder(args[0], args[1:]); err != nil {
			return err
		}
		fmt.Printf("Reordered monitor %s (%d pages)\n", args[0], len(args)-1)
		return nil
	},
}

// --- Preset subcommands ---

var gridPresetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Named layout snapshots (save / load / list / rm)",
}

var gridPresetLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List named layout presets",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		presets, err := c.GetPresets()
		if err != nil {
			return err
		}
		if len(presets) == 0 {
			fmt.Println("(no presets)")
			return nil
		}
		for _, p := range presets {
			if p.Description != "" {
				fmt.Printf("  %-20s %s\n", p.Name, p.Description)
			} else {
				fmt.Printf("  %s\n", p.Name)
			}
		}
		return nil
	},
}

var gridPresetLoadCmd = &cobra.Command{
	Use:   "load <name>",
	Short: "Swap a named preset into the live layout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		if err := c.ApplyPreset(args[0]); err != nil {
			return err
		}
		fmt.Printf("Loaded preset '%s'\n", args[0])
		return nil
	},
}

var gridPresetSaveDesc string

var gridPresetSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Snapshot the current live layout as a named preset",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		if err := c.SavePreset(args[0], gridPresetSaveDesc); err != nil {
			return err
		}
		if gridPresetSaveDesc != "" {
			fmt.Printf("Saved preset '%s' — %s\n", args[0], gridPresetSaveDesc)
		} else {
			fmt.Printf("Saved preset '%s'\n", args[0])
		}
		return nil
	},
}

var gridPresetRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Delete a named preset",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := grid.NewClient(gridURL)
		if err := c.DeletePreset(args[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted preset '%s'\n", args[0])
		return nil
	},
}

func init() {
	gridCmd.PersistentFlags().StringVar(&gridURL, "url", grid.DefaultURL, "Grid Server URL")
	gridSetCmd.Flags().BoolVar(&gridSetRaw, "raw", false, "Treat value as a raw URL instead of a tmux session name")
	gridBspAppendCmd.Flags().BoolVar(&gridBspAppendRaw, "raw", false, "Treat value as a raw URL instead of a tmux session name")
	gridBspPrependCmd.Flags().BoolVar(&gridBspPrependRaw, "raw", false, "Treat value as a raw URL instead of a tmux session name")
	gridPresetSaveCmd.Flags().StringVarP(&gridPresetSaveDesc, "description", "d", "", "Human-readable description for the preset")

	gridBspCmd.AddCommand(gridBspListCmd)
	gridBspCmd.AddCommand(gridBspAppendCmd)
	gridBspCmd.AddCommand(gridBspPrependCmd)
	gridBspCmd.AddCommand(gridBspRemoveCmd)
	gridBspCmd.AddCommand(gridBspReorderCmd)

	gridPresetCmd.AddCommand(gridPresetLsCmd)
	gridPresetCmd.AddCommand(gridPresetLoadCmd)
	gridPresetCmd.AddCommand(gridPresetSaveCmd)
	gridPresetCmd.AddCommand(gridPresetRmCmd)

	gridCmd.AddCommand(gridSetCmd)
	gridCmd.AddCommand(gridOpenCmd)
	gridCmd.AddCommand(gridClearCmd)
	gridCmd.AddCommand(gridBspCmd)
	gridCmd.AddCommand(gridPresetCmd)
	rootCmd.AddCommand(gridCmd)
}
