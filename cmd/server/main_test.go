package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestComposeDefaultsMatchMainGo verifies that the defaults in deploy/docker-compose.yml
// match the defaults used by main.go, preventing cross-artifact drift.
func TestComposeDefaultsMatchMainGo(t *testing.T) {
	// Read the compose file
	composeFile := filepath.Join("..", "..", "deploy", "docker-compose.yml")
	composeData, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatalf("Failed to read compose file: %v", err)
	}

	// Parse the compose file
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal(composeData, &compose); err != nil {
		t.Fatalf("Failed to parse compose file: %v", err)
	}

	pantryService, exists := compose.Services["pantry"]
	if !exists {
		t.Fatal("pantry service not found in compose file")
	}

	// Test cases mapping compose defaults to Go constants/defaults
	testCases := []struct {
		name           string
		envVar         string
		composeDefault string
		goDefault      interface{}
	}{
		{
			name:           "PRODUCT_CACHE_TTL",
			envVar:         "PRODUCT_CACHE_TTL",
			composeDefault: extractDefault(t, pantryService.Environment["PRODUCT_CACHE_TTL"]),
			goDefault:      defaultProductCacheTTL,
		},
		{
			name:           "PRODUCT_MISS_TTL",
			envVar:         "PRODUCT_MISS_TTL",
			composeDefault: extractDefault(t, pantryService.Environment["PRODUCT_MISS_TTL"]),
			goDefault:      defaultMissTTL,
		},
		{
			name:           "DISABLE_EXTERNAL_PRODUCT_LOOKUP",
			envVar:         "DISABLE_EXTERNAL_PRODUCT_LOOKUP",
			composeDefault: extractDefault(t, pantryService.Environment["DISABLE_EXTERNAL_PRODUCT_LOOKUP"]),
			goDefault:      "false", // main.go checks != "true", so default is effectively false
		},
		{
			name:           "STOCK_IN_CONTROL_BARCODE",
			envVar:         "STOCK_IN_CONTROL_BARCODE",
			composeDefault: extractDefault(t, pantryService.Environment["STOCK_IN_CONTROL_BARCODE"]),
			goDefault:      "STOCK_IN", // from loadScanListenerConfig
		},
		{
			name:           "STOCK_OUT_CONTROL_BARCODE",
			envVar:         "STOCK_OUT_CONTROL_BARCODE",
			composeDefault: extractDefault(t, pantryService.Environment["STOCK_OUT_CONTROL_BARCODE"]),
			goDefault:      "STOCK_OUT", // from loadScanListenerConfig
		},
		{
			name:           "HEADLESS_USER_ID",
			envVar:         "HEADLESS_USER_ID",
			composeDefault: extractDefault(t, pantryService.Environment["HEADLESS_USER_ID"]),
			goDefault:      "user-1", // from loadScanListenerConfig
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			switch expected := tc.goDefault.(type) {
			case time.Duration:
				// Convert duration to string for comparison
				expectedStr := expected.String()
				if tc.composeDefault != expectedStr {
					t.Errorf("Compose default %q for %s doesn't match Go default %q",
						tc.composeDefault, tc.envVar, expectedStr)
				}
			case string:
				if tc.composeDefault != expected {
					t.Errorf("Compose default %q for %s doesn't match Go default %q",
						tc.composeDefault, tc.envVar, expected)
				}
			default:
				t.Errorf("Unsupported type %T for %s", expected, tc.envVar)
			}
		})
	}

	// Verify the duration parsing matches by actually parsing the compose defaults
	t.Run("DurationParsing", func(t *testing.T) {
		cacheTTLStr := extractDefault(t, pantryService.Environment["PRODUCT_CACHE_TTL"])
		cacheTTL, err := time.ParseDuration(cacheTTLStr)
		if err != nil {
			t.Errorf("Compose PRODUCT_CACHE_TTL %q is not a valid duration: %v", cacheTTLStr, err)
		} else if cacheTTL != defaultProductCacheTTL {
			t.Errorf("Parsed compose PRODUCT_CACHE_TTL %s != Go default %s", cacheTTL, defaultProductCacheTTL)
		}

		missTTLStr := extractDefault(t, pantryService.Environment["PRODUCT_MISS_TTL"])
		missTTL, err := time.ParseDuration(missTTLStr)
		if err != nil {
			t.Errorf("Compose PRODUCT_MISS_TTL %q is not a valid duration: %v", missTTLStr, err)
		} else if missTTL != defaultMissTTL {
			t.Errorf("Parsed compose PRODUCT_MISS_TTL %s != Go default %s", missTTL, defaultMissTTL)
		}
	})
}

// extractDefault extracts the default value from a ${VAR:-default} environment variable string
func extractDefault(t *testing.T, envValue string) string {
	t.Helper()

	// Expected format: ${VAR:-default}
	if !strings.HasPrefix(envValue, "${") || !strings.HasSuffix(envValue, "}") {
		t.Fatalf("Environment variable %q is not in expected ${VAR:-default} format", envValue)
	}

	// Remove ${ and }
	inner := envValue[2 : len(envValue)-1]

	// Split on :-
	parts := strings.SplitN(inner, ":-", 2)
	if len(parts) != 2 {
		t.Fatalf("Environment variable %q does not contain :- separator", envValue)
	}

	return parts[1]
}
