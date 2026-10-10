package grocysync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

// PantryClient reads the snapshot and the event stream.
type PantryClient struct {
	Base   string
	Token  string
	HTTP   *http.Client
	Events *http.Client
}

// NewPantryClient builds a client. base is the Pantry origin without a path, such as http://pantry:8080.
func NewPantryClient(base, token string) *PantryClient {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	return &PantryClient{
		Base:  base,
		Token: token,
		HTTP:  &http.Client{Timeout: 60 * time.Second},
		// The event stream stays open. A timeout would drop the nudge.
		Events: &http.Client{},
	}
}

// Snapshot calls GET /api/sync/snapshot.
func (c *PantryClient) Snapshot(ctx context.Context) (syncsnap.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/sync/snapshot", nil)
	if err != nil {
		return syncsnap.Snapshot{}, fmt.Errorf("could not read Pantry")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return syncsnap.Snapshot{}, fmt.Errorf("could not reach Pantry")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return syncsnap.Snapshot{}, fmt.Errorf("could not read Pantry")
	}
	if resp.StatusCode != http.StatusOK {
		return syncsnap.Snapshot{}, fmt.Errorf("Pantry returned %d for the sync snapshot", resp.StatusCode)
	}
	var snap syncsnap.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return syncsnap.Snapshot{}, fmt.Errorf("could not read Pantry")
	}
	return snap, nil
}

// Watch calls onEvent once per server-sent event until ctx ends or the stream breaks.
func (c *PantryClient) Watch(ctx context.Context, onEvent func()) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/events", nil)
	if err != nil {
		return fmt.Errorf("could not watch Pantry")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.Events.Do(req)
	if err != nil {
		return fmt.Errorf("could not watch Pantry")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Pantry returned %d for the event stream", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") {
			onEvent()
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("the event stream ended")
}
