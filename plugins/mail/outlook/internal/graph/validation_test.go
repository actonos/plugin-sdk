package graph_test

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

// An event that does not end after it starts is invalid whatever the server would say, so the
// plugin must refuse it before making a network call.
func TestEventsThatDoNotEndAfterTheyStartAreInvalid(t *testing.T) {
	_, b := seeded(t)
	start := time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC)
	cases := map[string]abi.Event{
		"zero value":       {},
		"end before start": {Title: "x", Start: start, End: start.Add(-time.Hour)},
		"zero duration":    {Title: "x", Start: start, End: start},
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := b.CalendarCreate(context.Background(), acct, abi.CalWriteParams{Event: ev}); !mailcal.IsCode(err, abi.ErrInvalid) {
				t.Errorf("create: want invalid, got %v", err)
			}
			ev.ID = "e1"
			if _, err := b.CalendarUpdate(context.Background(), acct, abi.CalWriteParams{Event: ev}); !mailcal.IsCode(err, abi.ErrInvalid) {
				t.Errorf("update: want invalid, got %v", err)
			}
		})
	}
}
