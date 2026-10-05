package app

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Address labels attribute addresses to real-world entities (exchanges,
// sanctioned parties, scams, protocols). Labels come from several sources with
// different reliability; the stored format follows GraphSense TagPacks so
// labels can be exchanged with other tools.

// Label categories. Imported vocabularies (GraphSense concepts and abuse
// types, Etherscan-style slugs) are mapped onto this set.
const (
	labelCategoryExchange      = "exchange"
	labelCategoryDeFi          = "defi"
	labelCategoryBridge        = "bridge"
	labelCategoryMixer         = "mixer"
	labelCategorySanctioned    = "sanctioned"
	labelCategoryScam          = "scam"
	labelCategoryHack          = "hack"
	labelCategoryRansomware    = "ransomware"
	labelCategoryDarknetMarket = "darknet_market"
	labelCategoryGambling      = "gambling"
	labelCategoryMiner         = "miner"
	labelCategoryWallet        = "wallet_service"
	labelCategoryExtremism     = "extremism"
	labelCategoryTerrorism     = "terrorism"
	labelCategoryProtocol      = "protocol"
	labelCategoryOther         = "other"
)

// Label sources. User labels come from address annotations and always win.
const (
	labelSourceUser        = "user"
	labelSourceBuiltin     = "builtin"
	labelSourceGraphSense  = "graphsense"
	labelSourceOFAC        = "ofac"
	labelSourceEthLabels   = "eth-labels"
	labelSourceScamSniffer = "scamsniffer"
)

// tagPackConfidence is the GraphSense TagPack confidence scale.
var tagPackConfidence = map[string]int{
	"override": 100, "ownership": 100, "ledger_immanent": 100, "manual_transaction": 90,
	"service_api": 70, "forensic_investigation": 70, "authority_data": 60, "trusted_provider": 50,
	"service_data": 50, "forensic": 50, "untrusted_transaction": 40, "web_crawl": 20,
	"heuristic": 10, "unknown": 5,
}

// AddressLabel is one attribution of an address.
type AddressLabel struct {
	Chain      string `json:"chain,omitempty"`
	Address    string `json:"address"`
	Label      string `json:"label"`
	Category   string `json:"category,omitempty"`
	ActorName  string `json:"actor,omitempty"`
	Source     string `json:"source"`
	SourceRef  string `json:"source_ref,omitempty"`
	Confidence int    `json:"confidence"`
}

// normalizeLabelCategory maps a vocabulary term (category or abuse type) onto
// the app's categories; unknown terms become "other".
func normalizeLabelCategory(category, abuse string) string {
	for _, term := range []string{category, abuse} {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		switch {
		case strings.Contains(term, "sanction"):
			return labelCategorySanctioned
		case strings.Contains(term, "exchange"), term == "cex", strings.Contains(term, "otc"):
			return labelCategoryExchange
		case strings.Contains(term, "bridge"):
			return labelCategoryBridge
		case strings.Contains(term, "mix"), strings.Contains(term, "tumbler"), strings.Contains(term, "coinjoin"), strings.Contains(term, "privacy"):
			return labelCategoryMixer
		case strings.Contains(term, "ransom"):
			return labelCategoryRansomware
		case strings.Contains(term, "hack"), strings.Contains(term, "exploit"), strings.Contains(term, "theft"), strings.Contains(term, "heist"):
			return labelCategoryHack
		case strings.Contains(term, "scam"), strings.Contains(term, "phish"), strings.Contains(term, "fraud"),
			strings.Contains(term, "ponzi"), strings.Contains(term, "sextortion"), strings.Contains(term, "drainer"):
			return labelCategoryScam
		case strings.Contains(term, "market"):
			return labelCategoryDarknetMarket
		case strings.Contains(term, "gambl"), strings.Contains(term, "casino"):
			return labelCategoryGambling
		case strings.Contains(term, "mining"), strings.Contains(term, "miner"):
			return labelCategoryMiner
		case strings.Contains(term, "wallet"), strings.Contains(term, "custod"), strings.Contains(term, "payment"):
			return labelCategoryWallet
		case strings.Contains(term, "terror"):
			return labelCategoryTerrorism
		case strings.Contains(term, "extrem"):
			return labelCategoryExtremism
		case strings.Contains(term, "defi"), strings.Contains(term, "dex"), strings.Contains(term, "swap"), strings.Contains(term, "lending"):
			return labelCategoryDeFi
		case term == labelCategoryProtocol:
			return labelCategoryProtocol
		}
	}
	if strings.TrimSpace(category) == "" && strings.TrimSpace(abuse) == "" {
		return ""
	}
	return labelCategoryOther
}

// labelChain maps a TagPack currency/network or chain id onto app chain codes.
func labelChain(currency, address string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "BTC", "XBT":
		return "BTC"
	case "TRX", "TRON":
		return "TRON"
	case "":
		return normalizeChain("", address)
	default:
		return strings.ToUpper(strings.TrimSpace(currency))
	}
}

