package product

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidateContribution(t *testing.T) {
	if err := ValidateContribution(false, "", ""); err != nil {
		t.Fatalf("skipping contribution should be valid, got %v", err)
	}
	if err := ValidateContribution(true, "012345678905", ExternalSourceOpenFoodFacts); err != nil {
		t.Fatalf("valid contribution: %v", err)
	}
	if err := ValidateContribution(true, "", ExternalSourceOpenFoodFacts); err == nil {
		t.Fatal("expected a missing barcode to be rejected")
	}
	if err := ValidateContribution(true, "ABC12345", ExternalSourceOpenFoodFacts); err == nil {
		t.Fatal("expected a non-digit barcode to be rejected")
	}
	if err := ValidateContribution(true, "012345678905", ""); err == nil {
		t.Fatal("expected a missing database to be rejected")
	}
	if err := ValidateContribution(true, "012345678905", "not-a-database"); err == nil {
		t.Fatal("expected an unknown database to be rejected")
	}
}

func TestNewContributorFromEnvStaysLocalWithoutCredentials(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{name: "nothing set", env: map[string]string{}},
		{name: "user only", env: map[string]string{"PRODUCT_OPENER_USER_ID": "pantry"}},
		{name: "password only", env: map[string]string{"PRODUCT_OPENER_PASSWORD": "secret"}},
		{
			name: "lookup disabled",
			env: map[string]string{
				"DISABLE_EXTERNAL_PRODUCT_LOOKUP": "true",
				"PRODUCT_OPENER_USER_ID":          "pantry",
				"PRODUCT_OPENER_PASSWORD":         "secret",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contributor := NewContributorFromEnv(func(key string) string { return tc.env[key] })
			if contributor.Configured() {
				t.Fatal("expected contributor to stay unconfigured")
			}
		})
	}
}

func TestNewContributorFromEnvUsesAccountWhenLookupIsAllowed(t *testing.T) {
	contributor := NewContributorFromEnv(func(key string) string {
		switch key {
		case "PRODUCT_OPENER_USER_ID":
			return " pantry "
		case "PRODUCT_OPENER_PASSWORD":
			return "secret"
		default:
			return ""
		}
	})
	if !contributor.Configured() {
		t.Fatal("expected a configured contributor")
	}
	opened, ok := contributor.(*ProductOpenerContributor)
	if !ok {
		t.Fatalf("expected ProductOpenerContributor, got %T", contributor)
	}
	if opened.UserID != "pantry" {
		t.Errorf("user id = %q", opened.UserID)
	}
}

