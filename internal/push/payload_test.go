package push

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNotifyKeepsALargeBatchUnderTheCap(t *testing.T) {
	subs := newTestSubscriptions(t)
	ctx := context.Background()
	endpoint := "https://updates.push.services.mozilla.com/wpush/v2/batch"
	if err := subs.Save(ctx, sampleSub(endpoint)); err != nil {
		t.Fatal(err)
	}
	var got []byte
	sender := NewSender(subs, Config{
		Post: func(_ context.Context, sub Subscription, payload []byte) (int, error) {
			if sub.Endpoint != endpoint {
				t.Fatalf("endpoint = %s", sub.Endpoint)
			}
			got = append([]byte(nil), payload...)
			return http.StatusCreated, nil
		},
	})
	const items = 500
	names := make([]string, items)
	for i := range names {
		// The ampersand expands to \u0026 in JSON, so the cap has to measure
		// the encoded payload and not the Go string.
		names[i] = strings.Repeat("North Atlantic smoked salmon & fillet ", 8) + "item-" + strconv.Itoa(i)
	}
	sender.Notify(ctx, Notice{
		Title: "Stock out batch is ready",
		Tag:   "pantry-scan-batch-stock_out",
		URL:   "/?scanAlert=stock_out",
		Count: items,
		Names: names,
	})
	if len(got) == 0 || len(got) > maxPushPlaintext {
		t.Fatalf("payload length = %d, cap %d", len(got), maxPushPlaintext)
	}
	if !utf8.Valid(got) {
		t.Fatal("payload is not valid UTF-8")
	}
	var wire wireNotice
	if err := json.Unmarshal(got, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Title != "Stock out batch is ready" {
		t.Fatalf("title = %q", wire.Title)
	}
	if wire.URL != "/?scanAlert=stock_out" {
		t.Fatalf("url = %q", wire.URL)
	}
	if !strings.Contains(wire.Body, "500") || !strings.Contains(wire.Body, "item-0") {
		t.Fatalf("body = %q", wire.Body)
	}
	if strings.Contains(wire.Body, "item-499") {
		t.Fatalf("body included the whole batch: %q", wire.Body)
	}
	if !strings.HasSuffix(wire.Body, "…") {
		t.Fatalf("body = %q", wire.Body)
	}
}

func TestTruncateRunesKeepsMultibyteUTF8(t *testing.T) {
	// é is 2 bytes, 你 is 3, 🙂 is 4. Limits that fall inside a character
	// must not tear it.
	input := strings.Repeat("é你🙂", 40)
	if !utf8.ValidString(input) {
		t.Fatal("fixture is not valid UTF-8")
	}
	for maxBytes := 0; maxBytes <= len(input)+2; maxBytes++ {
		got := truncateRunes(input, maxBytes)
		if !utf8.ValidString(got) {
			t.Fatalf("maxBytes %d: invalid UTF-8 %q", maxBytes, got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Fatalf("maxBytes %d: replacement rune in %q", maxBytes, got)
		}
		if len(got) > maxBytes {
			t.Fatalf("maxBytes %d: length %d in %q", maxBytes, len(got), got)
		}
		if maxBytes > 0 && len(input) > maxBytes && maxBytes >= len("…") && !strings.HasSuffix(got, "…") {
			t.Fatalf("maxBytes %d: missing ellipsis in %q", maxBytes, got)
		}
		bare := strings.TrimRight(strings.TrimSuffix(got, "…"), " ,")
		if !strings.HasPrefix(input, bare) {
			t.Fatalf("maxBytes %d: %q is not a rune prefix of the input", maxBytes, got)
		}
	}

	name := strings.Repeat("é你🙂", 400)
	payload, err := pushPayload(Notice{
		Title: "Stock out batch is ready",
		URL:   "/?scanAlert=stock_out",
		Count: 1,
		Names: []string{name},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxPushPlaintext {
		t.Fatalf("payload length = %d", len(payload))
	}
	var wire wireNotice
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(wire.Body) || strings.ContainsRune(wire.Body, utf8.RuneError) {
		t.Fatalf("body is not valid UTF-8: %q", wire.Body)
	}
	if !strings.HasSuffix(wire.Body, "…") {
		t.Fatalf("body = %q", wire.Body)
	}
	full := compactBody(Notice{Count: 1, Names: []string{name}})
	bare := strings.TrimRight(strings.TrimSuffix(wire.Body, "…"), " ,")
	if !strings.HasPrefix(full, bare) {
		t.Fatalf("body %q is not a rune prefix of %q", wire.Body, full[:80])
	}
}

func TestPushPayloadCapsAnOversizedShell(t *testing.T) {
	payload, err := pushPayload(Notice{
		Title: strings.Repeat("Scan & failed ", 40),
		Body:  strings.Repeat("Open the queue. ", 400),
		Tag:   strings.Repeat("pantry-scan-", 80),
		URL:   "/?" + strings.Repeat("a&b", 4000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 || len(payload) > maxPushPlaintext {
		t.Fatalf("payload length = %d", len(payload))
	}
	if !utf8.Valid(payload) {
		t.Fatal("payload is not valid UTF-8")
	}
	var wire wireNotice
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wire.URL, "/") {
		t.Fatalf("url = %q", wire.URL)
	}
	if wire.Title == "" {
		t.Fatal("empty title")
	}
}

func TestPushPayloadKeepsAShortNotice(t *testing.T) {
	payload, err := pushPayload(Notice{
		Title: "Scan failed",
		Body:  "Couldn't look up that barcode. Open the scan queue to try again.",
		Tag:   "pantry-scan-failure-lookup-1",
		URL:   "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire wireNotice
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Title != "Scan failed" || wire.URL != "/" {
		t.Fatalf("wire = %#v", wire)
	}
	if wire.Body != "Couldn't look up that barcode. Open the scan queue to try again." {
		t.Fatalf("body = %q", wire.Body)
	}
	if wire.Tag != "pantry-scan-failure-lookup-1" {
		t.Fatalf("tag = %q", wire.Tag)
	}
}
