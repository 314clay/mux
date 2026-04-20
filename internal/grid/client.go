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
	Index  *int   `json:"index,omitempty"` // bsp mode: 0-based position
}

type BSPPage struct {
	Index int    `json:"index"`
	URL   string `json:"url"`
	Width int    `json:"width,omitempty"`
}

type BSPInfo struct {
	Monitor string    `json:"monitor"`
	Count   int       `json:"count"`
	Pages   []BSPPage `json:"pages"`
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
// Layout comes back as a dimension string like "1x1" from current
// Grid Server versions; older versions sent a column->cellIDs map.
// We keep it opaque and derive existence from Cells.
type MonitorCells struct {
	Monitor string                 `json:"monitor"`
	Cells   map[string]interface{} `json:"cells"`
	Layout  json.RawMessage        `json:"layout,omitempty"`
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
// Accepts upper or lower case columns and normalizes to upper case.
func ParseCellID(cellID string) (col string, row int, err error) {
	if len(cellID) < 2 {
		return "", 0, fmt.Errorf("invalid cell ID %q: must be letter + number (e.g. B4)", cellID)
	}
	c := cellID[0]
	if c >= 'a' && c <= 'z' {
		c = c - 'a' + 'A'
	}
	if c < 'A' || c > 'Z' {
		return "", 0, fmt.Errorf("invalid cell ID %q: must start with A-Z", cellID)
	}
	col = string(c)
	row, err = strconv.Atoi(cellID[1:])
	if err != nil || row < 1 {
		return "", 0, fmt.Errorf("invalid cell ID %q: row must be a positive number", cellID)
	}
	return col, row, nil
}

// ParseBSPCell parses a single-letter BSP cell ID like "C" into
// 0-based index 2. Accepts upper or lower case.
func ParseBSPCell(cellID string) (idx int, err error) {
	if len(cellID) != 1 {
		return 0, fmt.Errorf("invalid bsp cell %q: must be a single letter (A, B, C, ...)", cellID)
	}
	c := cellID[0]
	if c >= 'a' && c <= 'z' {
		c = c - 'a' + 'A'
	}
	if c < 'A' || c > 'Z' {
		return 0, fmt.Errorf("invalid bsp cell %q: must be A-Z", cellID)
	}
	return int(c - 'A'), nil
}

// IndexToBSPCell converts 0-based index 2 → "C".
func IndexToBSPCell(idx int) string {
	if idx < 0 || idx > 25 {
		return fmt.Sprintf("#%d", idx)
	}
	return string(rune('A' + idx))
}

// ParseTarget splits a combined monitor+cell target into a monitor ID
// and a cell ID. The monitor is leading digits; the cell follows.
//
// Grid-mode cells are letter+row digits (e.g. "1a1" → ("1","A1"),
// "10c4" → ("10","C4")). BSP-mode cells are a single letter
// (e.g. "3a" → ("3","A"), "1c" → ("1","C")). The caller decides what
// to do with the cell based on the monitor's mode.
func ParseTarget(target string) (monitor, cell string, err error) {
	if target == "" {
		return "", "", fmt.Errorf("empty target")
	}
	i := 0
	for i < len(target) && target[i] >= '0' && target[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", "", fmt.Errorf("invalid target %q: must start with monitor number (e.g. 1a1 or 1c)", target)
	}
	if i == len(target) {
		return "", "", fmt.Errorf("invalid target %q: missing cell (e.g. 1a1 or 1c)", target)
	}
	monitor = target[:i]
	cellRaw := target[i:]

	// Letter-only → BSP cell (single letter A-Z).
	if len(cellRaw) == 1 {
		idx, perr := ParseBSPCell(cellRaw)
		if perr != nil {
			return "", "", fmt.Errorf("invalid target %q: %w", target, perr)
		}
		return monitor, IndexToBSPCell(idx), nil
	}

	// Otherwise must be grid form letter+row.
	col, row, perr := ParseCellID(cellRaw)
	if perr != nil {
		return "", "", fmt.Errorf("invalid target %q: %w", target, perr)
	}
	return monitor, fmt.Sprintf("%s%d", col, row), nil
}

// MonitorMode looks up the mode ("grid", "bsp", "queue") for a single
// monitor by hitting /layout. Returns the mode lower-cased.
func (c *Client) MonitorMode(monitorID string) (string, error) {
	layouts, err := c.GetLayout()
	if err != nil {
		return "", err
	}
	for _, l := range layouts {
		if l.Monitor.String() == monitorID {
			return l.Mode, nil
		}
	}
	return "", fmt.Errorf("monitor %s not found", monitorID)
}

// SetCell updates a cell on a monitor. Behavior depends on the
// monitor's mode:
//   - grid: cellID is letter+row (e.g. "B4"); blank cells are
//     auto-filled if the row doesn't exist yet.
//   - bsp:  cellID is a single letter (e.g. "C" → index 2). If the
//     position exists, its URL is replaced via /bsp/reorder. If it's
//     past the end, the page list is padded with about:blank fillers
//     and the target URL is appended.
func (c *Client) SetCell(monitorID, cellID, url string) error {
	mode, err := c.MonitorMode(monitorID)
	if err != nil {
		return fmt.Errorf("looking up monitor mode: %w", err)
	}
	switch mode {
	case "bsp":
		return c.setBSPCell(monitorID, cellID, url)
	case "queue":
		return fmt.Errorf("monitor %s is in queue mode; cell-level set not supported", monitorID)
	}
	return c.setGridCell(monitorID, cellID, url)
}

func (c *Client) setBSPCell(monitorID, cellID, url string) error {
	idx, err := ParseBSPCell(cellID)
	if err != nil {
		return err
	}
	info, err := c.GetBSP(monitorID)
	if err != nil {
		return fmt.Errorf("fetching bsp pages: %w", err)
	}
	urls := make([]string, len(info.Pages))
	for i, p := range info.Pages {
		urls[i] = p.URL
	}
	if idx < len(urls) {
		urls[idx] = url
	} else {
		for len(urls) < idx {
			urls = append(urls, "about:blank")
		}
		urls = append(urls, url)
	}
	return c.BSPReorder(monitorID, urls)
}

func (c *Client) setGridCell(monitorID, cellID, url string) error {
	col, row, err := ParseCellID(cellID)
	if err != nil {
		return err
	}

	mc, err := c.GetMonitorCells(monitorID)
	if err != nil {
		return fmt.Errorf("checking cells: %w", err)
	}

	// Count existing rows in this column from mc.Cells (keys like "A1","A2",...).
	currentRows := 0
	for k := range mc.Cells {
		kcol, krow, perr := ParseCellID(k)
		if perr != nil || kcol != col {
			continue
		}
		if krow > currentRows {
			currentRows = krow
		}
	}

	// Cell already exists — just update the URL.
	if row <= currentRows {
		body := map[string]string{"url": url}
		return c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, cellID), body)
	}

	// Need to add rows. Fill gaps with about:blank, then set the target.
	for r := currentRows + 1; r < row; r++ {
		newCell := fmt.Sprintf("%s%d", col, r)
		blankBody := map[string]string{"url": "about:blank"}
		if err := c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, newCell), blankBody); err != nil {
			return fmt.Errorf("creating filler cell %s: %w", newCell, err)
		}
	}
	body := map[string]string{"url": url}
	return c.patch(fmt.Sprintf("/monitor/%s/cell/%s", monitorID, cellID), body)
}