// parseTagPack reads a GraphSense TagPack. Tags inherit header fields.
func parseTagPack(raw []byte, packName string) ([]AddressLabel, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse tagpack %s: %w", packName, err)
	}
	header := map[string]string{}
	for key, value := range doc {
		if key == "tags" {
			continue
		}
		header[key] = fmt.Sprint(value)
	}
	tags, _ := doc["tags"].([]any)
	out := make([]AddressLabel, 0, len(tags))
	for _, item := range tags {
		tag, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field := func(name string) string {
			if value, ok := tag[name]; ok && value != nil {
				return strings.TrimSpace(fmt.Sprint(value))
			}
			return strings.TrimSpace(header[name])
		}
		address := field("address")
		label := firstNonEmpty(field("label"), field("actor"), field("title"))
		if address == "" || label == "" {
			continue
		}
		confidence := tagPackConfidence[strings.ToLower(field("confidence"))]
		if n, err := strconv.Atoi(field("confidence")); err == nil {
			confidence = n
		}
		if confidence == 0 {
			confidence = tagPackConfidence["unknown"]
		}
		out = append(out, AddressLabel{
			Chain:      labelChain(firstNonEmpty(field("currency"), field("network")), address),
			Address:    address,
			Label:      label,
			Category:   normalizeLabelCategory(field("category"), field("abuse")),
			ActorName:  field("actor"),
			SourceRef:  firstNonEmpty(field("source"), packName),
			Confidence: confidence,
		})
	}
	return out, nil
}

//go:embed labels/builtin.yaml
var builtinLabelsYAML []byte

// builtinLabels are labels shipped with the app; they also feed the display
// labels used during graph projection.
var builtinLabels = func() []AddressLabel {
	labels, err := parseTagPack(builtinLabelsYAML, "builtin.yaml")
	if err != nil {
		panic(err)
	}
	return labels
}()

func builtinLabelMap() map[string]string {
	out := make(map[string]string, len(builtinLabels))
	for _, label := range builtinLabels {
		out[label.Address] = label.Label
	}
	return out
}

