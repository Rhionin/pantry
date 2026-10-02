package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPublicProxyIsOptIn locks the deploy contract for internet access:
// a normal compose up stays LAN-only, and the public profile is the only
// thing that publishes 80/443 in front of the existing pantry service.
func TestPublicProxyIsOptIn(t *testing.T) {
	composePath := filepath.Join("..", "..", "deploy", "docker-compose.yml")
	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}

	var compose struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Profiles    []string          `yaml:"profiles"`
			Ports       []string          `yaml:"ports"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatalf("parse compose file: %v", err)
	}

	pantry, ok := compose.Services["pantry"]
	if !ok {
		t.Fatal("pantry service missing")
	}
	if len(pantry.Profiles) != 0 {
		t.Fatalf("pantry must stay in the default (LAN) project, got profiles %v", pantry.Profiles)
	}
	if !hasContainerPort(pantry.Ports, "8080") {
		t.Fatalf("pantry must still publish the container port 8080, got %v", pantry.Ports)
	}
	if !hasIPv4OnlyPort(pantry.Ports, "8080") {
		t.Fatalf("pantry must publish 8080 on 0.0.0.0 only so IPv6 does not expose it, got %v", pantry.Ports)
	}

	caddy, ok := compose.Services["caddy"]
	if !ok {
		t.Fatal("caddy service missing")
	}
	if len(caddy.Profiles) != 1 || caddy.Profiles[0] != "public" {
		t.Fatalf("caddy must be opt-in via the public profile, got %v", caddy.Profiles)
	}
	if caddy.Image != "caddy:2.11.4-alpine" {
		t.Fatalf("caddy image = %q, want pinned caddy:2.11.4-alpine", caddy.Image)
	}
	for _, want := range []string{"80:80", "443:443", "443:443/udp"} {
		if !hasExactPort(caddy.Ports, want) {
			t.Fatalf("caddy ports %v missing %q", caddy.Ports, want)
		}
	}
	if caddy.Environment["PUBLIC_HOST"] == "" || caddy.Environment["ACME_EMAIL"] == "" {
		t.Fatal("caddy must receive PUBLIC_HOST and ACME_EMAIL from the deployment .env")
	}
	if !hasExactPort(caddy.Volumes, "./auth.caddy:/etc/caddy/auth.caddy:ro") {
		t.Fatalf("caddy must mount the generated password hash, got %v", caddy.Volumes)
	}

	caddyfile, err := os.ReadFile(filepath.Join("..", "..", "deploy", "Caddyfile"))
	if err != nil {
		t.Fatalf("read Caddyfile: %v", err)
	}
	text := string(caddyfile)
	for _, want := range []string{
		"{$PUBLIC_HOST}",
		"{$ACME_EMAIL}",
		"import auth.caddy",
		"path /api/events",
		"reverse_proxy pantry:8080",
		"flush_interval -1",
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"max_size 1MB",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Caddyfile missing %q", want)
		}
	}

	envExample, err := os.ReadFile(filepath.Join("..", "..", "deploy", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	envText := string(envExample)
	if !strings.Contains(envText, "\nPUBLIC_HOST=\n") || !strings.Contains(envText, "\nACME_EMAIL=\n") {
		t.Fatal(".env.example must leave PUBLIC_HOST and ACME_EMAIL empty so install stays LAN-only")
	}
	if !strings.Contains(envText, "\nBASIC_AUTH_USER=pantry\n") || !strings.Contains(envText, "\nBASIC_AUTH_PASSWORD=\n") {
		t.Fatal(".env.example must set the public username and leave the shared password empty")
	}

	setup, err := os.ReadFile(filepath.Join("..", "..", "deploy", "setup.sh"))
	if err != nil {
		t.Fatalf("read setup.sh: %v", err)
	}
	setupText := string(setup)
	for _, want := range []string{"cmd_publish", "cmd_unpublish", "--profile public", "write_auth_caddy", "BASIC_AUTH_PASSWORD", "caddy hash-password", "basic_auth bcrypt Pantry"} {
		if !strings.Contains(setupText, want) {
			t.Fatalf("setup.sh missing %q", want)
		}
	}

	updater, err := os.ReadFile(filepath.Join("..", "..", "deploy", "systemd", "pantry-update.sh"))
	if err != nil {
		t.Fatalf("read pantry-update.sh: %v", err)
	}
	updaterText := string(updater)
	if !strings.Contains(updaterText, "--profile public") || !strings.Contains(updaterText, "PUBLIC_HOST") {
		t.Fatal("automatic updates must include the public profile only when PUBLIC_HOST is set")
	}
	if !strings.Contains(updaterText, "-f /opt/pantry/auth.caddy") {
		t.Fatal("automatic updates must not start the public proxy without the password hash file")
	}
	if !strings.Contains(setupText, "install_lan_guard") || !strings.Contains(setupText, "lan-guard.sh") {
		t.Fatal("setup.sh must install the LAN port filter")
	}
	guard, err := os.ReadFile(filepath.Join("..", "..", "deploy", "lan-guard.sh"))
	if err != nil {
		t.Fatalf("read lan-guard.sh: %v", err)
	}
	guardText := string(guard)
	for _, want := range []string{"DOCKER-USER", "pantry-lan-guard", "100.64.0.0/10", "--ctorigdstport"} {
		if !strings.Contains(guardText, want) {
			t.Fatalf("lan-guard.sh missing %q", want)
		}
	}
	if strings.Contains(setupText, "basic_auth /") {
		t.Fatal("basic auth must not be limited to a path; the whole public site needs the password")
	}

	ignore, err := os.ReadFile(filepath.Join("..", "..", ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(ignore), "deploy/auth.caddy") {
		t.Fatal(".gitignore must ignore deploy/auth.caddy so a generated hash is not committed")
	}
}

func hasExactPort(ports []string, want string) bool {
	for _, port := range ports {
		if port == want {
			return true
		}
	}
	return false
}

func hasIPv4OnlyPort(ports []string, containerPort string) bool {
	wantSuffix := ":" + containerPort
	for _, port := range ports {
		if strings.HasPrefix(port, "0.0.0.0:") && strings.HasSuffix(port, wantSuffix) && !strings.Contains(port, "[::]") {
			return true
		}
	}
	return false
}

func hasContainerPort(ports []string, containerPort string) bool {
	suffix := ":" + containerPort
	for _, port := range ports {
		if port == containerPort || strings.HasSuffix(port, suffix) {
			return true
		}
	}
	return false
}
