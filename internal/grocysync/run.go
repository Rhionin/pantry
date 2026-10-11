package grocysync

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
)

// Config is the sidecar's environment.
type Config struct {
	PantryURL   string
	PantryToken string
	GrocyURL    string
	GrocyKey    string
	MapPath     string
	Interval    time.Duration
	Loc         *time.Location
}

// Ready reports whether the three secrets that turn sync on are set.
func (c Config) Ready() bool {
	return strings.TrimSpace(c.GrocyURL) != "" &&
		strings.TrimSpace(c.GrocyKey) != "" &&
		strings.TrimSpace(c.PantryToken) != ""
}

// ConfigFromEnv reads the process environment. A bad interval falls back to 5 minutes.
func ConfigFromEnv(getenv func(string) string) Config {
	interval := 5 * time.Minute
	if v := strings.TrimSpace(getenv("GROCY_SYNC_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	pantry := strings.TrimRight(strings.TrimSpace(getenv("PANTRY_URL")), "/")
	if pantry == "" {
		pantry = "http://pantry:8080"
	}
	mapPath := strings.TrimSpace(getenv("GROCY_SYNC_MAP"))
	if mapPath == "" {
		mapPath = "/data/map.db"
	}
	return Config{
		PantryURL:   pantry,
		PantryToken: strings.TrimSpace(getenv("PANTRY_SYNC_TOKEN")),
		GrocyURL:    strings.TrimSpace(getenv("GROCY_URL")),
		GrocyKey:    strings.TrimSpace(getenv("GROCY_API_KEY")),
		MapPath:     mapPath,
		Interval:    interval,
		Loc:         time.Local,
	}
}

// BackfillFlags is the explicit one-time command.
type BackfillFlags struct {
	DBPath  string
	Confirm bool
}

// ParseBackfillArgs reads `backfill --db PATH --confirm`.
func ParseBackfillArgs(args []string) (BackfillFlags, error) {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	db := fs.String("db", "", "path to grocy.db")
	confirm := fs.Bool("confirm", false, "back up grocy.db and write stock_log")
	if err := fs.Parse(args); err != nil {
		return BackfillFlags{}, fmt.Errorf("usage: grocy-sync backfill --db /path/to/grocy.db --confirm")
	}
	return BackfillFlags{DBPath: strings.TrimSpace(*db), Confirm: *confirm}, nil
}

// Run reconciles until ctx is cancelled.
// When the Grocy URL, Grocy key, or Pantry token is missing it waits and does not sync.
func Run(ctx context.Context, cfg Config) error {
	if !cfg.Ready() {
		log.Print("grocy-sync: disabled until GROCY_URL, GROCY_API_KEY, and PANTRY_SYNC_TOKEN are set")
		<-ctx.Done()
		return nil
	}
	if len(cfg.PantryToken) < 16 {
		log.Print("grocy-sync: PANTRY_SYNC_TOKEN is shorter than 16 characters")
	}
	m, err := OpenMap(cfg.MapPath)
	if err != nil {
		return err
	}
	defer m.Close()
	api, err := NewAPI(cfg.GrocyURL, cfg.GrocyKey)
	if err != nil {
		return err
	}
	pantry := NewPantryClient(cfg.PantryURL, cfg.PantryToken)
	rec := &Reconciler{Pantry: pantry, Grocy: api, Map: m, Now: time.Now, Loc: cfg.Loc}
	return loop(ctx, cfg.Interval, rec, pantry)
}

func loop(ctx context.Context, interval time.Duration, rec *Reconciler, pantry *PantryClient) error {
	nudge := make(chan struct{}, 1)
	go func() {
		for ctx.Err() == nil {
			err := pantry.Watch(ctx, func() {
				select {
				case nudge <- struct{}{}:
				default:
				}
			})
			if ctx.Err() != nil {
				return
			}
			log.Printf("grocy-sync: event stream stopped (%v); retrying", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	reconcile := func() {
		if err := rec.Reconcile(ctx); err != nil && ctx.Err() == nil {
			log.Printf("grocy-sync: %v", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var delay *time.Timer
	var delayC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			reconcile()
		case <-nudge:
			if delay == nil {
				delay = time.NewTimer(2 * time.Second)
				delayC = delay.C
				continue
			}
			if !delay.Stop() {
				select {
				case <-delay.C:
				default:
				}
			}
			delay.Reset(2 * time.Second)
		case <-delayC:
			reconcile()
		}
	}
}
