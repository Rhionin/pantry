package grocysync

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

func TestPantrySnapshotReadsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/sync/snapshot" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(syncsnap.Snapshot{
			Products: []syncsnap.Product{{ID: "p1", Name: "Milk"}},
		})
	}))
	defer srv.Close()
	client := NewPantryClient(srv.URL, "tok")
	snap, err := client.Snapshot(context.Background())
	if err != nil || len(snap.Products) != 1 || snap.Products[0].Name != "Milk" {
		t.Fatalf("snap = %+v %v", snap, err)
	}
	bad := NewPantryClient(srv.URL, "other")
	if _, err := bad.Snapshot(context.Background()); err == nil {
		t.Fatal("expected a rejection")
	}
}

func TestRunReconcilesUntilCancelled(t *testing.T) {
	fake := newFakeGrocy()
	grocy := fake.server()
	defer grocy.Close()
	pantrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/snapshot") {
			_ = json.NewEncoder(w).Encode(syncsnap.Snapshot{
				Products:     []syncsnap.Product{},
				Groups:       []syncsnap.Group{},
				Instances:    []syncsnap.Instance{},
				Removals:     []syncsnap.Removal{},
				Moves:        []syncsnap.Move{},
				ShoppingList: []syncsnap.ShoppingLine{},
			})
			return
		}
		_, _ = w.Write([]byte("data: {}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer pantrySrv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{
		PantryURL:   pantrySrv.URL,
		PantryToken: "short",
		GrocyURL:    grocy.URL,
		GrocyKey:    fake.key,
		MapPath:     filepath.Join(t.TempDir(), "map.db"),
		Interval:    time.Hour,
		Loc:         time.UTC,
	}
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prev)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fake.count(http.MethodPost, "/objects/locations") == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(2500 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not stop")
	}
	if fake.count(http.MethodPost, "/objects/locations") < 1 {
		t.Fatal("expected a location")
	}
	if fake.count(http.MethodGet, "/system/info") < 1 || !strings.Contains(logs.String(), "grocy-sync: Grocy 4.7.1") {
		t.Fatalf("version log missing: %s", logs.String())
	}
}

func TestRunContinuesWhenGrocyVersionIsUnreadable(t *testing.T) {
	fake := newFakeGrocy()
	fake.systemInfoCode = http.StatusInternalServerError
	grocy := fake.server()
	defer grocy.Close()
	pantrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/snapshot") {
			_ = json.NewEncoder(w).Encode(syncsnap.Snapshot{})
			return
		}
		<-r.Context().Done()
	}))
	defer pantrySrv.Close()
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prev)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			PantryURL:   pantrySrv.URL,
			PantryToken: "0123456789abcdef",
			GrocyURL:    grocy.URL,
			GrocyKey:    fake.key,
			MapPath:     filepath.Join(t.TempDir(), "map.db"),
			Interval:    time.Hour,
			Loc:         time.UTC,
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fake.count(http.MethodPost, "/objects/locations") == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not stop")
	}
	if !strings.Contains(logs.String(), "could not read the Grocy version") || strings.Contains(logs.String(), "do-not-leak") {
		t.Fatalf("logs = %s", logs.String())
	}
}

func TestRunIdlesWhenDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestWatchNotifiesThenEnds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("data: {\"ok\":true}\n\n"))
	}))
	defer srv.Close()
	client := NewPantryClient(srv.URL, "tok")
	n := 0
	err := client.Watch(context.Background(), func() { n++ })
	if n != 1 || err == nil {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestLoopStopsWhenCancelled(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenMap(filepath.Join(t.TempDir(), "map.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	pantry := NewPantryClient(srv.URL, "tok")
	rec := &Reconciler{Pantry: pantry, Grocy: api, Map: m, Now: time.Now, Loc: time.UTC}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- loop(ctx, time.Hour, rec, pantry)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop")
	}
}
