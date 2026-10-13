package push

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceWorkerShowsTheScanQueue(t *testing.T) {
	path := filepath.Join("..", "..", "frontend", "public", "sw.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read service worker: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"addEventListener('push'",
		"showNotification",
		"addEventListener('notificationclick'",
		"openWindow",
		"pantry-scan-alert",
		"alertPath",
		"requireInteraction: true",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("service worker missing %q", want)
		}
	}
	if strings.Contains(source, "import ") {
		t.Fatal("the service worker must stay a classic script so every browser can install it")
	}
}
