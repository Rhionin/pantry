package shopping

import "testing"

func TestRetailerDealStatus(t *testing.T) {
	status, detail := (RetailerDealConfig{}).Status()
	if status != RetailerDealsUnavailable || detail == "" {
		t.Fatalf("unconfigured status = %q %q", status, detail)
	}
	status, detail = (RetailerDealConfig{APIKey: "secret", BaseURL: "https://example.invalid"}).Status()
	if status != RetailerDealsStubbed || detail == "" {
		t.Fatalf("keyed status = %q %q", status, detail)
	}
	deals, err := (RetailerDealConfig{APIKey: "secret"}).LiveDeals(t.Context(), "user-1", []string{"item"})
	if err != nil || len(deals) != 0 {
		t.Fatalf("live deals = %+v, %v; a key must not invent prices", deals, err)
	}
}
