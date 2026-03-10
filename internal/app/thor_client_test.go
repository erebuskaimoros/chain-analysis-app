package app

import (
	"reflect"
	"testing"
	"time"
)

func TestThorClientEndpointsForPathPrefersPrimaryForAllTimeActions(t *testing.T) {
	client := NewThorClient([]string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}, time.Second)

	path := "/actions?address=thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu&fromTimestamp=0&timestamp=1773125546&limit=50&offset=0"

	first := client.endpointsForPath(path)
	second := client.endpointsForPath(path)

	want := []string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("first endpoint order = %v, want %v", first, want)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second endpoint order = %v, want %v", second, want)
	}
}

func TestThorClientEndpointsForPathRotatesForBoundedActions(t *testing.T) {
	client := NewThorClient([]string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}, time.Second)

	path := "/actions?address=thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu&fromTimestamp=1741589562&timestamp=1773125562&limit=50&offset=0"

	first := client.endpointsForPath(path)
	second := client.endpointsForPath(path)

	wantFirst := []string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}
	wantSecond := []string{
		"https://midgard.thorchain.liquify.com/v2",
		"https://midgard.ninerealms.com/v2",
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Fatalf("first endpoint order = %v, want %v", first, wantFirst)
	}
	if !reflect.DeepEqual(second, wantSecond) {
		t.Fatalf("second endpoint order = %v, want %v", second, wantSecond)
	}
}

func TestThorClientEndpointsForPathPrefersPrimaryForActionLookupWithoutTimeRange(t *testing.T) {
	client := NewThorClient([]string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}, time.Second)

	path := "/actions?txid=F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA"

	first := client.endpointsForPath(path)
	second := client.endpointsForPath(path)

	want := []string{
		"https://midgard.ninerealms.com/v2",
		"https://midgard.thorchain.liquify.com/v2",
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("first endpoint order = %v, want %v", first, want)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second endpoint order = %v, want %v", second, want)
	}
}
