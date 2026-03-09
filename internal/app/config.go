package app

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BindAddr               string
	DBPath                 string
	StaticDir              string
	LastRunLogPath         string
	BuildVersion           string
	BuildCommit            string
	BuildTime              string
	ThornodeEndpoints      []string
	MidgardEndpoints       []string
	ChainTrackerProviders  map[string]string
	ChainTrackerOverrides  map[string]string
	ChainTrackerCandidates map[string][]string
	BlockscoutAPIURLs      map[string]string
	BlockscoutAPIKeys      map[string]string
	UtxoTrackerURLs        map[string]string
	CosmosTrackerURLs      map[string]string
	EtherscanAPIURL        string
	EtherscanAPIKey        string
	EthplorerAPIURL        string
	EthplorerAPIKey        string
	AvaCloudBaseURL        string
	AvaCloudAPIKey         string
	NodeRealBSCURL         string
	NodeRealAPIKey         string
	SolanaRPCURL           string
	TronGridURL            string
	TronGridAPIKey         string
	XRPRPCURL              string
	RequestTimeout         time.Duration
	MidgardTimeout         time.Duration
}

// loadDotEnv reads a .env file and sets any variables not already in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // missing .env is fine
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		// Don't override existing env vars
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	log.Printf("loaded env from %s", path)
}

func LoadConfigFromEnv() Config {
	cwd, _ := os.Getwd()
	defaultDB := "data/chain-analysis.db"
	defaultStatic := "internal/web/static"
	defaultLastRunLog := "data/logs/actor-tracker-last-run.log"
	if strings.HasSuffix(filepath.ToSlash(cwd), "/THORChain") {
		loadDotEnv("chain-analysis-app/.env")
		defaultDB = "chain-analysis-app/data/chain-analysis.db"
		defaultStatic = "chain-analysis-app/internal/web/static"
		defaultLastRunLog = "chain-analysis-app/data/logs/actor-tracker-last-run.log"
	} else {
		loadDotEnv(".env")
	}

	cfg := Config{
		BindAddr:               getEnv("CHAIN_ANALYSIS_ADDR", ":8090"),
		DBPath:                 getEnv("CHAIN_ANALYSIS_DB", defaultDB),
		StaticDir:              getEnv("CHAIN_ANALYSIS_STATIC_DIR", defaultStatic),
		LastRunLogPath:         getEnv("CHAIN_ANALYSIS_LAST_RUN_LOG", defaultLastRunLog),
		BuildVersion:           getEnv("CHAIN_ANALYSIS_BUILD_VERSION", "dev"),
		BuildCommit:            getEnv("CHAIN_ANALYSIS_BUILD_COMMIT", "unknown"),
		BuildTime:              getEnv("CHAIN_ANALYSIS_BUILD_TIME", "unknown"),
		ChainTrackerProviders:  defaultChainTrackerProviders(),
		ChainTrackerOverrides:  map[string]string{},
		ChainTrackerCandidates: defaultChainTrackerCandidates(),
		BlockscoutAPIURLs:      map[string]string{},
		BlockscoutAPIKeys:      map[string]string{},
		UtxoTrackerURLs:        map[string]string{},
		CosmosTrackerURLs:      map[string]string{},
		EtherscanAPIURL:        strings.TrimRight(getEnv("CHAIN_ANALYSIS_ETHERSCAN_API_URL", "https://api.etherscan.io/v2/api"), "/"),
		EtherscanAPIKey:        strings.TrimSpace(os.Getenv("CHAIN_ANALYSIS_ETHERSCAN_API_KEY")),
		EthplorerAPIURL:        strings.TrimRight(getEnv("CHAIN_ANALYSIS_ETHPLORER_API_URL", "https://api.ethplorer.io"), "/"),
		EthplorerAPIKey:        strings.TrimSpace(getEnv("CHAIN_ANALYSIS_ETHPLORER_API_KEY", "freekey")),
		AvaCloudBaseURL:        strings.TrimRight(getEnv("CHAIN_ANALYSIS_AVACLOUD_BASE_URL", "https://glacier-api.avax.network"), "/"),
		AvaCloudAPIKey:         strings.TrimSpace(os.Getenv("CHAIN_ANALYSIS_AVACLOUD_API_KEY")),
		NodeRealBSCURL:         strings.TrimRight(getEnv("CHAIN_ANALYSIS_NODEREAL_BSC_URL", "https://bsc-mainnet.nodereal.io/v1"), "/"),
		NodeRealAPIKey:         strings.TrimSpace(os.Getenv("CHAIN_ANALYSIS_NODEREAL_API_KEY")),
		SolanaRPCURL:           strings.TrimRight(getEnv("CHAIN_ANALYSIS_SOLANA_RPC_URL", "https://api.mainnet-beta.solana.com|https://solana-rpc.publicnode.com"), "/"),
		TronGridURL:            strings.TrimRight(getEnv("CHAIN_ANALYSIS_TRONGRID_URL", "https://api.trongrid.io"), "/"),
		TronGridAPIKey:         strings.TrimSpace(os.Getenv("CHAIN_ANALYSIS_TRONGRID_API_KEY")),
		XRPRPCURL:              strings.TrimRight(getEnv("CHAIN_ANALYSIS_XRP_RPC_URL", "https://s1.ripple.com:51234|https://s2.ripple.com:51234|https://xrplcluster.com"), "/"),
		RequestTimeout:         time.Duration(getIntEnv("CHAIN_ANALYSIS_TIMEOUT_SECONDS", 20)) * time.Second,
		MidgardTimeout:         time.Duration(getIntEnv("CHAIN_ANALYSIS_MIDGARD_TIMEOUT_SECONDS", 10)) * time.Second,
	}

	thornodeEndpoints := getEnv("THORNODE_ENDPOINTS", "https://thornode.ninerealms.com,https://thornode.thorchain.liquify.com")
	for _, raw := range strings.Split(thornodeEndpoints, ",") {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		cfg.ThornodeEndpoints = append(cfg.ThornodeEndpoints, strings.TrimRight(v, "/"))
	}

	midgardEndpoints := getEnv("MIDGARD_ENDPOINTS", "https://midgard.ninerealms.com/v2,https://midgard.thorchain.liquify.com/v2")
	for _, raw := range strings.Split(midgardEndpoints, ",") {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		cfg.MidgardEndpoints = append(cfg.MidgardEndpoints, strings.TrimRight(v, "/"))
	}

	cfg.UtxoTrackerURLs = parseChainURLMapEnv(
		getEnv("CHAIN_ANALYSIS_UTXO_TRACKERS", "BTC=https://blockstream.info/api|https://mempool.space/api,LTC=https://litecoinspace.org/api,BCH=https://bchexplorer.cash/api,DOGE=https://explorer.doged.io"),
	)
	cfg.BlockscoutAPIURLs = parseChainURLMapEnv(
		getEnv("CHAIN_ANALYSIS_BLOCKSCOUT_API_URLS", "BASE=https://base.blockscout.com/api"),
	)
	cfg.CosmosTrackerURLs = parseChainURLMapEnv(
		getEnv("CHAIN_ANALYSIS_COSMOS_TRACKERS", "GAIA=https://rest.cosmos.directory/cosmoshub|https://rest.cosmoshub-main.ccvalidators.com|https://rest.lavenderfive.com/cosmoshub"),
	)
	cfg.BlockscoutAPIKeys = parseChainValueMapEnv(os.Getenv("CHAIN_ANALYSIS_BLOCKSCOUT_API_KEYS"))
	cfg.ChainTrackerOverrides = parseChainProviderMapEnv(os.Getenv("CHAIN_ANALYSIS_CHAIN_TRACKERS"))
	cfg.ChainTrackerProviders = mergeChainStringMaps(
		cfg.ChainTrackerProviders,
		cfg.ChainTrackerOverrides,
	)
	cfg.ChainTrackerCandidates = mergeChainProviderCandidates(
		cfg.ChainTrackerCandidates,
		parseChainProviderCandidatesEnv(os.Getenv("CHAIN_ANALYSIS_CHAIN_TRACKER_CANDIDATES")),
	)

	return cfg
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func parseChainURLMapEnv(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		chain := strings.ToUpper(strings.TrimSpace(pieces[0]))
		url := strings.TrimRight(strings.TrimSpace(pieces[1]), "/")
		if chain == "" || url == "" {
			continue
		}
		out[chain] = url
	}
	return out
}

