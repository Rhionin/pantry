package grocysync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFlexIDAcceptsNumberStringAndNull(t *testing.T) {
	var id flexID
	if err := json.Unmarshal([]byte(`"tx-1"`), &id); err != nil || id.String() != "tx-1" {
		t.Fatalf("string id = %q %v", id, err)
	}
	if _, err := id.Int(); err == nil {
		t.Fatal("a transaction id is not a product id")
	}
	if err := json.Unmarshal([]byte(`12`), &id); err != nil {
		t.Fatal(err)
	}
	n, err := id.Int()
	if err != nil || n != 12 {
		t.Fatalf("int = %d %v", n, err)
	}
	if err := json.Unmarshal([]byte(`null`), &id); err != nil {
		t.Fatal(err)
	}
	if _, err := id.Int(); err == nil {
		t.Fatal("null id should be unusable")
	}
	if err := json.Unmarshal([]byte(`"0"`), &id); err != nil {
		t.Fatal(err)
	}
	if _, err := id.Int(); err == nil {
		t.Fatal("zero id should be unusable")
	}
}

func TestFlexFloatAcceptsNumberStringAndNull(t *testing.T) {
	var n flexFloat
	if err := json.Unmarshal([]byte(`"1.5"`), &n); err != nil || float64(n) != 1.5 {
		t.Fatalf("string = %v %v", n, err)
	}
	if err := json.Unmarshal([]byte(`2`), &n); err != nil || float64(n) != 2 {
		t.Fatalf("number = %v %v", n, err)
	}
	if err := json.Unmarshal([]byte(`null`), &n); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`""`), &n); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`"nope"`), &n); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestNewAPIRejectsANonHTTPURL(t *testing.T) {
	if _, err := NewAPI("ftp://grocy", "key"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := NewAPI("   ", "key"); err == nil {
		t.Fatal("expected an error")
	}
	api, err := NewAPI("http://grocy:80/", "key")
	if err != nil || api.Base != "http://grocy:80/api" {
		t.Fatalf("api = %+v %v", api, err)
	}
}

func TestCreateRejectsUnknownProductColumn(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.Create(context.Background(), "products", map[string]any{
		"name":                        "Milk",
		"qu_factor_purchase_to_stock": 1,
		"description":                 "do-not-leak-this-body",
	})
	if err == nil || !strings.Contains(err.Error(), "no column named qu_factor_purchase_to_stock") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "do-not-leak-this-body") || strings.Contains(err.Error(), fake.key) {
		t.Fatalf("error leaked request data: %v", err)
	}
	id, err := api.Create(context.Background(), "products", map[string]any{
		"name": "Milk", "qu_id_purchase": 1, "qu_id_stock": 1, "location_id": 1, "active": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Update(context.Background(), "products", id, map[string]any{"active": 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create(context.Background(), "locations", map[string]any{"name": "Pantry", "is_freezer": 0}); err != nil {
		t.Fatal(err)
	}
	_, err = api.Create(context.Background(), "products", map[string]any{"name": "milk", "location_id": 1, "qu_id_purchase": 1, "qu_id_stock": 1})
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: products.name") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPIOmitsBodyWhenGrocySendsNoMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"request":"do-not-leak","api_key":"secret-key"}`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	err = api.do(context.Background(), http.MethodGet, "/system/info", nil, nil)
	if err == nil || err.Error() != "Grocy returned 500 for /system/info" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error leaked the reply: %v", err)
	}
}

func TestAddStockUsesGrocy4TransactionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stock/products/84/add" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":12,"transaction_id":"6acae98caaf07","stock_id":"abc","undone":0}]`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	booking, err := api.AddStock(context.Background(), 84, addStockBody{Amount: 1, Note: "pantry-instance:i"})
	if err != nil {
		t.Fatal(err)
	}
	if booking.TransactionID != "6acae98caaf07" || booking.StockID != "abc" {
		t.Fatalf("booking = %#v", booking)
	}
}

func TestAddStockRejectsARowArrayWithoutTransactionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":12,"stock_id":"abc"}]`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.AddStock(context.Background(), 1, addStockBody{Amount: 1})
	if err == nil || !strings.Contains(err.Error(), "Grocy did not return a stock transaction") {
		t.Fatalf("err = %v", err)
	}
}

func TestAddStockAcceptsALegacyTransactionObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"tx-old","stock_id":"st1"}`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	booking, err := api.AddStock(context.Background(), 1, addStockBody{Amount: 1})
	if err != nil || booking.TransactionID != "tx-old" || booking.StockID != "st1" {
		t.Fatalf("booking = %#v %v", booking, err)
	}
}

func TestConsumeIncludesGrocyErrorText(t *testing.T) {
	const msg = "Amount to consume is bigger than the current stock amount"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_message":"` + msg + `","api_key":"secret-key"}`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.Consume(context.Background(), 84, consumeBody{Amount: 1})
	if err == nil || !strings.Contains(err.Error(), msg) || !strings.Contains(err.Error(), "/stock/products/84/consume") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error leaked the API key: %v", err)
	}
}

func TestStockEntriesReadsTheEntriesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stock/products/84/entries" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"stock_id":"st1","amount":"2","note":"pantry-instance:i","best_before_date":"2026-06-01","purchased_date":"2026-01-02"}]`))
	}))
	defer srv.Close()
	api, err := NewAPI(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := api.StockEntries(context.Background(), 84)
	if err != nil || len(entries) != 1 || entries[0].StockID != "st1" || entries[0].note() != "pantry-instance:i" || float64(entries[0].Amount) != 2 {
		t.Fatalf("entries = %#v %v", entries, err)
	}
}

func TestVersionReadsGrocySystemInfo(t *testing.T) {
	fake := newFakeGrocy()
	srv := fake.server()
	defer srv.Close()
	api, err := NewAPI(srv.URL, fake.key)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := api.Version(context.Background())
	if err != nil || ver != "4.7.1" {
		t.Fatalf("version = %q %v", ver, err)
	}

	stringSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"grocy_version":"4.7.1"}`))
	}))
	defer stringSrv.Close()
	stringAPI, err := NewAPI(stringSrv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	ver, err = stringAPI.Version(context.Background())
	if err != nil || ver != "4.7.1" {
		t.Fatalf("string version = %q %v", ver, err)
	}

	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"grocy_version":{"ReleaseDate":"2026-09-04"}}`))
	}))
	defer emptySrv.Close()
	emptyAPI, err := NewAPI(emptySrv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emptyAPI.Version(context.Background()); err == nil {
		t.Fatal("expected a missing version")
	}
}
