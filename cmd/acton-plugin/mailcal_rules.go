package main

import (
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

// permanentDeleteScope lets a plugin delete a Gmail mailbox's mail for good; providers must not ask for it.
const permanentDeleteScope = "https://mail.google.com/"

func isMailcal(m sdk.PluginManifest) bool {
	for _, c := range m.Capabilities {
		if c == sdk.CapabilityMail || c == sdk.CapabilityCalendar {
			return true
		}
	}
	return false
}

// validateMailcal applies the rules the ActonOS core enforces on mail and calendar provider
// manifests. It returns nothing for any other kind of plugin.
func validateMailcal(m sdk.PluginManifest) (errs, warns []string) {
	if !isMailcal(m) {
		return nil, nil
	}
	p := m.Permissions
	if p == nil {
		p = &sdk.Permissions{}
	}

	for _, h := range p.NetOutbound {
		if strings.TrimSpace(h) == "*" {
			errs = append(errs, "net_outbound must not contain a bare '*'; list the provider's hosts (or '*.domain' when needed)")
		}
	}
	for _, h := range p.NetTCP {
		h = strings.TrimSpace(h)
		switch {
		case h == "*" || strings.HasPrefix(h, "*:"):
			errs = append(errs, "net_tcp must not contain a bare '*'; list 'host:port' endpoints")
		case !strings.Contains(h, ":"):
			errs = append(errs, fmt.Sprintf("net_tcp entry %q must be 'host:port'", h))
		}
	}

	if len(p.Secrets) > 0 {
		errs = append(errs, "mail/calendar providers must not request 'secrets'; the core hands out tokens and passwords itself")
	}
	if p.Storage {
		errs = append(errs, "mail/calendar providers must not request 'storage'")
	}
	if p.Workspace {
		errs = append(errs, "mail/calendar providers must not request 'workspace'")
	}
	if len(p.BusEvents) > 0 {
		errs = append(errs, "mail/calendar providers must not request 'bus_events'")
	}

	if p.Credentials && (m.OAuth != nil || len(p.OAuth) > 0) {
		errs = append(errs, "a plugin must not use both OAuth and password sign-in")
	}

	switch {
	case m.OAuth == nil && len(p.OAuth) > 0:
		errs = append(errs, "permissions.oauth is set but the manifest has no 'oauth' block")
	case m.OAuth != nil:
		o := m.OAuth
		if o.Provider == "" {
			errs = append(errs, "oauth.provider is required")
		}
		if len(p.OAuth) != 1 || p.OAuth[0] != o.Provider {
			errs = append(errs, fmt.Sprintf("permissions.oauth must match oauth.provider: want [%q], got %v", o.Provider, p.OAuth))
		}
		for _, f := range []struct{ name, url string }{{"auth_url", o.AuthURL}, {"token_url", o.TokenURL}} {
			if !strings.HasPrefix(f.url, "https://") {
				errs = append(errs, fmt.Sprintf("oauth.%s must be an https URL", f.name))
			}
		}
		if len(o.Scopes) == 0 {
			errs = append(errs, "oauth.scopes must list at least one scope (ask for the narrowest that works)")
		}
		for _, s := range o.Scopes {
			if s == permanentDeleteScope {
				errs = append(errs, "oauth.scopes must not include the permanent-delete scope "+permanentDeleteScope)
			}
		}
	}
	return errs, warns
}

// categoryFor picks the marketplace category for a registry entry. Mail and calendar providers
// are service integrations, so they list under "connector"; for every other plugin the core
// derives the category itself, so the field is left empty.
func categoryFor(caps []sdk.Capability) string {
	for _, c := range caps {
		if c == sdk.CapabilityMail || c == sdk.CapabilityCalendar {
			return "connector"
		}
	}
	return ""
}