func parseURLListValue(raw string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	}) {
		url := strings.TrimRight(strings.TrimSpace(part), "/")
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

func expandChainURLMap(raw map[string]string) map[string][]string {
	if len(raw) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(raw))
	for chain, value := range raw {
		urls := parseURLListValue(value)
		if len(urls) == 0 {
			continue
		}
		out[strings.ToUpper(strings.TrimSpace(chain))] = urls
	}
	return out
}

func (c Config) etherscanAPIURLs() []string {
	return parseURLListValue(c.EtherscanAPIURL)
}

func (c Config) ethplorerAPIURLs() []string {
	return parseURLListValue(c.EthplorerAPIURL)
}

func (c Config) avaCloudBaseURLs() []string {
	return parseURLListValue(c.AvaCloudBaseURL)
}

func (c Config) nodeRealBSCURLs() []string {
	return parseURLListValue(c.NodeRealBSCURL)
}

func (c Config) solanaRPCURLs() []string {
	return parseURLListValue(c.SolanaRPCURL)
}

func (c Config) tronGridURLs() []string {
	return parseURLListValue(c.TronGridURL)
}

func (c Config) xrplRPCURLs() []string {
	return parseURLListValue(c.XRPRPCURL)
}

func (c Config) blockscoutAPIURLsForChain(chain string) []string {
	return parseURLListValue(c.BlockscoutAPIURLs[strings.ToUpper(strings.TrimSpace(chain))])
}

