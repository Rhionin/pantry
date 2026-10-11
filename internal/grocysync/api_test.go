package grocysync

import (
	"encoding/json"
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
