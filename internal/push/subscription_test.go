package push

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Rhionin/stockbot/internal/app"
	_ "modernc.org/sqlite"
)

func newTestSubscriptions(t *testing.T) *Subscriptions {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewSubscriptions(conn)
}

func sampleSub(endpoint string) Subscription {
	return Subscription{
		Endpoint: endpoint,
		P256dh:   "p256dh-key",
		Auth:     "auth-key",
	}
}

func TestSubscriptionSaveReplacesKeys(t *testing.T) {
	subs := newTestSubscriptions(t)
	ctx := context.Background()
	first := sampleSub("https://push.example/device")
	if problem := first.Problem(); problem != "" {
		t.Fatal(problem)
	}
	if err := subs.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	updated := first
	updated.Auth = "auth-rotated"
	if err := subs.Save(ctx, updated); err != nil {
		t.Fatal(err)
	}
	got, err := subs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Auth != "auth-rotated" || got[0].P256dh != first.P256dh {
		t.Fatalf("stored = %#v", got)
	}
	if err := subs.Delete(ctx, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	if err := subs.Delete(ctx, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	got, err = subs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("after delete = %#v", got)
	}
}

func TestSubscriptionRejectsANonPushURL(t *testing.T) {
	for _, endpoint := range []string{"http://push.example/s", "https://user:pass@push.example/s", "", "not a url"} {
		if sampleSub(endpoint).Problem() == "" {
			t.Fatalf("accepted %q", endpoint)
		}
	}
}

func TestSenderDropsGoneSubscriptions(t *testing.T) {
	subs := newTestSubscriptions(t)
	ctx := context.Background()
	keep := sampleSub("https://push.example/keep")
	gone := sampleSub("https://push.example/gone")
	missing := sampleSub("https://push.example/missing")
	for _, sub := range []Subscription{keep, gone, missing} {
		if err := subs.Save(ctx, sub); err != nil {
			t.Fatal(err)
		}
	}
	var calls []string
	sender := NewSender(subs, Config{
		Post: func(_ context.Context, sub Subscription, payload []byte) (int, error) {
			calls = append(calls, sub.Endpoint)
			if len(payload) == 0 {
				t.Fatal("empty payload")
			}
			switch sub.Endpoint {
			case gone.Endpoint:
				return http.StatusGone, nil
			case missing.Endpoint:
				return http.StatusNotFound, nil
			default:
				return http.StatusCreated, nil
			}
		},
	})
	sender.Notify(ctx, Notice{Title: "Scan failed", Body: "Try again.", Tag: "pantry-scan-failure-1", URL: "/"})
	if len(calls) != 3 {
		t.Fatalf("calls = %#v", calls)
	}
	left, err := subs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Endpoint != keep.Endpoint {
		t.Fatalf("remaining = %#v", left)
	}
	calls = nil
	sender.Notify(ctx, Notice{Title: "Unrecognized product", Body: "Match it.", Tag: "pantry-scan-unrecognized-1", URL: "/?scanAlert=stock_out"})
	if len(calls) != 1 || calls[0] != keep.Endpoint {
		t.Fatalf("second calls = %#v", calls)
	}
}

func TestSenderKeepsASubscriptionWhenThePushServiceFails(t *testing.T) {
	subs := newTestSubscriptions(t)
	ctx := context.Background()
	sub := sampleSub("https://push.example/busy")
	if err := subs.Save(ctx, sub); err != nil {
		t.Fatal(err)
	}
	sender := NewSender(subs, Config{
		Post: func(context.Context, Subscription, []byte) (int, error) {
			return http.StatusInternalServerError, nil
		},
	})
	sender.Notify(ctx, Notice{Title: "Scan failed", URL: "/"})
	left, err := subs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 {
		t.Fatalf("a 500 removed the browser: %#v", left)
	}
}

func TestNotifyLogsHostAndStatus(t *testing.T) {
	subs := newTestSubscriptions(t)
	ctx := context.Background()
	ok := sampleSub("https://updates.push.services.mozilla.com/wpush/v2/DO-NOT-LOG-TOKEN")
	ok.P256dh = "p256dh-DO-NOT-LOG"
	ok.Auth = "auth-DO-NOT-LOG"
	gone := sampleSub("https://fcm.googleapis.com/fcm/send/DO-NOT-LOG-TOKEN")
	gone.P256dh = "p256dh-DO-NOT-LOG"
	gone.Auth = "auth-DO-NOT-LOG"
	for _, sub := range []Subscription{ok, gone} {
		if err := subs.Save(ctx, sub); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	sender := NewSender(subs, Config{
		Post: func(_ context.Context, sub Subscription, _ []byte) (int, error) {
			if sub.Endpoint == gone.Endpoint {
				return http.StatusGone, nil
			}
			return http.StatusCreated, nil
		},
	})
	sender.Notify(ctx, Notice{Title: "Scan failed", Body: "Try again.", URL: "/"})

	logged := buf.String()
	for _, want := range []string{
		"scan alerts: push to updates.push.services.mozilla.com returned 201",
		"scan alerts: push to fcm.googleapis.com returned 410",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("log missing %q\n%s", want, logged)
		}
	}
	for _, secret := range []string{"DO-NOT-LOG-TOKEN", "p256dh-DO-NOT-LOG", "auth-DO-NOT-LOG", "https://"} {
		if strings.Contains(logged, secret) {
			t.Errorf("log included %q\n%s", secret, logged)
		}
	}
}