// replaceLabelSource replaces every label from source with labels, so
// re-importing the same data is idempotent and drops entries the source
// removed. It returns the number of stored labels.
func replaceLabelSource(ctx context.Context, db *sql.DB, source, version, license string, labels []AddressLabel) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM labels WHERE source = ?`, source); err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO labels(chain, address, normalized_address, label, category, actor_name, source, source_ref, confidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(normalized_address, source, label) DO UPDATE SET
			chain = excluded.chain, category = excluded.category, actor_name = excluded.actor_name,
			source_ref = excluded.source_ref, confidence = MAX(confidence, excluded.confidence)
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	stored := 0
	for _, label := range labels {
		normalized := normalizeAddress(label.Address)
		if normalized == "" || strings.TrimSpace(label.Label) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, strings.ToUpper(label.Chain), strings.TrimSpace(label.Address), normalized,
			strings.TrimSpace(label.Label), label.Category, label.ActorName, source, label.SourceRef, label.Confidence); err != nil {
			return 0, err
		}
		stored++
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM labels WHERE source = ?`, source).Scan(&count); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO label_sources(source, version, license, imported_at, count) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source) DO UPDATE SET version = excluded.version, license = excluded.license,
			imported_at = excluded.imported_at, count = excluded.count
	`, source, version, license, time.Now().UTC().Format(time.RFC3339), count); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

// LabelSource describes one imported label source.
type LabelSource struct {
	Source     string `json:"source"`
	Version    string `json:"version"`
	License    string `json:"license"`
	ImportedAt string `json:"imported_at"`
	Count      int    `json:"count"`
}

func listLabelSources(ctx context.Context, db *sql.DB) ([]LabelSource, error) {
	rows, err := db.QueryContext(ctx, `SELECT source, version, license, imported_at, count FROM label_sources ORDER BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LabelSource
	for rows.Next() {
		var s LabelSource
		if err := rows.Scan(&s.Source, &s.Version, &s.License, &s.ImportedAt, &s.Count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// labelSourceRank orders sources when confidence ties: built-in labels beat
// imported ones.
func labelSourceRank(source string) int {
	switch source {
	case labelSourceUser:
		return 0
	case labelSourceBuiltin:
		return 1
	default:
		return 2
	}
}

// lookupAddressLabels returns labels for each normalized address, best first:
// user annotations, then built-in labels, then imported labels by confidence.
func lookupAddressLabels(ctx context.Context, db *sql.DB, addresses []string) (map[string][]AddressLabel, error) {
	out := map[string][]AddressLabel{}
	unique := map[string]struct{}{}
	for _, address := range addresses {
		if normalized := normalizeAddress(address); normalized != "" {
			unique[normalized] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	const batch = 400
	for start := 0; start < len(keys); start += batch {
		chunk := keys[start:min(start+batch, len(keys))]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, 0, len(chunk))
		for _, key := range chunk {
			args = append(args, key)
		}
		rows, err := db.QueryContext(ctx, `
			SELECT chain, address, normalized_address, label, category, actor_name, source, source_ref, confidence
			FROM labels WHERE normalized_address IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var l AddressLabel
			var normalized string
			if err := rows.Scan(&l.Chain, &l.Address, &normalized, &l.Label, &l.Category, &l.ActorName, &l.Source, &l.SourceRef, &l.Confidence); err != nil {
				rows.Close()
				return nil, err
			}
			out[normalized] = append(out[normalized], l)
		}
		rows.Close()
		annotationRows, err := db.QueryContext(ctx, `
			SELECT address, normalized_address, value FROM address_annotations
			WHERE kind = 'label' AND TRIM(value) != '' AND normalized_address IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for annotationRows.Next() {
			var address, normalized, value string
			if err := annotationRows.Scan(&address, &normalized, &value); err != nil {
				annotationRows.Close()
				return nil, err
			}
			out[normalized] = append(out[normalized], AddressLabel{Address: address, Label: value, Source: labelSourceUser, Confidence: 100})
		}
		annotationRows.Close()
	}
	for key := range out {
		labels := out[key]
		sort.SliceStable(labels, func(i, j int) bool {
			if ri, rj := labelSourceRank(labels[i].Source), labelSourceRank(labels[j].Source); ri != rj {
				return ri < rj
			}
			return labels[i].Confidence > labels[j].Confidence
		})
	}
	return out, nil
}

// seedBuiltinLabels stores the built-in labels so lookups, exports, and
// trace stop rules see them.
func seedBuiltinLabels(ctx context.Context, db *sql.DB) error {
	labels := make([]AddressLabel, len(builtinLabels))
	copy(labels, builtinLabels)
	for i := range labels {
		labels[i].Source = labelSourceBuiltin
	}
	_, err := replaceLabelSource(ctx, db, labelSourceBuiltin, "embedded", "app", labels)
	return err
}
