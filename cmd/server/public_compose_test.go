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
			SecurityOpt []string          `yaml:"security_opt"`
			CapDrop     []string          `yaml:"cap_drop"`
			ReadOnly    bool              `yaml:"read_only"`
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
	if len(pantry.Ports) != 1 || !strings.HasPrefix(pantry.Ports[0], "0.0.0.0:") {
		t.Fatalf("pantry must publish IPv4 only so a global IPv6 address is not an open API, got %v", pantry.Ports)
	}
	if !hasExactPort(pantry.SecurityOpt, "no-new-privileges:true") {
		t.Fatalf("pantry security_opt = %v", pantry.SecurityOpt)
	}
	if !hasExactPort(pantry.CapDrop, "ALL") {
		t.Fatalf("pantry cap_drop = %v", pantry.CapDrop)
	}
	if !pantry.ReadOnly {
		t.Fatal("pantry root filesystem must be read-only; the database volume stays writable")
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
	if !hasExactPort(caddy.SecurityOpt, "no-new-privileges:true") {
		t.Fatalf("caddy security_opt = %v", caddy.SecurityOpt)
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
		"path /api/telemetry /api/telemetry/client",
		"path /brand/logo.png /terms /privacy",
		"path /api/events",
		"Do not add other API paths to this matcher.",
		"This is not an API exception.",
		"reverse_proxy pantry:8080",
		"flush_interval -1",
		"route {",
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"camera=(self)",
		"admin off",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Caddyfile missing %q", want)
		}
	}
	authAt := strings.Index(text, "import auth.caddy")
	if authAt < 0 {
		t.Fatal("Caddyfile must import auth.caddy")
	}
	// Only these exact paths are reachable before basic auth. Anything else,
	// including /favicon.ico, /health, and the rest of /brand, stays in the
	// password handle.
	publicMatchers := pathMatchers(text[:authAt])
	wantPublic := []string{
		"path /api/telemetry /api/telemetry/client",
		"path /brand/logo.png /terms /privacy",
	}
	if strings.Join(publicMatchers, "\n") != strings.Join(wantPublic, "\n") {
		t.Fatalf("public path matchers = %#v, want %#v", publicMatchers, wantPublic)
	}
	for _, matcher := range publicMatchers {
		if strings.Contains(matcher, "*") {
			t.Fatalf("public matcher %q is a prefix", matcher)
		}
	}
	telemetryMatcher := publicMatchers[0]
	for _, legal := range []string{"/brand/", "/terms", "/privacy"} {
		if strings.Contains(telemetryMatcher, legal) {
			t.Fatalf("telemetry matcher must not include %s", legal)
		}
	}
	authedMatchers := pathMatchers(text[authAt:])
	if strings.Join(authedMatchers, "\n") != "path /api/events" {
		t.Fatalf("path matchers after basic auth = %#v, want only /api/events", authedMatchers)
	}
	beforeAuth := text[:authAt]
	for _, closed := range []string{"/api/inventory", "/api/scans", "/api/events", "/health", "/favicon.ico"} {
		if strings.Contains(beforeAuth, closed) {
			t.Fatalf("path %s is outside basic auth", closed)
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
	for _, want := range []string{
		"cmd_apply",
		"cmd_publish",
		"cmd_unpublish",
		"cmd_firewall",
		"--profile public",
		"write_auth_caddy",
		"BASIC_AUTH_PASSWORD",
		"caddy hash-password",
		"basic_auth bcrypt Pantry",
		"Keeping existing",
		"PANTRY_LAN_FIREWALL",
		"${1:-apply}",
		"pantry.local",
		"Home Wi-Fi cannot open https://",
		"does not hairpin NAT",
	} {
		if !strings.Contains(setupText, want) {
			t.Fatalf("setup.sh missing %q", want)
		}
	}
	if strings.Contains(setupText, "read -p") || strings.Contains(setupText, "read -s") {
		t.Fatal("setup.sh must not prompt for BASIC_AUTH_PASSWORD; an existing auth.caddy is kept")
	}

	if !strings.Contains(envText, "\nPANTRY_LAN_FIREWALL=on\n") {
		t.Fatal(".env.example must default the LAN firewall on so setup can opt out explicitly")
	}
	if !strings.Contains(envText, "\nPANTRY_LAN_IPV4=\n") {
		t.Fatal(".env.example must leave PANTRY_LAN_IPV4 empty so detection stays the default")
	}
	if !strings.Contains(envText, "\nPANTRY_SPLIT_DNS=off\n") {
		t.Fatal(".env.example must leave split-horizon DNS off so setup does not start a resolver")
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

	ignore, err := os.ReadFile(filepath.Join("..", "..", ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(ignore), "deploy/auth.caddy") {
		t.Fatal(".gitignore must ignore deploy/auth.caddy so a generated hash is not committed")
	}

	readme, err := os.ReadFile(filepath.Join("..", "..", "deploy", "README.md"))
	if err != nil {
		t.Fatalf("read deploy README: %v", err)
	}
	readmeText := string(readme)
	for _, want := range []string{
		"Home Wi-Fi hangs on the public name",
		"http://pantry.local:8080",
		"Gryphon Connect",
		"NAT loopback",
		"PANTRY_SPLIT_DNS=on",
		"Do not forward port 53",
		"Do not forward port 8080",
	} {
		if !strings.Contains(readmeText, want) {
			t.Fatalf("deploy README missing %q", want)
		}
	}
	if !strings.Contains(text, "Do not publish port 8080") {
		t.Fatal("Caddyfile must keep the warning that port 8080 is not the hairpin workaround")
	}
}

// pathMatchers returns every Caddy `path` matcher, in source order.
// Comment lines are skipped so a note about a path is not treated as a route.
func pathMatchers(text string) []string {
	var got []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		const marker = " path "
		if i := strings.Index(line, marker); i >= 0 {
			got = append(got, strings.TrimSpace(line[i+1:]))
			continue
		}
		if strings.HasPrefix(line, "path ") {
			got = append(got, line)
		}
	}
	return got
}

func hasExactPort(ports []string, want string) bool {
	for _, port := range ports {
		if port == want {
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
