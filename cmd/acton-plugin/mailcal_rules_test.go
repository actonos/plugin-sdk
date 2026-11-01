package main

import (
	"strings"
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
)

func validMail() sdk.PluginManifest {
	return sdk.PluginManifest{
		ID: "mailcal-x", Name: "X", Version: "1.0.0",
		Capabilities: []sdk.Capability{sdk.CapabilityMail, sdk.CapabilityCalendar},
		Permissions:  &sdk.Permissions{NetOutbound: []string{"gmail.googleapis.com"}, OAuth: []string{"google"}},
		OAuth: &sdk.OAuthConfig{
			Provider: "google",
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/gmail.modify"},
		},
	}
}

func TestAValidMailManifestHasNoErrors(t *testing.T) {
	if errs, _ := validateMailcal(validMail()); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestANonMailPluginIsNotChecked(t *testing.T) {
	m := sdk.PluginManifest{ID: "t", Name: "T", Version: "1.0.0",
		Capabilities: []sdk.Capability{sdk.CapabilityTool},
		Permissions:  &sdk.Permissions{Secrets: []string{"k"}, Storage: true}}
	if errs, _ := validateMailcal(m); len(errs) != 0 {
		t.Fatalf("a tool plugin may use secrets and storage: %v", errs)
	}
}

func TestMailManifestViolationsAreErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*sdk.PluginManifest)
		want   string
	}{
		{"bare wildcard host", func(m *sdk.PluginManifest) { m.Permissions.NetOutbound = []string{"*"} }, "bare '*'"},
		{"bare wildcard tcp", func(m *sdk.PluginManifest) { m.Permissions.NetTCP = []string{"*"} }, "bare '*'"},
		{"tcp without a port", func(m *sdk.PluginManifest) { m.Permissions.NetTCP = []string{"imap.example.com"} }, "host:port"},
		{"secrets", func(m *sdk.PluginManifest) { m.Permissions.Secrets = []string{"k"} }, "secrets"},
		{"storage", func(m *sdk.PluginManifest) { m.Permissions.Storage = true }, "storage"},
		{"workspace", func(m *sdk.PluginManifest) { m.Permissions.Workspace = true }, "workspace"},
		{"bus events", func(m *sdk.PluginManifest) { m.Permissions.BusEvents = []string{"x"} }, "bus_events"},
		{"oauth and credentials", func(m *sdk.PluginManifest) { m.Permissions.Credentials = true }, "both OAuth and password"},
		{"oauth block without permission", func(m *sdk.PluginManifest) { m.Permissions.OAuth = nil }, "permissions.oauth"},
		{"permission without oauth block", func(m *sdk.PluginManifest) { m.OAuth = nil }, "'oauth' block"},
		{"provider mismatch", func(m *sdk.PluginManifest) { m.Permissions.OAuth = []string{"microsoft"} }, "must match"},
		{"two oauth providers", func(m *sdk.PluginManifest) { m.Permissions.OAuth = []string{"google", "microsoft"} }, "must match"},
		{"plain http token url", func(m *sdk.PluginManifest) { m.OAuth.TokenURL = "http://oauth2.googleapis.com/token" }, "https"},
		{"no scopes", func(m *sdk.PluginManifest) { m.OAuth.Scopes = nil }, "scopes"},
		{"permanent delete scope hidden in a space-joined entry", func(m *sdk.PluginManifest) {
			m.OAuth.Scopes = []string{"https://www.googleapis.com/auth/gmail.modify https://mail.google.com/"}
		}, "permanent"},
		{"permanent delete scope with surrounding whitespace", func(m *sdk.PluginManifest) { m.OAuth.Scopes = []string{"  https://mail.google.com/\t"} }, "permanent"},
		{"permanent delete scope", func(m *sdk.PluginManifest) { m.OAuth.Scopes = []string{"https://mail.google.com/"} }, "permanent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validMail()
			tc.mutate(&m)
			errs, _ := validateMailcal(m)
			if !strings.Contains(strings.Join(errs, "\n"), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, errs)
			}
		})
	}
}

func TestAPasswordProviderIsValid(t *testing.T) {
	m := validMail()
	m.OAuth = nil
	m.Permissions.OAuth = nil
	m.Permissions.Credentials = true
	m.Permissions.NetTCP = []string{"imap.example.com:993"}
	if errs, _ := validateMailcal(m); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
