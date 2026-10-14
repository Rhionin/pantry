package push

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/Rhionin/pantry/internal/events"
	webpush "github.com/SherClockHolmes/webpush-go"
)

// pushTTL is how long a push service should keep an alert for a phone that
// is asleep. A scan the household already confirmed can still be opened.
const pushTTL = 24 * 60 * 60

// pushRecordSize is the aes128gcm record webpush-go pads every alert to.
// An unset RecordSize uses MaxRecordSize (4096), and Mozilla autopush then
// answers 413. In v1.4.0 the HTTP body is exactly RecordSize: 16-byte salt,
// 4-byte record size, 1-byte key id length, 65-byte sender key, the
// plaintext, a 1-byte delimiter, zero padding, and a 16-byte tag. 3070 holds
// the 2500-byte plaintext cap (2500+103) and stays on the constrained-device
// limit autopush documents as 3070 bytes.
const pushRecordSize uint32 = 3070

var pushClient = &http.Client{Timeout: 15 * time.Second}

// Sender delivers a notice to every stored browser and drops subscriptions
// the push service says are gone.
type Sender struct {
	subs *Subscriptions
	cfg  Config
}

// NewSender sends with cfg. A nil Post uses Web Push.
func NewSender(subs *Subscriptions, cfg Config) *Sender {
	return &Sender{subs: subs, cfg: cfg}
}

// Listen follows the same broadcaster the scan queue uses, so a push goes
// out when no browser tab is open to see the event stream. scans is read
// when a batch settles, so a scan already confirmed is not announced again.
func Listen(b *events.Broadcaster, sender *Sender, scans scanReader) {
	if b == nil || sender == nil || !sender.cfg.CanSend() {
		return
	}
	msgs, unsub := b.Subscribe()
	go func() {
		defer unsub()
		alerts := newAttention(func(notice Notice) {
			go sender.Notify(context.Background(), notice)
		}, sender.cfg.settle(), realClock{}, scans)
		defer alerts.stop()
		for msg := range msgs {
			alerts.handle(msg)
		}
	}()
}

// Notify pushes one notice. Subscriptions that answer 404 or 410 are removed.
func (s *Sender) Notify(ctx context.Context, notice Notice) {
	if s == nil || s.subs == nil || !s.cfg.CanSend() {
		return
	}
	subs, err := s.subs.List(ctx)
	if err != nil {
		log.Printf("scan alerts: could not read push subscriptions: %v", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	payload, err := pushPayload(notice)
	if err != nil {
		log.Printf("scan alerts: could not encode a notification")
		return
	}
	for _, sub := range subs {
		status, deliverErr := s.deliver(ctx, sub, payload)
		if status == http.StatusNotFound || status == http.StatusGone {
			if err := s.subs.Delete(ctx, sub.Endpoint); err != nil {
				log.Printf("scan alerts: could not remove an expired push subscription: %v", err)
			}
			continue
		}
		if deliverErr != nil || status >= 400 {
			log.Printf("scan alerts: push to %s did not go through (%s)", endpointHost(sub.Endpoint), deliveryProblem(status, deliverErr))
		}
	}
}

func (s *Sender) deliver(ctx context.Context, sub Subscription, payload []byte) (int, error) {
	if s.cfg.Post != nil {
		return s.cfg.Post(ctx, sub, payload)
	}
	return postWebPush(s.cfg, sub, payload)
}

func postWebPush(cfg Config, sub Subscription, payload []byte) (int, error) {
	return deliverWebPush(pushClient, cfg, sub, payload)
}

func deliverWebPush(client webpush.HTTPClient, cfg Config, sub Subscription, payload []byte) (int, error) {
	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}, &webpush.Options{
		HTTPClient:      client,
		Subscriber:      cfg.subject(),
		VAPIDPublicKey:  cfg.PublicKey,
		VAPIDPrivateKey: cfg.PrivateKey,
		TTL:             pushTTL,
		Urgency:         webpush.UrgencyHigh,
		RecordSize:      pushRecordSize,
	})
	if resp != nil {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if err != nil {
			return resp.StatusCode, err
		}
		return resp.StatusCode, nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func endpointHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "a browser"
	}
	return parsed.Host
}

func deliveryProblem(status int, err error) string {
	if err != nil {
		return "the push service did not accept it"
	}
	if status == 0 {
		return "no response"
	}
	return http.StatusText(status)
}
