package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Label importers read source data a user downloaded (a git clone or release
// file) from a local path. Each import replaces that source's labels.

// LabelImportResult reports one import.
type LabelImportResult struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// ImportLabels imports labels for source from path. Sources: graphsense (a
// graphsense-tagpacks checkout or a packs directory), ofac (the lists branch
// of 0xB10C/ofac-sanctioned-digital-currency-addresses, or one list file),
// eth-labels (data/json/accounts.json from dawsbot/eth-labels), scamsniffer
// (blacklist/address.json from scamsniffer/scam-database).
func (a *App) ImportLabels(ctx context.Context, source, path string) (LabelImportResult, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	var (
		labels  []AddressLabel
		license string
		err     error
	)
	switch source {
	case labelSourceGraphSense:
		labels, err = readGraphSenseTagPacks(path)
		license = "MIT"
	case labelSourceOFAC:
		labels, err = readOFACLists(path)
		license = "public domain (OFAC SDN list)"
	case labelSourceEthLabels:
		labels, err = readEthLabels(path)
		license = "MIT"
	case labelSourceScamSniffer:
		labels, err = readScamSnifferAddresses(path)
		license = "GPL-3.0 (data not redistributed)"
	default:
		return LabelImportResult{}, fmt.Errorf("unknown label source %q (want graphsense, ofac, eth-labels, scamsniffer)", source)
	}
	if err != nil {
		return LabelImportResult{}, err
	}
	for i := range labels {
		labels[i].Source = source
	}
	count, err := replaceLabelSource(ctx, a.db, source, filepath.Base(path), license, labels)
	if err != nil {
		return LabelImportResult{}, err
	}
	return LabelImportResult{Source: source, Count: count}, nil
}

func readGraphSenseTagPacks(root string) ([]AddressLabel, error) {
	packs := root
	if info, err := os.Stat(filepath.Join(root, "packs")); err == nil && info.IsDir() {
		packs = filepath.Join(root, "packs")
	}
	var files []string
	err := filepath.WalkDir(packs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no TagPack .yaml files under %s", packs)
	}
	var out []AddressLabel
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		labels, err := parseTagPack(raw, filepath.Base(file))
		if err != nil {
			return nil, err
		}
		out = append(out, labels...)
	}
	return out, nil
}

var ofacListName = regexp.MustCompile(`sanctioned_addresses_([A-Z0-9]+)\.txt$`)

