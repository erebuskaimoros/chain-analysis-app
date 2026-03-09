package app

import "testing"

func TestParseChainProviderMapEnv(t *testing.T) {
	got := parseChainProviderMapEnv("base= BlockScout ; avax = AvaCloud, tron=trongrid")
	if got["BASE"] != "blockscout" {
		t.Fatalf("expected BASE provider to normalize to blockscout, got %q", got["BASE"])
	}
	if got["AVAX"] != "avacloud" {
		t.Fatalf("expected AVAX provider to normalize to avacloud, got %q", got["AVAX"])
	}
	if got["TRON"] != "trongrid" {
		t.Fatalf("expected TRON provider to normalize to trongrid, got %q", got["TRON"])
	}
}

func TestParseChainValueMapEnv(t *testing.T) {
	got := parseChainValueMapEnv("base=abc123 ; avax = glacier-key")
	if got["BASE"] != "abc123" {
		t.Fatalf("expected BASE value to stay intact, got %q", got["BASE"])
	}
	if got["AVAX"] != "glacier-key" {
		t.Fatalf("expected AVAX value to stay intact, got %q", got["AVAX"])
	}
}

func TestParseChainProviderCandidatesEnv(t *testing.T) {
	got := parseChainProviderCandidatesEnv("bsc= NodeReal | etherScan ; base = blockscout|etherscan")
	if len(got["BSC"]) != 2 || got["BSC"][0] != "nodereal" || got["BSC"][1] != "etherscan" {
		t.Fatalf("unexpected BSC candidates: %#v", got["BSC"])
	}
	if len(got["BASE"]) != 2 || got["BASE"][0] != "blockscout" || got["BASE"][1] != "etherscan" {
		t.Fatalf("unexpected BASE candidates: %#v", got["BASE"])
	}
}

func TestParseURLListValue(t *testing.T) {
	got := parseURLListValue(" https://one.example/ | https://two.example ; https://one.example ")
	if len(got) != 2 {
		t.Fatalf("expected 2 urls, got %#v", got)
	}
	if got[0] != "https://one.example" || got[1] != "https://two.example" {
		t.Fatalf("unexpected url list: %#v", got)
	}
}

func TestExpandChainURLMap(t *testing.T) {
	got := expandChainURLMap(map[string]string{
		"btc": "https://a.example/api|https://b.example/api",
	})
	urls := got["BTC"]
	if len(urls) != 2 {
		t.Fatalf("expected 2 expanded urls, got %#v", got)
	}
	if urls[0] != "https://a.example/api" || urls[1] != "https://b.example/api" {
		t.Fatalf("unexpected expanded urls: %#v", urls)
	}
}

func TestTrackerProviderForChainDefaults(t *testing.T) {
	cfg := Config{}
	cases := map[string]string{
		"":     "",
		"THOR": "",
		"ETH":  "etherscan",
		"BSC":  "nodereal",
		"AVAX": "avacloud",
		"BASE": "blockscout",
		"BTC":  "utxo",
		"DOGE": "utxo",
		"GAIA": "cosmos",
		"SOL":  "solana",
		"TRON": "trongrid",
		"XRP":  "xrpl",
		"NOPE": "",
	}
	for chain, want := range cases {
		if got := cfg.trackerProviderForChain(chain); got != want {
			t.Fatalf("trackerProviderForChain(%q) = %q, want %q", chain, got, want)
		}
	}
}

func TestTrackerProviderForChainOverrides(t *testing.T) {
	cfg := Config{
		ChainTrackerOverrides: parseChainProviderMapEnv("BASE=blockscout,AVAX=avacloud"),
		ChainTrackerProviders: mergeChainStringMaps(
			defaultChainTrackerProviders(),
			parseChainProviderMapEnv("BASE=blockscout,AVAX=avacloud"),
		),
	}

	if got := cfg.trackerProviderForChain("BASE"); got != "blockscout" {
		t.Fatalf("expected BASE override to use blockscout, got %q", got)
	}
	if got := cfg.trackerProviderForChain("AVAX"); got != "avacloud" {
		t.Fatalf("expected AVAX override to use avacloud, got %q", got)
	}
	if got := cfg.trackerProviderForChain("BSC"); got != "nodereal" {
		t.Fatalf("expected BSC to keep default nodereal provider, got %q", got)
	}
}

func TestTrackerProvidersForChainCandidates(t *testing.T) {
	cfg := Config{}
	got := cfg.trackerProvidersForChain("BSC")
	if len(got) != 1 || got[0] != "nodereal" {
		t.Fatalf("unexpected BSC provider candidates: %#v", got)
	}
}

func TestTrackerProvidersForChainFiltersUnsupportedFallbacks(t *testing.T) {
	cfg := Config{
		ChainTrackerCandidates: parseChainProviderCandidatesEnv("BSC=nodereal|etherscan"),
	}

	got := cfg.trackerProvidersForChain("BSC")
	if len(got) != 1 || got[0] != "nodereal" {
		t.Fatalf("expected BSC candidates to drop unsupported etherscan fallback, got %#v", got)
	}
}
