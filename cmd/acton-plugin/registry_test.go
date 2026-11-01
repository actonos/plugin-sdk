package main

import (
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
)

func TestMailPluginsAreRegisteredAsConnectors(t *testing.T) {
	cases := []struct {
		caps []sdk.Capability
		want string
	}{
		{[]sdk.Capability{sdk.CapabilityMail, sdk.CapabilityCalendar}, "connector"},
		{[]sdk.Capability{sdk.CapabilityCalendar}, "connector"},
		{[]sdk.Capability{sdk.CapabilityChannel}, ""},
		{[]sdk.Capability{sdk.CapabilityTool}, ""},
	}
	for _, c := range cases {
		if got := categoryFor(c.caps); got != c.want {
			t.Errorf("categoryFor(%v) = %q, want %q", c.caps, got, c.want)
		}
	}
}
