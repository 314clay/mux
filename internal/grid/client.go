package grid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const DefaultURL = "http://100.112.120.2:8420"

type Client struct {
	BaseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Layout types

type MonitorLayout struct {
	Monitor json.Number `json:"monitor"`
	Label   string      `json:"label"`
	Mode    string      `json:"mode"`
	Panes   []Pane      `json:"panes,omitempty"`
}

type Pane struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Column string `json:"column,omitempty"`
	Row    int    `json:"row,omitempty"`
}

type CellData struct {
	URL string `json:"url"`
}

// GetLayout returns the layout for all monitors.
func (c *Client) GetLayout() ([]MonitorLayout, error) {
	resp, err := c.get("/layout")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var wrapper struct {
		Monitors []MonitorLayout `json:"monitors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decoding layout: %w", err)
	}
	return wrapper.Monitors, nil
}

// MonitorCells holds the response from GET /monitor/:id/cells.
type MonitorCells struct {
	Monitor string                 `json:"monitor"`
	Cells   map[string]interface{} `json:"cells"`
	Layout  map[string][]string    `json:"layout"` // column -> cell IDs
}

// GetMonitorCells returns cells for a specific monitor.
func (c *Client) GetMonitorCells(monitorID string) (*MonitorCells, error) {
	resp, err := c.get(fmt.Sprintf("/monitor/%s/cells", monitorID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var mc MonitorCells
	if err := json.NewDecoder(resp.Body).Decode(&mc); err != nil {
		return nil, fmt.Errorf("decoding cells: %w", err)
	}
	return &mc, nil
}

// ParseCellID splits a cell ID like "B4" into column "B" and row 4.
func ParseCellID(cellID string) (col string, row int, err error) {
	if len(cellID) < 2 {
		return "", 0, fmt.Errorf("invalid cell ID %q: must be letter + number (e.g. B4)", cellID)
	}
	col = string(cellID[0])
	if col[0] < 'A' || col[0] > 'Z' {
		return "", 0, fmt.Errorf("invalid cell ID %q: must start with A-Z", cellID)
	}
	row, err = strconv.Atoi(cellID[1:])
	if err != nil || row < 1 {
		return "", 0, fmt.Errorf("invalid cell ID %q: row must be a positive number", cellID)
	}
	return col, row, nil
}

// SetCell updates a cell's URL on a monitor.
// Cell names are positional: B4 = column B, row 4.
// If the cell doesn't exist yet, blank rows are auto-filled up to it
// (e.g. setting B4 on a 2-row column creates B3=about:blank then B4=url).
func (c *Client) SetCell(monitorID, cellID, url string) error {
	col, row, err := ParseCellID(cellID)
	if err != nil {
		return err
	}

	mc, err := c.GetMonitorCells(monitorID)
	if err != nil {
		return fmt.Errorf("checking cells: %w", err)
	}

	if mc.Layout == nil {
		mc.Layout = make(map[string][]string)
	}

	existing := mc.Layout[col]
	currentRows := len(existing)

	// Cell already exists in layout — just update the URL
	if row <= currentRows {
		body := map[string]string{"url": url}
		return c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, cellID), body)
	}

	// Need to add rows. Fill gaps with about:blank.
	for r := currentRows + 1; r <= row; r++ {
		newCell := fmt.Sprintf("%s%d", col, r)
		existing = append(existing, newCell)
		if r < row {
			// Blank filler cell
			blankBody := map[string]string{"url": "about:blank"}
			if err := c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, newCell), blankBody); err != nil {
				return fmt.Errorf("creating filler cell %s: %w", newCell, err)
			}
		}
	}

	// Update column layout with new cells
	mc.Layout[col] = existing
	if err := c.patch(fmt.Sprintf("/monitor/%s", monitorID), map[string]interface{}{
		"columns": mc.Layout,
	}); err != nil {
		return fmt.Errorf("updating column layout: %w", err)
	}

	// Set the actual target cell
	body := map[string]string{"url": url}
	return c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, cellID), body)
}

// ClearCell sets a cell to about:blank.
func (c *Client) ClearCell(monitorID, cellID string) error {
	return c.SetCell(monitorID, cellID, "about:blank")
}

// TtydURL generates a ttyd URL that attaches to a tmux session.
func TtydURL(session string) string {
	return fmt.Sprintf("http://localhost:7681/?arg=tmux&arg=attach&arg=-t&arg=%s", session)
}

func (c *Client) get(path string) (*http.Response, error) {
	resp, err := c.httpClient.Get(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}
	return resp, nil
}

func (c *Client) patch(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: %d %s", path, resp.StatusCode, string(b))
	}
	return nil
}
