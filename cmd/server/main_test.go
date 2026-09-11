package main

import (
	"os"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// productCacheTTL
// --------------------------------------------------------------------------

// Feature: product-cache-freshness, Property 16: TTL configuration is a total function
//
// Validates: Requirements 6.1, 6.2, 6.3
//
// productCacheTTL reads PRODUCT_CACHE_TTL and always returns a duration
// without panicking, calling log.Fatal, or exiting: unset or empty yields the
// default, a valid Go duration is returned exactly as parsed (even when
// non-positive), and an unparseable value falls back to the default.
func TestProductCacheTTL(t *testing.T) {
	tests := []struct {
		name  string
		unset bool
		value string
		want  time.Duration
	}{
		{
			name:  "unset uses default",
			unset: true,
			want:  defaultProductCacheTTL,
		},
		{
			name:  "empty uses default",
			value: "",
			want:  defaultProductCacheTTL,
		},
		{
			name:  "one hour",
			value: "1h",
			want:  time.Hour,
		},
		{
			name:  "twenty four hours",
			value: "24h",
			want:  24 * time.Hour,
		},
		{
			name:  "zero duration is used as given, not defaulted",
			value: "0s",
			want:  0,
		},
		{
			name:  "seven hundred twenty hours",
			value: "720h",
			want:  720 * time.Hour,
		},
		{
			name:  "unparseable word falls back to default",
			value: "not-a-duration",
			want:  defaultProductCacheTTL,
		},
		{
			name:  "bare number with no unit falls back to default",
			value: "5",
			want:  defaultProductCacheTTL,
		},
		{
			name:  "unit with a space falls back to default",
			value: "1 day",
			want:  defaultProductCacheTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				original, wasSet := os.LookupEnv("PRODUCT_CACHE_TTL")
				os.Unsetenv("PRODUCT_CACHE_TTL")
				t.Cleanup(func() {
					if wasSet {
						os.Setenv("PRODUCT_CACHE_TTL", original)
					}
				})
			} else {
				t.Setenv("PRODUCT_CACHE_TTL", tt.value)
			}

			got := productCacheTTL()

			if got != tt.want {
				t.Errorf("productCacheTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// loadScanListenerConfig
// --------------------------------------------------------------------------

// Feature: background-scan-listener
//
// Validates: Requirements 2.7, 4.2
//
// loadScanListenerConfig reads the stock-in/stock-out control barcodes and
// headless user ID from the environment, defaulting each when unset, and
// refuses to build a listener when the two control barcodes are identical.
func TestLoadScanListenerConfig(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		listener, ok := loadScanListenerConfig()

		if !ok {
			t.Fatal("loadScanListenerConfig() ok = false, want true")
		}
		if listener.StockInBarcode != "STOCK_IN" {
			t.Errorf("StockInBarcode = %q, want %q", listener.StockInBarcode, "STOCK_IN")
		}
		if listener.StockOutBarcode != "STOCK_OUT" {
			t.Errorf("StockOutBarcode = %q, want %q", listener.StockOutBarcode, "STOCK_OUT")
		}
		if listener.HeadlessUserID != "user-1" {
			t.Errorf("HeadlessUserID = %q, want %q", listener.HeadlessUserID, "user-1")
		}
	})

	t.Run("explicit env values override defaults", func(t *testing.T) {
		t.Setenv("STOCK_IN_CONTROL_BARCODE", "IN-123")
		t.Setenv("STOCK_OUT_CONTROL_BARCODE", "OUT-456")
		t.Setenv("HEADLESS_USER_ID", "user-headless")

		listener, ok := loadScanListenerConfig()

		if !ok {
			t.Fatal("loadScanListenerConfig() ok = false, want true")
		}
		if listener.StockInBarcode != "IN-123" {
			t.Errorf("StockInBarcode = %q, want %q", listener.StockInBarcode, "IN-123")
		}
		if listener.StockOutBarcode != "OUT-456" {
			t.Errorf("StockOutBarcode = %q, want %q", listener.StockOutBarcode, "OUT-456")
		}
		if listener.HeadlessUserID != "user-headless" {
			t.Errorf("HeadlessUserID = %q, want %q", listener.HeadlessUserID, "user-headless")
		}
	})

	t.Run("equal control barcodes yield ok=false", func(t *testing.T) {
		t.Setenv("STOCK_IN_CONTROL_BARCODE", "SAME")
		t.Setenv("STOCK_OUT_CONTROL_BARCODE", "SAME")

		listener, ok := loadScanListenerConfig()

		if ok {
			t.Fatal("loadScanListenerConfig() ok = true, want false")
		}
		if listener != nil {
			t.Errorf("listener = %v, want nil", listener)
		}
	})
}
