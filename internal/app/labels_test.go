package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTagPackInheritsHeaderAndMapsVocabulary(t *testing.T) {
	raw := []byte(`
title: Example pack
source: https://example.org/report
label: Header Label
abuse: ransomware
confidence: forensic
currency: BTC
tags:
- address: bc1qexample0000000000000000000000000000000
- address: '0xAbC0000000000000000000000000000000000001'
  currency: ETH
  label: Exchange Hot Wallet
  category: exchange
  confidence: service_api
`)
	labels, err := parseTagPack(raw, "example.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	first, second := labels[0], labels[1]
	if first.Label != "Header Label" || first.Category != labelCategoryRansomware || first.Confidence != 50 || first.Chain != "BTC" || first.SourceRef != "https://example.org/report" {
		t.Fatalf("unexpected inherited label %+v", first)
	}
	if second.Label != "Exchange Hot Wallet" || second.Category != labelCategoryExchange || second.Confidence != 70 || second.Chain != "ETH" {
		t.Fatalf("unexpected overridden label %+v", second)
	}
}

func TestImportOFACListsIsIdempotentAndResolves(t *testing.T) {
	app, err := New(Config{DBPath: filepath.Join(t.TempDir(), "labels.db"), RequestTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	dir := t.TempDir()
	const sanctioned = "0x098B716B8Aaf21512996dC57EB0615e2383E2f96"
	if err := os.WriteFile(filepath.Join(dir, "sanctioned_addresses_ETH.txt"), []byte(sanctioned+"\n0x1111111111111111111111111111111111111111\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sanctioned_addresses_XBT.txt"), []byte("bc1qsanctioned000000000000000000000000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		result, err := app.ImportLabels(ctx, "ofac", dir)
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		if result.Count != 3 {
			t.Fatalf("import %d: expected 3 labels, got %d", i, result.Count)
		}
	}
	labels, err := app.AddressLabels(ctx, sanctioned)
	if err != nil || len(labels) != 1 || labels[0].Category != labelCategorySanctioned || labels[0].Source != labelSourceOFAC {
		t.Fatalf("expected one sanctioned OFAC label, got %+v err=%v", labels, err)
	}
	btc, _ := app.AddressLabels(ctx, "bc1qsanctioned000000000000000000000000000")
	if len(btc) != 1 || btc[0].Chain != "BTC" {
		t.Fatalf("expected XBT list addresses on BTC, got %+v", btc)
	}
}

func TestLabelPrecedenceUserThenBuiltinThenConfidence(t *testing.T) {
	app, err := New(Config{DBPath: filepath.Join(t.TempDir(), "labels.db"), RequestTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	ctx := context.Background()
	const address = "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0" // built-in: Asgard Module
	if _, err := replaceLabelSource(ctx, app.db, labelSourceGraphSense, "test", "MIT", []AddressLabel{
		{Address: address, Label: "Low confidence guess", Category: labelCategoryOther, Confidence: 20},
		{Address: address, Label: "Better attribution", Category: labelCategoryProtocol, Confidence: 70},
	}); err != nil {
		t.Fatalf("store imported: %v", err)
	}
	labels, _ := app.AddressLabels(ctx, address)
	if len(labels) != 3 || labels[0].Source != labelSourceBuiltin || labels[1].Label != "Better attribution" {
		t.Fatalf("expected builtin first, then imported by confidence, got %+v", labels)
	}
	if err := upsertAddressAnnotation(ctx, app.db, address, "label", "My vault"); err != nil {
		t.Fatalf("annotate: %v", err)
	}
	labels, _ = app.AddressLabels(ctx, address)
	if labels[0].Source != labelSourceUser || labels[0].Label != "My vault" {
		t.Fatalf("expected the user label first, got %+v", labels)
	}
}

func TestApplyAddressLabelsKeepsMeaningfulNodeLabels(t *testing.T) {
	app, err := New(Config{DBPath: filepath.Join(t.TempDir(), "labels.db"), RequestTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()
	ctx := context.Background()
	const exchange = "0x28c6c06298d514db089934071355e5743bf21d60"
	if _, err := replaceLabelSource(ctx, app.db, labelSourceEthLabels, "test", "MIT", []AddressLabel{
		{Chain: "ETH", Address: exchange, Label: "Binance 14", Category: labelCategoryExchange, Confidence: 20},
	}); err != nil {
		t.Fatal(err)
	}
	nodes := []FlowNode{
		{ID: "a", Label: shortAddress(exchange), Metrics: map[string]any{"address": exchange}},
		{ID: "b", Label: "Treasury Hot Wallet", Metrics: map[string]any{"address": exchange}},
	}
	app.applyAddressLabels(ctx, nodes)
	if nodes[0].Label != "Binance 14" || nodes[0].Metrics["label_category"] != labelCategoryExchange {
		t.Fatalf("expected the imported label on an unlabeled node, got %q %v", nodes[0].Label, nodes[0].Metrics)
	}
	if nodes[1].Label != "Treasury Hot Wallet" || nodes[1].Metrics["label_category"] != labelCategoryExchange {
		t.Fatalf("expected an existing label kept with category attached, got %q %v", nodes[1].Label, nodes[1].Metrics)
	}
}

func TestNormalizeLabelCategory(t *testing.T) {
	cases := map[[2]string]string{
		{"exchange", ""}:          labelCategoryExchange,
		{"mining_service", ""}:    labelCategoryMiner,
		{"", "criminal"}:          labelCategoryOther,
		{"admin", ""}:             labelCategoryOther,
		{"", "ransomware"}:        labelCategoryRansomware,
		{"coinjoin", ""}:          labelCategoryMixer,
		{"", "sanctioned_entity"}: labelCategorySanctioned,
		{"phish-hack", ""}:        labelCategoryHack,
		{"defi_dex", ""}:          labelCategoryDeFi,
		{"", ""}:                  "",
	}
	for in, want := range cases {
		if got := normalizeLabelCategory(in[0], in[1]); got != want {
			t.Errorf("normalizeLabelCategory(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
