package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	// Distroless has no zoneinfo. Purchase dates follow TZ, which should
	// match the Grocy container or a late-evening stock-in lands on the wrong day.
	_ "time/tzdata"

	"github.com/Rhionin/stockbot/internal/grocysync"
)

func main() {
	cfg := grocysync.ConfigFromEnv(os.Getenv)
	if len(os.Args) > 1 && os.Args[1] == "backfill" {
		if err := runBackfill(cfg, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := grocysync.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func runBackfill(cfg grocysync.Config, args []string) error {
	flags, err := grocysync.ParseBackfillArgs(args)
	if err != nil {
		return err
	}
	if !flags.Confirm {
		return fmt.Errorf("%s\n\nThis writes past Pantry uses into Grocy's stock journal with their real dates. It copies grocy.db to a backup beside that file before it changes anything. Stop the grocy container first.\n\n%s", grocysync.ErrNotConfirmed.Error(), grocysync.RestoreSteps("grocy.db.bak-<timestamp>", flags.DBPath))
	}
	if !cfg.Ready() {
		return fmt.Errorf("set GROCY_URL, GROCY_API_KEY, and PANTRY_SYNC_TOKEN so products can be matched")
	}
	m, err := grocysync.OpenMap(cfg.MapPath)
	if err != nil {
		return err
	}
	defer m.Close()
	products, err := m.ByType("product")
	if err != nil {
		return err
	}
	ids := map[string]int{}
	for _, link := range products {
		if link.GrocyID > 0 {
			ids[link.PantryID] = link.GrocyID
		}
	}
	loc, _, err := m.Get("location", "Pantry")
	if err != nil {
		return err
	}
	pantry := grocysync.NewPantryClient(cfg.PantryURL, cfg.PantryToken)
	snap, err := pantry.Snapshot(context.Background())
	if err != nil {
		return err
	}
	result, err := grocysync.ApplyHistory(context.Background(), grocysync.HistoryOptions{
		DBPath:     flags.DBPath,
		LocationID: loc.GrocyID,
		Products:   ids,
		Snap:       snap,
		Loc:        cfg.Loc,
		Confirm:    true,
	})
	if err != nil {
		return err
	}
	if result.BackupPath == "" {
		log.Print("grocy-sync: history already matches Pantry. Nothing was written.")
		return nil
	}
	log.Printf("grocy-sync: wrote stock history (%d new, %d updated, %d undone). Backup: %s", result.Inserted, result.Updated, result.Undone, result.BackupPath)
	log.Print(grocysync.RestoreSteps(result.BackupPath, flags.DBPath))
	return nil
}