// ClearCell sets a cell to about:blank.
func (c *Client) ClearCell(monitorID, cellID string) error {
	return c.SetCell(monitorID, cellID, "about:blank")
}

// GetBSP returns the BSP page list for a monitor.
func (c *Client) GetBSP(monitorID string) (*BSPInfo, error) {
	resp, err := c.get(fmt.Sprintf("/monitor/%s/bsp", monitorID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var info BSPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding bsp: %w", err)
	}
	return &info, nil
}

// BSPAppend pushes a URL to the end of the BSP page list.
func (c *Client) BSPAppend(monitorID, url string) error {
	return c.post(fmt.Sprintf("/monitor/%s/bsp/append", monitorID), map[string]string{"url": url})
}

// BSPPrepend pushes a URL to the front of the BSP page list.
func (c *Client) BSPPrepend(monitorID, url string) error {
	return c.post(fmt.Sprintf("/monitor/%s/bsp/prepend", monitorID), map[string]string{"url": url})
}

// BSPRemoveByIndex removes the page at index from the BSP list.
func (c *Client) BSPRemoveByIndex(monitorID string, index int) error {
	return c.delete(fmt.Sprintf("/monitor/%s/bsp/%d", monitorID, index))
}

// BSPRemoveByURL removes the first page matching url from the BSP list.
func (c *Client) BSPRemoveByURL(monitorID, url string) error {
	return c.post(fmt.Sprintf("/monitor/%s/bsp/remove", monitorID), map[string]string{"url": url})
}

// BSPReorder replaces the BSP page list with urls (in order).
func (c *Client) BSPReorder(monitorID string, urls []string) error {
	return c.post(fmt.Sprintf("/monitor/%s/bsp/reorder", monitorID), map[string][]string{"pages": urls})
}

// --- Layout presets ---

type PresetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetPresets lists named layout snapshots stored on the grid server.
func (c *Client) GetPresets() ([]PresetInfo, error) {
	resp, err := c.get("/layout/presets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var wrapper struct {
		Presets []PresetInfo `json:"presets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decoding presets: %w", err)
	}
	return wrapper.Presets, nil
}

// ApplyPreset swaps the named preset into the live layout.
func (c *Client) ApplyPreset(name string) error {
	return c.post("/layout/apply", map[string]string{"preset": name})
}

// SavePreset snapshots the current live layout under the given name.
func (c *Client) SavePreset(name, description string) error {
	return c.post("/layout/save", map[string]string{"name": name, "description": description})
}

// DeletePreset removes a named snapshot.
func (c *Client) DeletePreset(name string) error {
	return c.delete("/layout/presets/" + name)
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
	return c.send("PATCH", path, body)
}

func (c *Client) post(path string, body interface{}) error {
	return c.send("POST", path, body)
}

func (c *Client) delete(path string) error {
	return c.send("DELETE", path, nil)
}

func (c *Client) send(method, path string, body interface{}) error {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, string(b))
	}
	return nil
}