func TestProductOpenerContributorPostsOnlyTheSelectedDatabase(t *testing.T) {
	const password = "test-password"
	var foodHits, productsHits int
	var posted url.Values
	var userAgent, contentType string

	food := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foodHits++
		http.Error(w, "food should not be called", http.StatusInternalServerError)
	}))
	defer food.Close()

	products := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		productsHits++
		if r.URL.Path != "/cgi/product_jqm2.pl" {
			t.Errorf("path = %s", r.URL.Path)
		}
		userAgent = r.Header.Get("User-Agent")
		contentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		posted, err = url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"status_verbose":"fields saved"}`))
	}))
	defer products.Close()

	contributor := &ProductOpenerContributor{
		UserID:     "pantry-bot",
		Password:   password,
		HTTPClient: products.Client(),
		Origins: map[ExternalSource]string{
			ExternalSourceOpenFoodFacts:     food.URL,
			ExternalSourceOpenProductsFacts: products.URL,
		},
	}

	err := contributor.Contribute(context.Background(), Contribution{
		Barcode:       "012345678905",
		Name:          "House Sponge",
		Category:      "Cleaning",
		UnitOfMeasure: "each",
		Database:      ExternalSourceOpenProductsFacts,
	})
	if err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	if foodHits != 0 {
		t.Errorf("Open Food Facts was called %d times", foodHits)
	}
	if productsHits != 1 {
		t.Fatalf("Open Products Facts calls = %d", productsHits)
	}
	if userAgent != productOpenerUserAgent {
		t.Errorf("user agent = %q", userAgent)
	}
	if !strings.Contains(contentType, "application/x-www-form-urlencoded") {
		t.Errorf("content type = %q", contentType)
	}
	if posted.Get("user_id") != "pantry-bot" || posted.Get("password") != password {
		t.Fatalf("account fields were not the test credentials")
	}
	if posted.Get("code") != "012345678905" || posted.Get("product_name") != "House Sponge" {
		t.Fatalf("product fields = %v", posted)
	}
	if posted.Get("categories") != "Cleaning" || posted.Get("quantity") != "each" {
		t.Fatalf("optional fields = %v", posted)
	}
}

func TestProductOpenerContributorOmitsEmptyOptionalFields(t *testing.T) {
	var posted url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	contributor := &ProductOpenerContributor{
		UserID:     "pantry-bot",
		Password:   "test-password",
		HTTPClient: server.Client(),
		Origins: map[ExternalSource]string{
			ExternalSourceOpenFoodFacts: server.URL,
		},
	}
	if err := contributor.Contribute(context.Background(), Contribution{
		Barcode:  "012345678905",
		Name:     "Plain Oats",
		Database: ExternalSourceOpenFoodFacts,
	}); err != nil {
		t.Fatalf("Contribute: %v", err)
	}
	if _, ok := posted["categories"]; ok {
		t.Errorf("empty category was sent: %q", posted.Get("categories"))
	}
	if _, ok := posted["quantity"]; ok {
		t.Errorf("empty quantity was sent: %q", posted.Get("quantity"))
	}
}

func TestProductOpenerContributorRejectsUpstreamFailureWithoutLeakingPassword(t *testing.T) {
	const password = "test-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"status_verbose":"bad password test-password"}`))
	}))
	defer server.Close()

	contributor := &ProductOpenerContributor{
		UserID:     "pantry-bot",
		Password:   password,
		HTTPClient: server.Client(),
		Origins: map[ExternalSource]string{
			ExternalSourceOpenBeautyFacts: server.URL,
		},
	}
	err := contributor.Contribute(context.Background(), Contribution{
		Barcode:  "012345678905",
		Name:     "Hand Soap",
		Database: ExternalSourceOpenBeautyFacts,
	})
	if err == nil {
		t.Fatal("expected upstream failure")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("error leaked the password: %v", err)
	}
}

func TestProductOpenerContributorDoesNotCallWhenUnconfigured(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	contributor := &ProductOpenerContributor{
		Origins: map[ExternalSource]string{
			ExternalSourceOpenFoodFacts: server.URL,
		},
	}
	err := contributor.Contribute(context.Background(), Contribution{
		Barcode:  "012345678905",
		Name:     "Oats",
		Database: ExternalSourceOpenFoodFacts,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if called {
		t.Fatal("unconfigured contributor made a network call")
	}
}

func TestUnconfiguredContributorRefusesToSend(t *testing.T) {
	err := UnconfiguredContributor{}.Contribute(context.Background(), Contribution{
		Barcode:  "012345678905",
		Name:     "Oats",
		Database: ExternalSourceOpenFoodFacts,
	})
	if err == nil {
		t.Fatal("expected an unconfigured contributor to refuse")
	}
}

func TestDisplayNameNamesEachDatabase(t *testing.T) {
	got := map[ExternalSource]string{
		ExternalSourceOpenFoodFacts:     ExternalSourceOpenFoodFacts.DisplayName(),
		ExternalSourceOpenProductsFacts: ExternalSourceOpenProductsFacts.DisplayName(),
		ExternalSourceOpenBeautyFacts:   ExternalSourceOpenBeautyFacts.DisplayName(),
		ExternalSourceOpenPetFoodFacts:  ExternalSourceOpenPetFoodFacts.DisplayName(),
		ExternalSource("other"):         ExternalSource("other").DisplayName(),
	}
	want := map[ExternalSource]string{
		ExternalSourceOpenFoodFacts:     "Open Food Facts",
		ExternalSourceOpenProductsFacts: "Open Products Facts",
		ExternalSourceOpenBeautyFacts:   "Open Beauty Facts",
		ExternalSourceOpenPetFoodFacts:  "Open Pet Food Facts",
		ExternalSource("other"):         "the open database",
	}
	for source, name := range want {
		if got[source] != name {
			t.Errorf("%s display name = %q, want %q", source, got[source], name)
		}
	}
}

func TestSafeWriteDetail(t *testing.T) {
	if got := safeWriteDetail("", "secret"); got != "the open database did not accept this product" {
		t.Fatalf("empty detail = %q", got)
	}
	if got := safeWriteDetail("fields saved", "secret"); got != "fields saved" {
		t.Fatalf("verbose detail = %q", got)
	}
}