func (c Config) utxoTrackerURLsForChain(chain string) []string {
	return parseURLListValue(c.UtxoTrackerURLs[strings.ToUpper(strings.TrimSpace(chain))])
}

func (c Config) cosmosTrackerURLsForChain(chain string) []string {
	return parseURLListValue(c.CosmosTrackerURLs[strings.ToUpper(strings.TrimSpace(chain))])
}

func parseChainProviderMapEnv(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		chain := strings.ToUpper(strings.TrimSpace(pieces[0]))
		provider := strings.ToLower(strings.TrimSpace(pieces[1]))
		if chain == "" || provider == "" {
			continue
		}
		out[chain] = provider
	}
	return out
}

func parseChainProviderCandidatesEnv(raw string) map[string][]string {
	out := map[string][]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		chain := strings.ToUpper(strings.TrimSpace(pieces[0]))
		if chain == "" {
			continue
		}
		var providers []string
		for _, rawProvider := range strings.Split(pieces[1], "|") {
			provider := strings.ToLower(strings.TrimSpace(rawProvider))
			if provider == "" {
				continue
			}
			providers = append(providers, provider)
		}
		if len(providers) == 0 {
			continue
		}
		out[chain] = dedupeProviders(providers)
	}
	return out
}

func parseChainValueMapEnv(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		chain := strings.ToUpper(strings.TrimSpace(pieces[0]))
		value := strings.TrimSpace(pieces[1])
		if chain == "" || value == "" {
			continue
		}
		out[chain] = value
	}
	return out
}

func mergeChainStringMaps(base, overrides map[string]string) map[string]string {
	if base == nil {
		base = map[string]string{}
	}
	for chain, value := range overrides {
		chain = strings.ToUpper(strings.TrimSpace(chain))
		value = strings.TrimSpace(value)
		if chain == "" || value == "" {
			continue
		}
		base[chain] = value
	}
	return base
}

func mergeChainProviderCandidates(base, overrides map[string][]string) map[string][]string {
	if base == nil {
		base = map[string][]string{}
	}
	for chain, providers := range overrides {
		chain = strings.ToUpper(strings.TrimSpace(chain))
		if chain == "" {
			continue
		}
		base[chain] = dedupeProviders(providers)
	}
	return base
}

func defaultChainTrackerProviders() map[string]string {
	return map[string]string{
		"ETH":  "etherscan",
		"BSC":  "nodereal",
		"AVAX": "avacloud",
		"BASE": "blockscout",
		"BTC":  "utxo",
		"LTC":  "utxo",
		"BCH":  "utxo",
		"DOGE": "utxo",
		"GAIA": "cosmos",
		"SOL":  "solana",
		"TRON": "trongrid",
		"XRP":  "xrpl",
	}
}

func defaultChainTrackerCandidates() map[string][]string {
	return map[string][]string{
		"ETH":  {"etherscan"},
		"BSC":  {"nodereal"},
		"AVAX": {"avacloud", "etherscan"},
		"BASE": {"blockscout", "etherscan"},
		"BTC":  {"utxo"},
		"LTC":  {"utxo"},
		"BCH":  {"utxo"},
		"DOGE": {"utxo"},
		"GAIA": {"cosmos"},
		"SOL":  {"solana"},
		"TRON": {"trongrid"},
		"XRP":  {"xrpl"},
	}
}

func (c Config) trackerProviderForChain(chain string) string {
	providers := c.trackerProvidersForChain(chain)
	if len(providers) == 0 {
		return ""
	}
	return providers[0]
}

func (c Config) trackerProvidersForChain(chain string) []string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	switch chain {
	case "", "THOR":
		return nil
	}
	if provider := strings.ToLower(strings.TrimSpace(c.ChainTrackerOverrides[chain])); provider != "" {
		return filterSupportedProviders(chain, []string{provider})
	}
	if providers := filterSupportedProviders(chain, dedupeProviders(c.ChainTrackerCandidates[chain])); len(providers) > 0 {
		return providers
	}
	if provider := strings.ToLower(strings.TrimSpace(c.ChainTrackerProviders[chain])); provider != "" {
		return filterSupportedProviders(chain, []string{provider})
	}
	if providers := filterSupportedProviders(chain, dedupeProviders(defaultChainTrackerCandidates()[chain])); len(providers) > 0 {
		return providers
	}
	if provider := strings.ToLower(strings.TrimSpace(defaultChainTrackerProviders()[chain])); provider != "" {
		return filterSupportedProviders(chain, []string{provider})
	}
	return nil
}

func dedupeProviders(providers []string) []string {
	if len(providers) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(providers))
	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	return out
}

func filterSupportedProviders(chain string, providers []string) []string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if len(providers) == 0 || chain == "" {
		return nil
	}
	var out []string
	for _, provider := range dedupeProviders(providers) {
		if !providerSupportsChain(provider, chain) {
			continue
		}
		out = append(out, provider)
	}
	return out
}

func providerSupportsChain(provider, chain string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	switch provider {
	case "etherscan":
		return chain != "BSC"
	default:
		return true
	}
}