func readOFACLists(path string) ([]AddressLabel, error) {
	var files []string
	if info, err := os.Stat(path); err != nil {
		return nil, err
	} else if info.IsDir() {
		matches, _ := filepath.Glob(filepath.Join(path, "sanctioned_addresses_*.txt"))
		files = matches
	} else {
		files = []string{path}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no sanctioned_addresses_*.txt files at %s", path)
	}
	var out []AddressLabel
	for _, file := range files {
		asset := ""
		if m := ofacListName.FindStringSubmatch(filepath.Base(file)); m != nil {
			asset = m[1]
		}
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			address := strings.TrimSpace(scanner.Text())
			if address == "" || strings.HasPrefix(address, "#") {
				continue
			}
			chain := labelChain(asset, address)
			// Stablecoin lists hold token holder addresses on their host chains.
			if asset == "USDT" || asset == "USDC" {
				chain = normalizeChain("", address)
			}
			out = append(out, AddressLabel{
				Chain:      chain,
				Address:    address,
				Label:      "OFAC SDN listed",
				Category:   labelCategorySanctioned,
				SourceRef:  "OFAC SDN " + firstNonEmpty(asset, filepath.Base(file)),
				Confidence: tagPackConfidence["authority_data"],
			})
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

var ethLabelsChains = map[int]string{1: "ETH", 56: "BSC", 8453: "BASE", 43114: "AVAX", 42161: "ARB", 10: "OP", 137: "POLYGON"}

// ethLabelsExchanges are eth-labels slugs for centralized exchanges.
var ethLabelsExchanges = map[string]bool{
	"binance": true, "coinbase": true, "kraken": true, "okx": true, "okex": true, "bybit": true, "kucoin": true,
	"bitfinex": true, "huobi": true, "htx": true, "gate-io": true, "gate.io": true, "crypto-com": true,
	"gemini": true, "bitstamp": true, "upbit": true, "bithumb": true, "mexc": true, "bitget": true,
	"poloniex": true, "bittrex": true, "deribit": true, "ftx": true, "bitmex": true, "changenow": true,
}

func readEthLabels(path string) ([]AddressLabel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Address string `json:"address"`
		ChainID int    `json:"chainId"`
		Label   string `json:"label"`
		NameTag string `json:"nameTag"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse eth-labels %s: %w", path, err)
	}
	out := make([]AddressLabel, 0, len(rows))
	for _, row := range rows {
		chain, ok := ethLabelsChains[row.ChainID]
		if !ok || strings.TrimSpace(row.Address) == "" {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(row.Label))
		category := normalizeLabelCategory(slug, "")
		if ethLabelsExchanges[slug] {
			category = labelCategoryExchange
		}
		out = append(out, AddressLabel{
			Chain:      chain,
			Address:    row.Address,
			Label:      firstNonEmpty(strings.TrimSpace(row.NameTag), slug),
			Category:   category,
			ActorName:  slug,
			SourceRef:  "eth-labels " + slug,
			Confidence: tagPackConfidence["web_crawl"],
		})
	}
	return out, nil
}

func readScamSnifferAddresses(path string) ([]AddressLabel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var addresses []string
	if err := json.Unmarshal(raw, &addresses); err != nil {
		return nil, fmt.Errorf("parse scamsniffer %s: %w", path, err)
	}
	out := make([]AddressLabel, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address) == "" {
			continue
		}
		out = append(out, AddressLabel{
			Chain:      normalizeChain("", address),
			Address:    address,
			Label:      "ScamSniffer blacklist",
			Category:   labelCategoryScam,
			SourceRef:  "scamsniffer address.json",
			Confidence: tagPackConfidence["trusted_provider"],
		})
	}
	return out, nil
}

// applyAddressLabels attaches the best label and its category to address
// nodes, and the top labels for inspection. A node keeps its existing label
// unless that label is just the shortened address.
func (a *App) applyAddressLabels(ctx context.Context, nodes []FlowNode) {
	addresses := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if address := getString(node.Metrics, "address"); address != "" {
			addresses = append(addresses, address)
		}
	}
	labelsByAddress, err := lookupAddressLabels(ctx, a.db, addresses)
	if err != nil {
		logError(ctx, "address_labels_lookup_failed", err, nil)
		return
	}
	for i := range nodes {
		address := getString(nodes[i].Metrics, "address")
		labels := labelsByAddress[normalizeAddress(address)]
		if len(labels) == 0 {
			continue
		}
		best := labels[0]
		nodes[i].Metrics["label_category"] = best.Category
		nodes[i].Metrics["label_source"] = best.Source
		nodes[i].Metrics["label_confidence"] = best.Confidence
		top := make([]map[string]any, 0, min(len(labels), 5))
		for _, l := range labels[:min(len(labels), 5)] {
			top = append(top, map[string]any{"label": l.Label, "category": l.Category, "source": l.Source, "confidence": l.Confidence})
		}
		nodes[i].Metrics["labels"] = top
		if strings.TrimSpace(nodes[i].Label) == "" || nodes[i].Label == shortAddress(address) {
			nodes[i].Label = best.Label
		}
	}
}

// AddressLabels returns all labels for one address, best first.
func (a *App) AddressLabels(ctx context.Context, address string) ([]AddressLabel, error) {
	labels, err := lookupAddressLabels(ctx, a.db, []string{address})
	if err != nil {
		return nil, err
	}
	return labels[normalizeAddress(address)], nil
}

func (a *App) LabelSources(ctx context.Context) ([]LabelSource, error) {
	return listLabelSources(ctx, a.db)
}
