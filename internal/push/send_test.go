package push

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// captureClient records the encrypted body webpush-go would POST. Mozilla
// autopush limits that body, not the plaintext.
type captureClient struct {
	body   []byte
	header http.Header
}

func (c *captureClient) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	c.body = body
	c.header = req.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestEncryptedPushBodyStaysUnderAutopushLimit(t *testing.T) {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := browserSubscription("https://updates.push.services.mozilla.com/wpush/v2/max")
	if err != nil {
		t.Fatal(err)
	}
	client := &captureClient{}
	payload := bytes.Repeat([]byte("a"), maxPushPlaintext)
	status, err := deliverWebPush(client, Config{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, sub, payload)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated {
		t.Fatalf("status = %d", status)
	}
	if client.header.Get("Content-Encoding") != "aes128gcm" {
		t.Fatalf("content-encoding = %q", client.header.Get("Content-Encoding"))
	}
	t.Logf("encrypted body = %d bytes for a %d-byte payload", len(client.body), len(payload))
	// webpush-go pads the request body out to RecordSize. 4096 is the
	// library default and is what autopush rejects; 3070 is the
	// constrained-device ceiling.
	if len(client.body) <= maxPushPlaintext || len(client.body) > 3070 || len(client.body) >= 4096 {
		t.Fatalf("encrypted body = %d bytes", len(client.body))
	}
}

func browserSubscription(endpoint string) (Subscription, error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Subscription{}, err
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		return Subscription{}, err
	}
	return Subscription{
		Endpoint: endpoint,
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}, nil
}
