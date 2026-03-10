package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpsertActorRejectsDuplicateAddresses(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := upsertActor(context.Background(), db, 0, ActorUpsertRequest{
		Name:  "Alpha",
		Color: "#123456",
		Addresses: []ActorAddressInput{
			{Address: "thor1abc"},
			{Address: "THOR1ABC"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate address validation error")
	}
}

func TestUpsertActorPersistsAddresses(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	actor, err := upsertActor(context.Background(), db, 0, ActorUpsertRequest{
		Name:  "Alpha",
		Color: "#123456",
		Notes: "desk",
		Addresses: []ActorAddressInput{
			{Address: "thor1abc", Label: "primary"},
			{Address: "0xabc", ChainHint: "ETH"},
		},
	})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}
	if actor.ID == 0 {
		t.Fatal("expected actor id")
	}
	if len(actor.Addresses) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(actor.Addresses))
	}

	actors, err := listActors(context.Background(), db)
	if err != nil {
		t.Fatalf("list actors: %v", err)
	}
	if len(actors) != 1 {
		t.Fatalf("expected 1 actor, got %d", len(actors))
	}
	if actors[0].Name != "Alpha" {
		t.Fatalf("unexpected actor name: %s", actors[0].Name)
	}
}

func TestMakePoolRefKeepsDistinctPoolsSeparate(t *testing.T) {
	builder := &graphBuilder{}
	solPool := builder.makePoolRef("sol.sol", 2)
	btcPool := builder.makePoolRef("btc.btc", 2)

	if solPool.Key == "" || btcPool.Key == "" {
		t.Fatal("expected non-empty pool keys")
	}
	if solPool.Key == btcPool.Key {
		t.Fatalf("expected distinct pool keys, got %q", solPool.Key)
	}
	if solPool.Label != "Pool SOL.SOL" {
		t.Fatalf("unexpected SOL pool label: %q", solPool.Label)
	}
	if btcPool.Label != "Pool BTC.BTC" {
		t.Fatalf("unexpected BTC pool label: %q", btcPool.Label)
	}
}

func TestMakeAddressRefUsesKnownRujiraLabels(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
	}

	rujiraSwap := builder.makeAddressRef("thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g", "THOR", 1)
	if rujiraSwap.Label != "Rujira THORChain Swap" {
		t.Fatalf("unexpected rujira swap label: %q", rujiraSwap.Label)
	}

	treasuryETH := builder.makeAddressRef("0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3", "ETH", 1)
	if treasuryETH.Label != "Treasury Eth Wallet" {
		t.Fatalf("unexpected treasury ETH label: %q", treasuryETH.Label)
	}

	asgard := builder.makeAddressRef("thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0", "THOR", 1)
	if asgard.ID != "" {
		t.Fatalf("expected Asgard module to be fully excluded, got node %#v", asgard)
	}
}

func TestNormalizeFrontierAddress(t *testing.T) {
	cases := []struct {
		in          string
		wantAddress string
		wantChain   string
	}{
		{in: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k", wantAddress: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k", wantChain: "THOR"},
		{in: "thor1abcxyz/thor1abcxyz", wantAddress: "thor1abcxyz", wantChain: "THOR"},
		{in: "ETH|0xAbCdEfabcdefabcdefabcdefabcdefabcdefabcd", wantAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", wantChain: "ETH"},
		{in: "0xAbCdEfabcdefabcdefabcdefabcdefabcdefabcd", wantAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd", wantChain: "ETH"},
		{in: "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE", wantAddress: "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE", wantChain: "TRON"},
		{in: "https://midgard.ninerealms.com", wantAddress: "", wantChain: ""},
		{in: "thor1abc?foo=bar", wantAddress: "", wantChain: ""},
	}

	for _, tc := range cases {
		got := normalizeFrontierAddress(tc.in)
		if got.Address != tc.wantAddress || got.Chain != tc.wantChain {
			t.Fatalf("normalizeFrontierAddress(%q) = %#v, want address=%q chain=%q", tc.in, got, tc.wantAddress, tc.wantChain)
		}
	}
}

func TestNormalizeChainPrefersAddressFormatForNonEVM(t *testing.T) {
	cases := []struct {
		name    string
		hint    string
		address string
		want    string
	}{
		{
			name:    "thor address overrides btc hint",
			hint:    "BTC",
			address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
			want:    "THOR",
		},
		{
			name:    "btc address overrides thor hint",
			hint:    "THOR",
			address: "bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs",
			want:    "BTC",
		},
		{
			name:    "evm address keeps explicit non-eth hint",
			hint:    "BASE",
			address: "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
			want:    "BASE",
		},
		{
			name:    "evm address defaults to eth with empty hint",
			hint:    "",
			address: "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
			want:    "ETH",
		},
		{
			name:    "invalid evm-like value does not infer eth",
			hint:    "THOR",
			address: "0xnotanaddress",
			want:    "THOR",
		},
		{
			name:    "unknown format keeps hint",
			hint:    "DOGE",
			address: "DQexampleUnrecognizedFormat",
			want:    "DOGE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeChain(tc.hint, tc.address); got != tc.want {
				t.Fatalf("normalizeChain(%q, %q) = %q, want %q", tc.hint, tc.address, got, tc.want)
			}
		})
	}
}

func TestNormalizeAddressPreservesCaseSensitiveChains(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "THOR1ABC", want: "thor1abc"},
		{in: "0xAbCdEfabcdefabcdefabcdefabcdefabcdefabcd", want: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		{in: "0xnotanaddress", want: "0xnotanaddress"},
		{in: "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE", want: "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE"},
		{in: "r4ntzus2299LYJnoMdRF1sAjzBc3oDa4qT", want: "r4ntzus2299LYJnoMdRF1sAjzBc3oDa4qT"},
		{in: "GLBxTgDNuV2QHLRq4z5npqhRoEbQGGcoB5RRDUDqcfWB", want: "GLBxTgDNuV2QHLRq4z5npqhRoEbQGGcoB5RRDUDqcfWB"},
	}

	for _, tc := range cases {
		if got := normalizeAddress(tc.in); got != tc.want {
			t.Fatalf("normalizeAddress(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeFrontierAddressRejectsInvalidEVMCandidate(t *testing.T) {
	if got := normalizeFrontierAddress("0xnotanaddress"); got.Address != "" || got.Chain != "" {
		t.Fatalf("expected invalid evm candidate to be rejected, got %#v", got)
	}
}

func TestNormalizeGraphAmount(t *testing.T) {
	cases := []struct {
		raw      string
		decimals int
		want     string
	}{
		{raw: "100000000", decimals: 8, want: "100000000"},
		{raw: "1000000000000000000", decimals: 18, want: "100000000"},
		{raw: "1000000", decimals: 6, want: "100000000"},
		{raw: "123456789", decimals: 9, want: "12345678"},
	}

	for _, tc := range cases {
		if got := normalizeGraphAmount(tc.raw, tc.decimals); got != tc.want {
			t.Fatalf("normalizeGraphAmount(%q, %d) = %q, want %q", tc.raw, tc.decimals, got, tc.want)
		}
	}
}

func TestMidgardGraphPagesForHop(t *testing.T) {
	if got := midgardGraphPagesForHop(0); got != midgardGraphPagesPerSeed {
		t.Fatalf("hop 0 pages = %d, want %d", got, midgardGraphPagesPerSeed)
	}
	if got := midgardGraphPagesForHop(1); got != midgardGraphPagesPerFirstHop {
		t.Fatalf("hop 1 pages = %d, want %d", got, midgardGraphPagesPerFirstHop)
	}
	if got := midgardGraphPagesForHop(2); got != midgardGraphPagesPerHop {
		t.Fatalf("hop 2 pages = %d, want %d", got, midgardGraphPagesPerHop)
	}
}

func TestProjectMidgardActionInfersContractSwapAmount(t *testing.T) {
	builder := &graphBuilder{
		ownerMap: map[string][]int64{
			normalizeAddress("thor1seed"): []int64{1},
		},
		actorsByID: map[int64]Actor{
			1: {ID: 1, Name: "Alpha", Color: "#4ca3ff"},
		},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC":   100000,
				"THOR.RUNE": 2,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor1seed",
				TxID:    "TX1",
				Coins:   []midgardActionCoin{},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1contract",
				TxID:    "TX1",
				Coins:   []midgardActionCoin{},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				Funds: "1475000000btc-btc",
				Msg: map[string]any{
					"update": map[string]any{
						"nodes": []any{
							map[string]any{
								"action": map[string]any{
									"action": map[string]any{
										"swap": map[string]any{
											"swap_amount": map[string]any{
												"amount": "158730",
												"denom":  "btc-btc",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Asset != "BTC.BTC" {
		t.Fatalf("expected BTC.BTC asset, got %s", segments[0].Asset)
	}
	if segments[0].AmountRaw != "158730" {
		t.Fatalf("expected inferred swap amount 158730, got %s", segments[0].AmountRaw)
	}
	if segments[0].ActionClass != "swaps" {
		t.Fatalf("expected swaps action class, got %s", segments[0].ActionClass)
	}
	if segments[0].USDSpot <= 0 {
		t.Fatalf("expected positive usd spot, got %f", segments[0].USDSpot)
	}
}

func TestProjectMidgardActionFallsBackToContractFunds(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.RUNE": 2,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor1source",
				TxID:    "TX2",
				Coins:   []midgardActionCoin{},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1target",
				TxID:    "TX2",
				Coins:   []midgardActionCoin{},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				Funds: "250000000rune",
				Msg:   map[string]any{},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Asset != "THOR.RUNE" {
		t.Fatalf("expected THOR.RUNE asset, got %s", segments[0].Asset)
	}
	if segments[0].AmountRaw != "250000000" {
		t.Fatalf("expected funds fallback amount 250000000, got %s", segments[0].AmountRaw)
	}
	if segments[0].ActionClass != "liquidity" {
		t.Fatalf("expected liquidity action class, got %s", segments[0].ActionClass)
	}
	if segments[0].USDSpot <= 0 {
		t.Fatalf("expected positive usd spot, got %f", segments[0].USDSpot)
	}
}

func TestProjectMidgardContractOrderCreateRoutesFundsToContract(t *testing.T) {
	builder := &graphBuilder{
		ownerMap: map[string][]int64{
			normalizeAddress("thor1jq0huwma0gvujn7a4mlygw8kutdmja5kza4e5p"): {1},
		},
		actorsByID: map[int64]Actor{
			1: {ID: 1, Name: "Maker", Color: "#4ca3ff"},
		},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{"BTC.BTC": 100000},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1758120688596260638",
		Height: "22879614",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor1jq0huwma0gvujn7a4mlygw8kutdmja5kza4e5p",
				TxID:    "8D9F6FC34A6A6D10AE60B6B7A5BD885A0DFDBFD97C76AFE83AC627293F27C12C",
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1dwsnlqw3lfhamc5dz3r57hlsppx3a2n2d7kppccxfdhfazjh06rs5077sz",
				TxID:    "8D9F6FC34A6A6D10AE60B6B7A5BD885A0DFDBFD97C76AFE83AC627293F27C12C",
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-fin/order.create",
				Funds:        "45000btc-btc",
				Msg: map[string]any{
					"order": []any{[]any{[]any{"base", map[string]any{"oracle": -51}, "45000"}}, nil},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("thor1jq0huwma0gvujn7a4mlygw8kutdmja5kza4e5p") {
		t.Fatalf("unexpected source address: %s", segments[0].Source.Address)
	}
	if segments[0].Target.Kind != "contract_address" {
		t.Fatalf("expected contract target, got %s", segments[0].Target.Kind)
	}
	if !strings.HasPrefix(segments[0].Target.Label, "Rujira FIN") {
		t.Fatalf("unexpected contract label: %q", segments[0].Target.Label)
	}
	if segments[0].Asset != "BTC.BTC" || segments[0].AmountRaw != "45000" {
		t.Fatalf("unexpected funded segment: %#v", segments[0])
	}
	if segments[0].ActionKey != "rujira.fin.order.create" {
		t.Fatalf("unexpected action key: %s", segments[0].ActionKey)
	}
}

func TestProjectMidgardContractTradeRoutesThroughContracts(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC":   100000,
				"THOR.TCY":  1,
				"THOR.RUNE": 2,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1771594964502249774",
		Height: "25032287",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1jshw3secvxhzfyza6aj530hrc73zave42zgs525n0xkc3e9d6wkqrm8j3y",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins: []midgardActionCoin{
					{Amount: "141710520481", Asset: "THOR.TCY"},
				},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-fin/trade",
				Funds:        "3000000000btc-btc",
				Msg: map[string]any{
					"update": map[string]any{
						"contract_address": "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8",
						"nodes": []any{
							map[string]any{
								"condition": map[string]any{
									"condition": map[string]any{
										"can_swap": map[string]any{
											"swap_amount": map[string]any{
												"amount": "158730",
												"denom":  "btc-btc",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 routed segments, got %d", len(segments))
	}

	if segments[0].Source.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("expected contract-address source, got %s", segments[0].Source.Address)
	}
	if segments[0].Target.Kind != "contract_address" {
		t.Fatalf("expected contract receiver, got %s", segments[0].Target.Kind)
	}
	if segments[0].Asset != "BTC.BTC" || segments[0].AmountRaw != "158730" {
		t.Fatalf("unexpected input leg: %#v", segments[0])
	}
	if segments[1].Source.Address != normalizeAddress("thor1jshw3secvxhzfyza6aj530hrc73zave42zgs525n0xkc3e9d6wkqrm8j3y") {
		t.Fatalf("unexpected payout source: %s", segments[1].Source.Address)
	}
	if segments[1].Target.Address != normalizeAddress("thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g") {
		t.Fatalf("unexpected payout target: %s", segments[1].Target.Address)
	}
	if segments[1].Asset != "THOR.TCY" || segments[1].AmountRaw != "141710520481" {
		t.Fatalf("unexpected payout leg: %#v", segments[1])
	}
	if segments[1].ActionLabel != "Rujira FIN trade" {
		t.Fatalf("unexpected action label: %s", segments[1].ActionLabel)
	}
}

func TestProjectMidgardCalcManagerUpdateUsesInboundSenderForFunds(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC":  100000,
				"THOR.TCY": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1771594964502249774",
		Height: "25032287",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor136rwqvwy3flttm9wfnc5xgnlr6mu5k8e2elgzs2hdhuwf50w3l2q0nu2qu",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins: []midgardActionCoin{
					{Amount: "141710520481", Asset: "THOR.TCY"},
				},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-manager/strategy.update",
				Funds:        "3000000000btc-btc",
				Msg: map[string]any{
					"update": map[string]any{
						"contract_address": "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8",
						"nodes": []any{
							map[string]any{
								"condition": map[string]any{
									"condition": map[string]any{
										"can_swap": map[string]any{
											"swap_amount": map[string]any{
												"amount": "158730",
												"denom":  "btc-btc",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected inbound sender source, got %s", segments[0].Source.Address)
	}
	if segments[0].Target.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("unexpected receiver target: %s", segments[0].Target.Address)
	}
	if segments[0].Asset != "BTC.BTC" || segments[0].AmountRaw != "3000000000" {
		t.Fatalf("unexpected funds segment: %#v", segments[0])
	}
	if segments[0].ActionKey != "calc.manager.strategy.update" {
		t.Fatalf("unexpected action key: %s", segments[0].ActionKey)
	}
	if segments[1].Source.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("expected contract payout source, got %s", segments[1].Source.Address)
	}
	if segments[1].Target.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected Treasury payout target, got %s", segments[1].Target.Address)
	}
	if segments[1].Asset != "THOR.TCY" || segments[1].AmountRaw != "141710520481" {
		t.Fatalf("unexpected payout segment: %#v", segments[1])
	}
}

func TestMidgardActionKeyDistinguishesContractTypes(t *testing.T) {
	strategyUpdate := midgardAction{
		Type:   "contract",
		Height: "25032287",
		Date:   "1771594964502249774",
		In:     []midgardActionLeg{{TxID: "E04EFC19"}},
		Out:    []midgardActionLeg{{TxID: "E04EFC19"}},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{ContractType: "wasm-calc-strategy/update"},
		},
	}
	managerUpdate := midgardAction{
		Type:   "contract",
		Height: "25032287",
		Date:   "1771594964502249774",
		In:     []midgardActionLeg{{TxID: "E04EFC19"}},
		Out:    []midgardActionLeg{{TxID: "E04EFC19"}},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{ContractType: "wasm-calc-manager/strategy.update"},
		},
	}
	k1 := midgardActionKey(strategyUpdate)
	k2 := midgardActionKey(managerUpdate)
	if k1 == k2 {
		t.Fatalf("contract actions with different contract types must produce distinct keys, got %q for both", k1)
	}
}

func TestCalcStrategyProcessServesAsRepresentative(t *testing.T) {
	// wasm-calc-strategy/process should produce the collapsed treasury view:
	// BTC into the contract, then TCY back to the treasury destination.
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC":  100000,
				"THOR.TCY": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	calcMsg := map[string]any{
		"update": map[string]any{
			"contract_address": "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8",
			"nodes": []any{
				map[string]any{
					"condition": map[string]any{
						"condition": map[string]any{
							"can_swap": map[string]any{
								"swap_amount": map[string]any{
									"amount": "158730",
									"denom":  "btc-btc",
								},
							},
						},
					},
				},
				map[string]any{
					"action": map[string]any{
						"action": map[string]any{
							"distribute": map[string]any{
								"destinations": []any{
									map[string]any{
										"recipient": map[string]any{
											"bank": map[string]any{
												"address": "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
											},
										},
										"shares": "10000",
									},
									map[string]any{
										"label": "CALC",
										"recipient": map[string]any{
											"bank": map[string]any{
												"address": "thor17dxtxrne37gguxdeun4n36vqd5jmxxku5tr6gkuhhsh4lz9e8gksck4ygu",
											},
										},
										"shares": "25",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processAction := midgardAction{
		Date:   "1771594964502249774",
		Height: "25032287",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8",
				TxID:    "E04EFC19066E741B8F94C7F35672392378273AB9D9C14F0E197897712EAECC04",
			},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "141710520481", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Funds:        "3000000000btc-btc",
				Msg:          calcMsg,
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(processAction, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected inbound sender source, got %s", segments[0].Source.Address)
	}
	if segments[0].Target.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("unexpected receiver target: %s", segments[0].Target.Address)
	}
	if segments[0].Asset != "BTC.BTC" || segments[0].AmountRaw != "3000000000" {
		t.Fatalf("unexpected funds segment: asset=%s amount=%s", segments[0].Asset, segments[0].AmountRaw)
	}
	if segments[0].ActionKey != "calc.strategy.process" {
		t.Fatalf("unexpected action key: %s", segments[0].ActionKey)
	}
	if segments[1].Source.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("expected contract payout source, got %s", segments[1].Source.Address)
	}
	if segments[1].Target.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected treasury payout target, got %s", segments[1].Target.Address)
	}
	if segments[1].Asset != "THOR.TCY" || segments[1].AmountRaw != "141710520481" {
		t.Fatalf("unexpected payout segment: asset=%s amount=%s", segments[1].Asset, segments[1].AmountRaw)
	}
}

func TestCalcStrategyProcessInstantiateRoutesPayoutToTreasury(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC":  100000,
				"THOR.TCY": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	processAction := midgardAction{
		Date:   "1758924108669767000",
		Height: "24133792",
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "C38C6D8AF827076B87DB79629389E1A453FD4B094C0D2B016C11D31581BA320C",
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1eglk73j46xsmxnhc3tx5lm8f97k36gmvepf5j4qhyp7csvgcne8qecy4kv",
				TxID:    "C38C6D8AF827076B87DB79629389E1A453FD4B094C0D2B016C11D31581BA320C",
			},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "88905765172", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Funds:        "500000000btc-btc",
				Msg: map[string]any{
					"instantiate": map[string]any{
						"label": "DCA into TCY",
						"nodes": []any{
							map[string]any{
								"action": map[string]any{
									"action": map[string]any{
										"distribute": map[string]any{
											"destinations": []any{
												map[string]any{
													"recipient": map[string]any{
														"bank": map[string]any{
															"address": "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
														},
													},
													"shares": "10000",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(processAction, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Target.Address != normalizeAddress("thor1eglk73j46xsmxnhc3tx5lm8f97k36gmvepf5j4qhyp7csvgcne8qecy4kv") {
		t.Fatalf("expected instantiated contract target, got %s", segments[0].Target.Address)
	}
	if segments[1].Source.Address != normalizeAddress("thor1eglk73j46xsmxnhc3tx5lm8f97k36gmvepf5j4qhyp7csvgcne8qecy4kv") {
		t.Fatalf("expected instantiated contract payout source, got %s", segments[1].Source.Address)
	}
	if segments[1].Target.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected treasury payout target, got %s", segments[1].Target.Address)
	}
	if segments[1].Asset != "THOR.TCY" || segments[1].AmountRaw != "88905765172" {
		t.Fatalf("unexpected instantiate payout segment: asset=%s amount=%s", segments[1].Asset, segments[1].AmountRaw)
	}
}

func TestCalcStrategyExecuteReusesKnownTreasuryPayout(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.TCY": 1,
			},
		},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
	}

	setupAction := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k", TxID: "C87FF679"},
		},
		Out: []midgardActionLeg{
			{Address: "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8", TxID: "C87FF679"},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "110291159867", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Funds:        "500000000btc-btc",
				Msg: map[string]any{
					"instantiate": map[string]any{
						"nodes": []any{
							map[string]any{
								"action": map[string]any{
									"action": map[string]any{
										"distribute": map[string]any{
											"destinations": []any{
												map[string]any{
													"recipient": map[string]any{
														"bank": map[string]any{
															"address": "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	builder.recordCalcRepresentativePayouts([]midgardAction{setupAction})

	executeAction := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{Address: "thor159p2hth4y3qf92whcr4yc5carllgvrwthejcrm", TxID: "28F10573"},
		},
		Out: []midgardActionLeg{
			{Address: "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8", TxID: "28F10573"},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "139890253597", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Msg: map[string]any{
					"execute": []any{"8354017092008261581", "18316960598186678417"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(executeAction, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("expected strategy contract source, got %s", segments[0].Source.Address)
	}
	if segments[0].Target.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected treasury payout target, got %s", segments[0].Target.Address)
	}
	if segments[0].Asset != "THOR.TCY" || segments[0].AmountRaw != "139890253597" {
		t.Fatalf("unexpected execute payout segment: asset=%s amount=%s", segments[0].Asset, segments[0].AmountRaw)
	}
}

func TestCalcStrategyProcessKnownContractFallbackRoutesPayoutToTreasury(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.TCY": 1,
			},
		},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
	}

	action := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{Address: "thor1user7d88zhlqhagf8vhar480ef0xps6tc7fake", TxID: "CALCFALLBACK1"},
		},
		Out: []midgardActionLeg{
			{Address: "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8", TxID: "CALCFALLBACK1"},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "109380344490", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Msg: map[string]any{
					"execute": []any{"16978320920792957714"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8") {
		t.Fatalf("expected contract payout source, got %s", segments[0].Source.Address)
	}
	if segments[0].Target.Address != normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k") {
		t.Fatalf("expected Treasury fallback target, got %s", segments[0].Target.Address)
	}
	if segments[0].Asset != "THOR.TCY" || segments[0].AmountRaw != "109380344490" {
		t.Fatalf("unexpected fallback payout segment: asset=%s amount=%s", segments[0].Asset, segments[0].AmountRaw)
	}
}

func TestCalcStrategyExecuteWithoutKnownContractSuppressesFallback(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.TCY": 1,
			},
		},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
	}

	executeAction := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{Address: "thor159p2hth4y3qf92whcr4yc5carllgvrwthejcrm", TxID: "28F10573"},
		},
		Out: []midgardActionLeg{
			{Address: "thor1t2w6wyg2kjzfefgkej6mfgk29jsh9u7eszljl0fqx2usf5cn7h5q2pzwh8", TxID: "28F10573"},
			{
				Address: "thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g",
				Coins:   []midgardActionCoin{{Amount: "139890253597", Asset: "THOR.TCY"}},
			},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
				Msg: map[string]any{
					"execute": []any{"8354017092008261581", "18316960598186678417"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(executeAction, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 0 {
		t.Fatalf("expected scheduler execute leg to be suppressed, got %#v", segments)
	}
}

func TestShouldSkipMidgardActionForGraphAllowsExecuteWithoutProcess(t *testing.T) {
	execute := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "CALCEXEC1"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/execute",
				Msg:          map[string]any{"execute": []any{"7458177349767362226"}},
			},
		},
	}

	if skip, reason := shouldSkipMidgardActionForGraph(execute, nil, nil, map[string]struct{}{"CALCEXEC1": {}}, nil); skip {
		t.Fatalf("did not expect execute fallback to be skipped without process, got reason=%q", reason)
	}
	if skip, reason := shouldSkipMidgardActionForGraph(execute, nil, nil, map[string]struct{}{"CALCEXEC1": {}}, map[string]struct{}{"CALCEXEC1": {}}); !skip || reason != "contract_sub_execution" {
		t.Fatalf("expected execute fallback to be skipped when process exists, got skip=%v reason=%q", skip, reason)
	}
}

func TestShouldSkipMidgardActionForGraphAllowsProcessReplyAsLastFallback(t *testing.T) {
	reply := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "CALCREPLY1"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process.reply",
				Msg:          map[string]any{"execute": []any{"7458177349767362226"}},
			},
		},
	}

	if skip, reason := shouldSkipMidgardActionForGraph(reply, nil, nil, nil, nil); skip {
		t.Fatalf("did not expect lone process.reply fallback to be skipped, got reason=%q", reason)
	}
	if skip, reason := shouldSkipMidgardActionForGraph(reply, nil, nil, map[string]struct{}{"CALCREPLY1": {}}, nil); !skip || reason != "contract_sub_execution" {
		t.Fatalf("expected process.reply to be skipped when execute/representative exists, got skip=%v reason=%q", skip, reason)
	}
}

func TestCalcStrategySubExecutionsSuppressed(t *testing.T) {
	// Contract actions that carry the CALC strategy msg payload (update.nodes)
	// but are not the representative type should be suppressed.
	calcMsg := map[string]any{
		"update": map[string]any{
			"contract_address": "thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8",
			"nodes": []any{
				map[string]any{"condition": map[string]any{}},
			},
		},
	}

	subTypes := []string{
		"wasm-calc-strategy/process.reply",
		"wasm-rujira-ghost-vault/borrow",
		"wasm-rujira-ghost-vault/repay",
		"wasm-rujira-thorchain-swap/swap",
		"wasm-rujira-fin/market-maker.fee",
	}
	for _, ct := range subTypes {
		action := midgardAction{
			Type: "contract",
			Metadata: midgardActionMetadata{
				Contract: &midgardContractMetadata{
					ContractType: ct,
					Funds:        "3000000000btc-btc",
					Msg:          calcMsg,
				},
			},
		}
		if !isSuppressedContractSubExecution(action) {
			t.Errorf("%s with CALC strategy msg should be suppressed", ct)
		}
	}

	// The representative types should NOT be suppressed.
	for _, ct := range []string{"wasm-calc-strategy/process", "wasm-calc-manager/strategy.update"} {
		action := midgardAction{
			Type: "contract",
			Metadata: midgardActionMetadata{
				Contract: &midgardContractMetadata{
					ContractType: ct,
					Funds:        "3000000000btc-btc",
					Msg:          calcMsg,
				},
			},
		}
		if isSuppressedContractSubExecution(action) {
			t.Errorf("%s should NOT be suppressed (it is the representative)", ct)
		}
	}

	// ghost-vault WITHOUT CALC strategy msg should NOT be suppressed.
	normalAction := midgardAction{
		Type: "contract",
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-ghost-vault/borrow",
				Funds:        "1000000rune",
				Msg:          map[string]any{"borrow": map[string]any{}},
			},
		},
	}
	if isSuppressedContractSubExecution(normalAction) {
		t.Error("ghost-vault/borrow without CALC strategy msg should NOT be suppressed")
	}

	instantiateAction := midgardAction{
		Type: "contract",
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-ghost-vault/borrow",
				Msg: map[string]any{
					"instantiate": map[string]any{
						"nodes": []any{
							map[string]any{"action": map[string]any{}},
						},
					},
				},
			},
		},
	}
	if !isSuppressedContractSubExecution(instantiateAction) {
		t.Error("ghost-vault/borrow with CALC instantiate msg should be suppressed")
	}
}

func TestAddProjectedSegmentKeepsDistinctContractCallsSeparate(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:         map[string][]int64{},
		actorsByID:       map[int64]Actor{},
		protocols:        protocolDirectory{},
		allowedFlowTypes: flowTypeSet([]string{"liquidity"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	source := builder.makeAddressRef("thor1source0000000000000000000000000000000", "THOR", 1)
	target := builder.makeContractRef("thor1target0000000000000000000000000000000", contractCallDescriptor{Contract: "Rujira FIN"}, 2)

	builder.addProjectedSegment(projectedSegment{
		Source:      source,
		Target:      target,
		ActionClass: "liquidity",
		ActionKey:   "rujira.fin.order.create",
		ActionLabel: "Rujira FIN order.create",
		Asset:       "BTC.BTC",
		AmountRaw:   "45000",
	})
	builder.addProjectedSegment(projectedSegment{
		Source:      source,
		Target:      target,
		ActionClass: "liquidity",
		ActionKey:   "rujira.fin.market_maker.fee",
		ActionLabel: "Rujira FIN market-maker.fee",
		Asset:       "BTC.BTC",
		AmountRaw:   "1200",
	})

	if len(builder.edges) != 2 {
		t.Fatalf("expected distinct edges per action key, got %d", len(builder.edges))
	}
}

func TestMakeContractRefInfersEVMChain(t *testing.T) {
	builder := &graphBuilder{}
	ref := builder.makeContractRef("0x63713Ec54af592A7BA9d762D5Fdf1d383b4eff5A", contractCallDescriptor{Contract: "Contract"}, 2)
	if ref.Kind != "contract_address" {
		t.Fatalf("unexpected kind: %#v", ref)
	}
	if ref.Chain != "ETH" {
		t.Fatalf("expected ETH chain, got %#v", ref)
	}
	if ref.Key != "ETH|0x63713ec54af592a7ba9d762d5fdf1d383b4eff5a" {
		t.Fatalf("expected chain-qualified key, got %#v", ref)
	}
}

func TestProjectMidgardAddLiquidityCreatesPoolSegments(t *testing.T) {
	builder := &graphBuilder{
		ownerMap: map[string][]int64{
			normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k"): {1},
		},
		actorsByID: map[int64]Actor{
			1: {ID: 1, Name: "Treasury", Color: "#4ca3ff"},
		},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.RUNE": 2,
				"TRON.USDT-TR7NHQJEKQXGTCI8Q8ZY4PL8OTSZGJLJ6T": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1759280596212434790",
		Height: "24000000",
		Type:   "addLiquidity",
		Status: "success",
		Pools:  []string{"TRON.USDT-TR7NHQJEKQXGTCI8Q8ZY4PL8OTSZGJLJ6T"},
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "96975D634CF6C9616578108DB8C0D82ACFB31512DD2C3FD5B7E620233CEB335C",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "13780000000000"},
				},
			},
			{
				Address: "TJ8LE5jBifN2CQmgE5TLiLZGFGVNny1dHT",
				TxID:    "AF2C7B90DF864A893BC4D466C4EEC4545098AD526B3DDACEF8762CE91E71F519",
				Coins: []midgardActionCoin{
					{Asset: "TRON.USDT-TR7NHQJEKQXGTCI8Q8ZY4PL8OTSZGJLJ6T", Amount: "15157000000000"},
				},
			},
		},
		Out: []midgardActionLeg{},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 pool-directed liquidity segments, got %d", len(segments))
	}

	sources := map[string]struct{}{}
	for _, seg := range segments {
		if seg.ActionClass != "liquidity" {
			t.Fatalf("expected liquidity action class, got %s", seg.ActionClass)
		}
		if seg.Target.Kind != "pool" {
			t.Fatalf("expected pool target, got %s", seg.Target.Kind)
		}
		if seg.Target.Label != "Pool TRON.USDT-TR7NHQJEKQXGTCI8Q8ZY4PL8OTSZGJLJ6T" {
			t.Fatalf("unexpected pool label: %q", seg.Target.Label)
		}
		sources[seg.Source.Address] = struct{}{}
	}
	if _, ok := sources[normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k")]; !ok {
		t.Fatal("expected THOR provider address in projected sources")
	}
	if _, ok := sources[normalizeAddress("TJ8LE5jBifN2CQmgE5TLiLZGFGVNny1dHT")]; !ok {
		t.Fatal("expected external provider address in projected sources")
	}
}

func TestProjectMidgardBondMapsWalletToValidatorNode(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{"THOR.RUNE": 2},
		},
		bondMemoNodeByTx: map[string]string{
			"B833FC6746BB8B56C615ADB847370BE1F882A6179E14059AE982F126397CB620": "thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3",
		},
		allowedFlowTypes: flowTypeSet([]string{"bonds"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23460201",
		Type:   "bond",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "B833FC6746BB8B56C615ADB847370BE1F882A6179E14059AE982F126397CB620",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "80000000000000"},
				},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: bondModuleAddress,
				TxID:    "B833FC6746BB8B56C615ADB847370BE1F882A6179E14059AE982F126397CB620",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "80000000000000"},
				},
			},
		},
		Metadata: midgardActionMetadata{
			Bond: &midgardBondMetadata{
				Memo:        "BOND:thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3:thor1provider:2000",
				NodeAddress: "thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3",
				Provider:    "thor1provider",
				Fee:         "2000",
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 bond segment, got %d", len(segments))
	}
	seg := segments[0]
	if seg.Source.Kind != "external_address" {
		t.Fatalf("expected wallet source, got %#v", seg.Source)
	}
	if seg.Source.Address != "thor1provider" {
		t.Fatalf("expected provider source address, got %#v", seg.Source)
	}
	if seg.Target.Kind != "node" {
		t.Fatalf("expected validator target, got %#v", seg.Target)
	}
	if seg.Source.Address == bondModuleAddress || seg.Target.Address == bondModuleAddress {
		t.Fatalf("did not expect bond module address in projected segment: %#v", seg)
	}
}

func TestProjectMidgardRebondMapsOldBondWalletToNewBondWallet(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:   map[string][]int64{},
		actorsByID: map[int64]Actor{},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{"THOR.RUNE": 2},
		},
		bondMemoNodeByTx: map[string]string{
			"F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA": "thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk",
		},
		allowedFlowTypes: flowTypeSet([]string{"bonds"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1759108019365569722",
		Height: "23034924",
		Type:   "rebond",
		Status: "success",
		In: []midgardActionLeg{{
			Address: "thor1mlucvrd56xrhac4zqqx6yku84a6e5edj6k8una",
			TxID:    "F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA",
			Coins: []midgardActionCoin{
				{Asset: "THOR.RUNE", Amount: "31018731723188"},
			},
		}},
		Out: []midgardActionLeg{{
			Address: "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0",
			TxID:    "F747ABEF3F185B4DB133B19F6F971F132FECCB6BE8271413B923021DE0FEDDCA",
			Coins: []midgardActionCoin{
				{Asset: "THOR.RUNE", Amount: "31018731723188"},
			},
		}},
		Metadata: midgardActionMetadata{
			Rebond: &midgardRebondMetadata{
				Memo:           "REBOND:thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk:thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu:31018731723188",
				NodeAddress:    "thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk",
				NewBondAddress: "thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu",
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 rebond segment, got %d", len(segments))
	}
	seg := segments[0]
	if seg.Source.Address != "thor1mlucvrd56xrhac4zqqx6yku84a6e5edj6k8una" {
		t.Fatalf("unexpected rebond source %#v", seg.Source)
	}
	if seg.Target.Address != "thor1yes8zxhq4p3r5rp9vt6rk9fdxqf2mnm43apumu" {
		t.Fatalf("unexpected rebond target %#v", seg.Target)
	}
	if seg.Target.Kind == "node" {
		t.Fatalf("did not expect rebond target to be a node: %#v", seg.Target)
	}
	if seg.ValidatorAddress != "thor1z6lg2u2kxccnmz3xy65856mcuslwaxcvx56uuk" {
		t.Fatalf("expected validator metadata on rebond segment, got %#v", seg)
	}
}

func TestProjectMidgardBondSkipsWhenMemoNodeMissing(t *testing.T) {
	builder := &graphBuilder{
		ownerMap:         map[string][]int64{},
		actorsByID:       map[int64]Actor{},
		protocols:        protocolDirectory{AddressKinds: map[string]protocolAddress{}},
		prices:           priceBook{AssetUSD: map[string]float64{"THOR.RUNE": 2}},
		bondMemoNodeByTx: map[string]string{},
		allowedFlowTypes: flowTypeSet([]string{"bonds"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23460201",
		Type:   "bond",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "B833FC6746BB8B56C615ADB847370BE1F882A6179E14059AE982F126397CB620",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "80000000000000"},
				},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: bondModuleAddress,
				TxID:    "B833FC6746BB8B56C615ADB847370BE1F882A6179E14059AE982F126397CB620",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "80000000000000"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 0 {
		t.Fatalf("expected no bond segments when memo node is missing, got %d", len(segments))
	}
}

func TestParseBondMemoNodeAddress(t *testing.T) {
	if got := parseBondMemoNodeAddress("BOND:thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3"); got != "thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3" {
		t.Fatalf("unexpected bond memo node: %q", got)
	}
	if got := parseBondMemoNodeAddress("UNBOND:thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3"); got != "thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3" {
		t.Fatalf("unexpected unbond memo node: %q", got)
	}
	if got := parseBondMemoNodeAddress("REBOND:thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3:thor1newbond"); got != "thor1m3fruxmw0x6a8hed6a6jns07h669uhcuc2jdg3" {
		t.Fatalf("unexpected rebond memo node: %q", got)
	}
	if got := parseBondMemoNodeAddress("SWAP:BTC.BTC:bc1abc"); got != "" {
		t.Fatalf("expected empty memo node for non-bond memo, got %q", got)
	}
}

func TestHydrateBondMemoNodeCacheUsesRebondMetadata(t *testing.T) {
	app := &App{}
	cache := map[string]string{}

	app.hydrateBondMemoNodeCache(context.Background(), []midgardAction{{
		Type: "rebond",
		In: []midgardActionLeg{{
			TxID: "rebondtx",
		}},
		Metadata: midgardActionMetadata{
			Rebond: &midgardRebondMetadata{
				Memo:           "REBOND:thor1validator:thor1newbond:123",
				NodeAddress:    "thor1validator",
				NewBondAddress: "thor1newbond",
			},
		},
	}}, cache)

	if got := cache["REBONDTX"]; got != "thor1validator" {
		t.Fatalf("expected rebond tx to hydrate validator node, got %q", got)
	}
}

func TestThorNodeDisplayLabelReflectsStatus(t *testing.T) {
	if got := thorNodeDisplayLabel("thor1active", "Active"); got != "Validator thor1active" {
		t.Fatalf("unexpected active label %q", got)
	}
	if got := thorNodeDisplayLabel("thor1white", "Whitelisted"); got != "Whitelisted Node thor1white" {
		t.Fatalf("unexpected whitelisted label %q", got)
	}
	if got := thorNodeDisplayLabel("thor1standby", "Standby"); got != "Standby Node thor1standby" {
		t.Fatalf("unexpected standby label %q", got)
	}
	if got := thorNodeDisplayLabel("thor1unknown", "Unknown"); got != "Node thor1unknown" {
		t.Fatalf("unexpected fallback label %q", got)
	}
}

func TestProjectMidgardSwapSkipsAsgardAndStaysDirect(t *testing.T) {
	builder := &graphBuilder{
		ownerMap: map[string][]int64{
			normalizeAddress("thor1seed"): {1},
		},
		actorsByID: map[int64]Actor{
			1: {ID: 1, Name: "Alpha", Color: "#4ca3ff"},
		},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.RUNE": 2,
				"BTC.BTC":   100000,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "swap",
		Status: "success",
		Pools:  []string{"BTC.BTC"},
		In: []midgardActionLeg{
			{
				Address: "thor1seed",
				TxID:    "TXSWAP1",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "500000000"},
				},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0",
				TxID:    "TXSWAP1",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "500000000"},
				},
			},
			{
				Address: "bc1qrecipient000000000000000000000000000000",
				TxID:    "TXSWAP1",
				Coins: []midgardActionCoin{
					{Asset: "BTC.BTC", Amount: "250000"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 direct swap segment, got %d", len(segments))
	}

	seg := segments[0]
	if seg.Source.Address == asgardModuleAddress || seg.Target.Address == asgardModuleAddress {
		t.Fatalf("unexpected Asgard node in segment: %#v", seg)
	}
	if seg.Source.Kind == "pool" || seg.Target.Kind == "pool" {
		t.Fatalf("did not expect pool node in swap segment: %#v", seg)
	}
	if seg.Source.Address != normalizeAddress("thor1seed") || seg.Target.Address != normalizeAddress("bc1qrecipient000000000000000000000000000000") {
		t.Fatalf("unexpected swap path: %s -> %s", seg.Source.Address, seg.Target.Address)
	}
}

func TestProjectMidgardSwapSuppressesFeeLikeOutLegs(t *testing.T) {
	builder := &graphBuilder{
		ownerMap: map[string][]int64{
			normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k"): {2},
		},
		actorsByID: map[int64]Actor{
			2: {ID: 2, Name: "Treasury", Color: "#58ff4d"},
		},
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.RUNE": 2,
				"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1756836185894825498",
		Height: "22669324",
		Type:   "swap",
		Status: "success",
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "7ABE8DD0A5C1FFC667657693325A317315064CDB070CCF5D8ABDFA27C0F69721",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "30000000000000"},
				},
			},
		},
		Out: []midgardActionLeg{
			{
				Address: "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3",
				TxID:    "22E2212EE09D73FD74332B9FEC0A989A5471F014D81E9AF9B4684E5988712A1C",
				Coins: []midgardActionCoin{
					{Asset: "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", Amount: "35202664243700"},
				},
			},
			{
				Address: "thor1jq0huwma0gvujn7a4mlygw8kutdmja5kza4e5p",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "29966060900"},
				},
			},
			{
				Address: "thor1dl7un46w7l7f3ewrnrm6nq58nerjtp0dradjtd",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "104783017700"},
				},
			},
		},
	}

	segments, _, warnings := builder.projectMidgardAction(action, 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 canonical swap segment after fee suppression, got %d", len(segments))
	}
	seg := segments[0]
	if seg.Target.Address != normalizeAddress("0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3") {
		t.Fatalf("expected canonical recipient leg, got target=%s", seg.Target.Address)
	}
	if seg.Asset != normalizeAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48") {
		t.Fatalf("expected canonical out asset, got %s", seg.Asset)
	}
	if builder.feeActionDrop != 2 {
		t.Fatalf("expected 2 suppressed fee-like legs, got %d", builder.feeActionDrop)
	}
}

func TestProjectExternalTokenTransferCarriesTokenMetadata(t *testing.T) {
	builder := &graphBuilder{
		prices: priceBook{
			AssetUSD: map[string]float64{
				"ETH.USDC-0XABC": 1,
			},
			PoolAssets:  map[string]struct{}{"ETH.USDC-0XABC": {}},
			HasPoolData: true,
		},
		allowedFlowTypes: flowTypeSet([]string{"transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	transfer := externalTransfer{
		Chain:         "ETH",
		Asset:         "ETH.USDC-0XABC",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0xabc",
		TokenSymbol:   "USDC",
		TokenName:     "USD Coin",
		TokenDecimals: 6,
		AmountRaw:     "100000000",
		From:          "0xfrom",
		To:            "0xto",
		TxID:          "0xTXMETA",
		Time:          time.Unix(100, 0).UTC(),
		ActionKey:     "tracker.evm.token_transfer",
		ActionLabel:   "ETH Token Transfer",
		Confidence:    0.97,
	}

	segments, _ := builder.projectExternalTransfer(transfer, 1)
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	builder.addProjectedSegment(segments[0])
	if len(builder.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(builder.edges))
	}
	for _, edge := range builder.edges {
		if len(edge.Assets) != 1 {
			t.Fatalf("expected 1 edge asset, got %#v", edge.Assets)
		}
		asset := edge.Assets[0]
		if asset.AssetKind != "fungible_token" || asset.TokenStandard != "erc20" {
			t.Fatalf("unexpected edge token metadata: %#v", asset)
		}
		if asset.TokenAddress != "0xabc" || asset.TokenSymbol != "USDC" || asset.TokenName != "USD Coin" || asset.TokenDecimals != 6 {
			t.Fatalf("unexpected edge token fields: %#v", asset)
		}
	}
	for _, action := range builder.actions {
		if action.TokenAddress != "0xabc" || action.TokenSymbol != "USDC" || action.AssetKind != "fungible_token" {
			t.Fatalf("unexpected supporting action token fields: %#v", action)
		}
	}
}

func TestProjectExternalUnsupportedTokenTransferDroppedWhenPoolBookAvailable(t *testing.T) {
	builder := &graphBuilder{
		prices: priceBook{
			AssetUSD:    map[string]float64{"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48": 1},
			PoolAssets:  map[string]struct{}{"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48": {}},
			HasPoolData: true,
		},
		allowedFlowTypes: flowTypeSet([]string{"transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	transfer := externalTransfer{
		Chain:         "ETH",
		Asset:         "ETH.USDC-0X55EEB8EBA481B5484E67B06B504762BA0486CF74",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0x55eeb8eba481b5484e67b06b504762ba0486cf74",
		TokenSymbol:   "USDC",
		TokenName:     "USD Coin",
		TokenDecimals: 6,
		AmountRaw:     "100000000",
		From:          "0xfrom",
		To:            "0xto",
		TxID:          "0xTXSCAM",
		Time:          time.Unix(100, 0).UTC(),
		ActionKey:     "tracker.evm.token_transfer",
		ActionLabel:   "ETH Token Transfer",
		Confidence:    0.97,
	}

	segments, next := builder.projectExternalTransfer(transfer, 1)
	if len(segments) != 0 || len(next) != 0 {
		t.Fatalf("expected unsupported token transfer to be dropped, got segments=%#v next=%#v", segments, next)
	}
}

func TestProjectExternalUnsupportedTokenTransferRetainedWithoutPoolBook(t *testing.T) {
	builder := &graphBuilder{
		prices:           priceBook{},
		allowedFlowTypes: flowTypeSet([]string{"transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	transfer := externalTransfer{
		Chain:         "ETH",
		Asset:         "ETH.USDC-0X55EEB8EBA481B5484E67B06B504762BA0486CF74",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0x55eeb8eba481b5484e67b06b504762ba0486cf74",
		TokenSymbol:   "USDC",
		TokenName:     "USD Coin",
		TokenDecimals: 6,
		AmountRaw:     "100000000",
		From:          "0xfrom",
		To:            "0xto",
		TxID:          "0xTXNOPOOLBOOK",
		Time:          time.Unix(100, 0).UTC(),
		ActionKey:     "tracker.evm.token_transfer",
		ActionLabel:   "ETH Token Transfer",
		Confidence:    0.97,
	}

	segments, _ := builder.projectExternalTransfer(transfer, 1)
	if len(segments) != 1 {
		t.Fatalf("expected token transfer to remain visible without pool book, got %d segments", len(segments))
	}
}

func TestProjectExternalTokenTransferNotFilteredWhenUnpriced(t *testing.T) {
	builder := &graphBuilder{
		minUSD:           1000,
		allowedFlowTypes: flowTypeSet([]string{"transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}
	transfer := externalTransfer{
		Chain:         "ETH",
		Asset:         "ETH.UNKNOWN-0XABC",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0xabc",
		TokenSymbol:   "UNKNOWN",
		AmountRaw:     "100000000",
		From:          "0xfrom",
		To:            "0xto",
		TxID:          "0xTXUNK",
		Time:          time.Unix(100, 0).UTC(),
	}

	segments, _ := builder.projectExternalTransfer(transfer, 1)
	if len(segments) != 1 {
		t.Fatalf("expected unpriced token transfer to remain visible, got %d segments", len(segments))
	}
}

func TestProjectMidgardActionWithExternalStitchesTokenSwapSource(t *testing.T) {
	builder := &graphBuilder{
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{
				normalizeAddress("0xinbound"): {Kind: "inbound", Chain: "ETH", Label: "ETH Inbound"},
			},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"ETH.USDC-0XABC": 1,
				"BTC.BTC":        100000,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "swap",
		Status: "success",
		Pools:  []string{"BTC.BTC"},
		In: []midgardActionLeg{{
			Address: "0xinbound",
			TxID:    "INTX1",
			Coins: []midgardActionCoin{
				{Asset: "ETH.USDC-0XABC", Amount: "100000000"},
			},
		}},
		Out: []midgardActionLeg{{
			Address: "0xrecipient",
			TxID:    "OUTTX1",
			Coins: []midgardActionCoin{
				{Asset: "BTC.BTC", Amount: "250000"},
			},
		}},
	}
	external := []externalTransfer{{
		Chain:         "ETH",
		Asset:         "ETH.USDC-0XABC",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0xabc",
		TokenSymbol:   "USDC",
		AmountRaw:     "100000000",
		From:          "0xsender",
		To:            "0xinbound",
		TxID:          "INTX1",
		Time:          time.Unix(100, 0).UTC(),
	}}

	segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, external)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 stitched swap segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress("0xsender") || segments[0].Target.Address != normalizeAddress("0xrecipient") {
		t.Fatalf("unexpected stitched path: %#v", segments[0])
	}
	if _, ok := consumed[externalTransferKey(external[0])]; !ok {
		t.Fatal("expected inbound external transfer to be consumed by stitch")
	}
}

func TestProjectMidgardActionWithExternalConsumesLiquidityWithdrawTransit(t *testing.T) {
	builder := &graphBuilder{
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{
				normalizeAddress("0xrouter"): {Kind: "router", Chain: "ETH", Label: "ETH Router"},
			},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"ETH.USDC-0XABC": 1,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "withdraw_liquidity",
		Status: "success",
		Pools:  []string{"ETH.USDC-0XABC"},
		Out: []midgardActionLeg{{
			Address: "0xrecipient",
			TxID:    "OUTTX2",
			Coins: []midgardActionCoin{
				{Asset: "ETH.USDC-0XABC", Amount: "100000000"},
			},
		}},
	}
	external := []externalTransfer{{
		Chain:         "ETH",
		Asset:         "ETH.USDC-0XABC",
		AssetKind:     "fungible_token",
		TokenStandard: "erc20",
		TokenAddress:  "0xabc",
		TokenSymbol:   "USDC",
		AmountRaw:     "100000000",
		From:          "0xrouter",
		To:            "0xrecipient",
		TxID:          "OUTTX2",
		Time:          time.Unix(100, 0).UTC(),
	}}

	segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, external)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 liquidity withdraw segment, got %d", len(segments))
	}
	if segments[0].Source.Kind != "pool" || segments[0].Target.Address != normalizeAddress("0xrecipient") {
		t.Fatalf("unexpected liquidity withdraw path: %#v", segments[0])
	}
	if _, ok := consumed[externalTransferKey(external[0])]; !ok {
		t.Fatal("expected outbound transit transfer to be consumed by stitch")
	}
}

func TestFetchMidgardActionsForAddressCanonicalizesShadowSendAcrossCache(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		hits++
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: []midgardAction{
				{
					Date:   "1769206297553545093",
					Height: "24649420",
					Type:   "swap",
					Status: "success",
					In: []midgardActionLeg{{
						Address: "thor1sender",
						TxID:    "INTX",
						Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
					}},
					Out: []midgardActionLeg{{
						Address: "0xrecipient",
						TxID:    "OUTTX",
						Coins:   []midgardActionCoin{{Amount: "58247165323", Asset: "BSC.USDT-0X55D398326F99059FF775485246999027B3197955"}},
					}},
					Pools: []string{"BSC.USDT-0X55D398326F99059FF775485246999027B3197955"},
				},
				{
					Date:   "1769206297553545093",
					Height: "24649420",
					Type:   "send",
					Status: "success",
					In: []midgardActionLeg{{
						Address: "thor1sender",
						TxID:    "INTX",
						Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
					}},
					Out: []midgardActionLeg{
						{
							Address: "thor1transit",
							TxID:    "INTX",
							Coins:   []midgardActionCoin{{Amount: "100000000000", Asset: "THOR.RUNE"}},
						},
						{
							Address: "0xrecipient",
							TxID:    "OUTTX",
							Coins:   []midgardActionCoin{{Amount: "58247165323", Asset: "BSC.USDT-0X55D398326F99059FF775485246999027B3197955"}},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "chain-analysis.db"),
		StaticDir:         "internal/web/static",
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	start := time.Unix(1769206297, 0).UTC().Add(-time.Hour)
	end := time.Unix(1769206297, 0).UTC().Add(time.Hour)

	for i := 0; i < 2; i++ {
		actions, truncated, err := app.fetchMidgardActionsForAddress(context.Background(), "thor1sender", start, end, 1)
		if err != nil {
			t.Fatalf("fetchMidgardActionsForAddress call %d: %v", i+1, err)
		}
		if truncated {
			t.Fatalf("did not expect truncation on call %d", i+1)
		}
		if len(actions) != 1 {
			t.Fatalf("expected canonicalized action list on call %d, got %#v", i+1, actions)
		}
		if got := strings.ToLower(strings.TrimSpace(actions[0].Type)); got != "swap" {
			t.Fatalf("expected swap action on call %d, got %q", i+1, got)
		}
	}

	if hits != 1 {
		t.Fatalf("expected second fetch to hit cache with canonicalized actions, upstream hits=%d", hits)
	}
}

func TestProjectMidgardActionWithExternalConsumesSecureTxTransfers(t *testing.T) {
	builder := &graphBuilder{
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{
				normalizeAddress("thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k"): {
					Kind:  "inbound",
					Chain: "BTC",
					Label: "BTC Inbound",
				},
			},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"BTC.BTC": 100000,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"transfers", "liquidity", "swaps"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "secure",
		Status: "success",
		In: []midgardActionLeg{{
			Address: "bc1qseed",
			TxID:    "SECURETX1",
			Coins: []midgardActionCoin{
				{Asset: "BTC.BTC", Amount: "3000000000"},
			},
		}},
		Out: []midgardActionLeg{{
			Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
			TxID:    "SECURETX1",
			Coins: []midgardActionCoin{
				{Asset: "BTC-BTC", Amount: "3000000000"},
			},
		}},
	}

	external := []externalTransfer{
		{
			Chain:       "BTC",
			Asset:       "BTC.BTC",
			AmountRaw:   "3000000000",
			From:        "bc1qseed",
			To:          "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
			TxID:        "SECURETX1",
			Time:        time.Unix(100, 0).UTC(),
			ActionKey:   "tracker.utxo.transfer",
			ActionLabel: "BTC Transfer",
		},
		{
			Chain:       "BTC",
			Asset:       "BTC.BTC",
			AmountRaw:   "1949627610",
			From:        "bc1qseed",
			To:          "bc1qchange",
			TxID:        "SECURETX1",
			Time:        time.Unix(100, 0).UTC(),
			ActionKey:   "tracker.utxo.transfer",
			ActionLabel: "BTC Transfer",
		},
	}

	segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, external)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 secure segment, got %d", len(segments))
	}
	for i := range external {
		if _, ok := consumed[externalTransferKey(external[i])]; !ok {
			t.Fatalf("expected secure tx transfer %d to be consumed", i)
		}
	}
}

func TestProjectMidgardActionWithExternalConsumesAddLiquidityTxTransfers(t *testing.T) {
	builder := &graphBuilder{
		protocols: protocolDirectory{
			AddressKinds: map[string]protocolAddress{
				normalizeAddress("TRrUWtaSJS1G1Aqa5PdBJULLESGp5iTjQk"): {
					Kind:  "inbound",
					Chain: "TRON",
					Label: "TRON Inbound",
				},
			},
		},
		prices: priceBook{
			AssetUSD: map[string]float64{
				"THOR.RUNE": 5,
				"TRON.TRX":  0.2,
			},
		},
		allowedFlowTypes: flowTypeSet([]string{"transfers", "liquidity", "swaps"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}

	action := midgardAction{
		Date:   "1763572616337487627",
		Height: "23747777",
		Type:   "addLiquidity",
		Status: "success",
		Pools:  []string{"TRON.TRX"},
		In: []midgardActionLeg{
			{
				Address: "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
				TxID:    "RUNETX1",
				Coins: []midgardActionCoin{
					{Asset: "THOR.RUNE", Amount: "2854000000000"},
				},
			},
			{
				Address: "TJ8LE5jBifN2CQmgE5TLiLZGFGVNny1dHT",
				TxID:    "LIQTX1",
				Coins: []midgardActionCoin{
					{Asset: "TRON.TRX", Amount: "8970000000000"},
				},
			},
		},
	}

	external := []externalTransfer{
		{
			Chain:       "TRON",
			Asset:       "TRON.TRX",
			AmountRaw:   "8970000000000",
			From:        "TJ8LE5jBifN2CQmgE5TLiLZGFGVNny1dHT",
			To:          "TRrUWtaSJS1G1Aqa5PdBJULLESGp5iTjQk",
			TxID:        "LIQTX1",
			Time:        time.Unix(100, 0).UTC(),
			ActionKey:   "tracker.tron.transfer",
			ActionLabel: "TRON Transfer",
		},
		{
			Chain:       "TRON",
			Asset:       "TRON.TRX",
			AmountRaw:   "1",
			From:        "TJ8LE5jBifN2CQmgE5TLiLZGFGVNny1dHT",
			To:          "TFeeAddress1111111111111111111111111",
			TxID:        "LIQTX1",
			Time:        time.Unix(100, 0).UTC(),
			ActionKey:   "tracker.tron.transfer",
			ActionLabel: "TRON Transfer",
		},
	}

	segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, external)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(segments) == 0 {
		t.Fatal("expected add-liquidity segments")
	}
	for i := range external {
		if _, ok := consumed[externalTransferKey(external[i])]; !ok {
			t.Fatalf("expected add-liquidity tx transfer %d to be consumed", i)
		}
	}
}

func TestCollectMidgardRefundTxIDs(t *testing.T) {
	actions := []midgardAction{
		{
			Type:   "refund",
			Status: "success",
			In: []midgardActionLeg{
				{TxID: "abc123"},
			},
		},
		{
			Type:   "swap",
			Status: "success",
			In: []midgardActionLeg{
				{TxID: "swaptx"},
			},
		},
	}
	got := collectMidgardRefundTxIDs(actions)
	if len(got) != 1 {
		t.Fatalf("expected 1 refund tx id, got %#v", got)
	}
	if _, ok := got["ABC123"]; !ok {
		t.Fatalf("expected refund tx ABC123, got %#v", got)
	}
}

func TestShouldSkipMidgardActionForGraph(t *testing.T) {
	refundAction := midgardAction{
		Type:   "refund",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "ABC123"},
		},
	}
	if skip, reason := shouldSkipMidgardActionForGraph(refundAction, nil, nil, nil, nil); !skip || reason != "refund_action" {
		t.Fatalf("expected refund_action skip, got skip=%v reason=%q", skip, reason)
	}

	associatedAction := midgardAction{
		Type:   "swap",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "ABC123"},
		},
	}
	if skip, reason := shouldSkipMidgardActionForGraph(associatedAction, map[string]struct{}{"ABC123": {}}, nil, nil, nil); !skip || reason != "refund_associated" {
		t.Fatalf("expected refund_associated skip, got skip=%v reason=%q", skip, reason)
	}
}

func TestShouldSkipExternalTransferForGraph(t *testing.T) {
	transfer := externalTransfer{
		TxID: "abc123",
	}
	if skip, reason := shouldSkipExternalTransferForGraph(transfer, map[string]struct{}{"ABC123": {}}, nil); !skip || reason != "refund_associated" {
		t.Fatal("expected external transfer suppression for refund-correlated tx")
	}
	if skip, _ := shouldSkipExternalTransferForGraph(externalTransfer{TxID: "DEF456"}, map[string]struct{}{"ABC123": {}}, nil); skip {
		t.Fatal("did not expect suppression for unrelated external transfer")
	}
}

func TestShouldSkipMidgardActionForGraphSkipsLiquidityFeeAction(t *testing.T) {
	feeAction := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "FEE123"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-fin/market-maker.fee",
			},
		},
	}
	if skip, reason := shouldSkipMidgardActionForGraph(feeAction, nil, nil, nil, nil); !skip || reason != "liquidity_fee_action" {
		t.Fatalf("expected liquidity_fee_action skip, got skip=%v reason=%q", skip, reason)
	}
}

func TestShouldSkipMidgardActionForGraphPrefersProcessOverManagerUpdate(t *testing.T) {
	managerUpdate := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "CALCTX1"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-manager/strategy.update",
			},
		},
	}
	process := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "CALCTX1"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
			},
		},
	}

	if skip, reason := shouldSkipMidgardActionForGraph(managerUpdate, nil, nil, map[string]struct{}{"CALCTX1": {}}, map[string]struct{}{"CALCTX1": {}}); !skip || reason != "contract_sub_execution" {
		t.Fatalf("expected manager.update to be skipped when process exists, got skip=%v reason=%q", skip, reason)
	}
	if skip, reason := shouldSkipMidgardActionForGraph(process, nil, nil, map[string]struct{}{"CALCTX1": {}}, map[string]struct{}{"CALCTX1": {}}); skip {
		t.Fatalf("did not expect process representative to be skipped, got reason=%q", reason)
	}
	if skip, reason := shouldSkipMidgardActionForGraph(managerUpdate, nil, nil, map[string]struct{}{"CALCTX1": {}}, nil); skip {
		t.Fatalf("did not expect manager.update fallback to be skipped without process, got reason=%q", reason)
	}
}

func TestShouldSkipMidgardActionForGraphDoesNotTreatRepresentativeAsFeeAssociated(t *testing.T) {
	process := midgardAction{
		Type:   "contract",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "FEECTX1"},
		},
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-strategy/process",
			},
		},
	}
	swap := midgardAction{
		Type:   "swap",
		Status: "success",
		In: []midgardActionLeg{
			{TxID: "FEECTX1"},
		},
	}

	if skip, reason := shouldSkipMidgardActionForGraph(process, nil, map[string]struct{}{"FEECTX1": {}}, nil, nil); skip {
		t.Fatalf("did not expect process representative to be skipped as fee-associated, got reason=%q", reason)
	}
	if skip, reason := shouldSkipMidgardActionForGraph(swap, nil, map[string]struct{}{"FEECTX1": {}}, nil, nil); !skip || reason != "liquidity_fee_associated" {
		t.Fatalf("expected swap to be skipped as fee-associated, got skip=%v reason=%q", skip, reason)
	}
}

func TestShouldSkipExternalTransferForGraphSkipsLiquidityFeeAssociated(t *testing.T) {
	transfer := externalTransfer{TxID: "fee123"}
	if skip, reason := shouldSkipExternalTransferForGraph(transfer, nil, map[string]struct{}{"FEE123": {}}); !skip || reason != "liquidity_fee_associated" {
		t.Fatalf("expected liquidity_fee_associated skip, got skip=%v reason=%q", skip, reason)
	}
}

func TestCollectMidgardLiquidityFeeTxIDs(t *testing.T) {
	actions := []midgardAction{
		{
			Type:   "contract",
			Status: "success",
			In: []midgardActionLeg{
				{TxID: "fee123"},
			},
			Metadata: midgardActionMetadata{
				Contract: &midgardContractMetadata{
					ContractType: "wasm-rujira-fin/market-maker.fee",
				},
			},
		},
		{
			Type:   "swap",
			Status: "success",
			In: []midgardActionLeg{
				{TxID: "SWAP123"},
			},
		},
	}
	got := collectMidgardLiquidityFeeTxIDs(actions)
	if len(got) != 1 {
		t.Fatalf("expected 1 liquidity fee tx id, got %#v", got)
	}
	if _, ok := got["FEE123"]; !ok {
		t.Fatalf("expected fee tx FEE123, got %#v", got)
	}
}

func TestAddProjectedSegmentDedupesCanonicalSwap(t *testing.T) {
	builder := &graphBuilder{
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
		seenCanonicalKey: map[string]struct{}{},
	}
	source := flowRef{
		ID:      "external_address:thor1source:external:1",
		Key:     normalizeAddress("thor1source"),
		Kind:    "external_address",
		Label:   "Source",
		Chain:   "THOR",
		Stage:   "external",
		Depth:   1,
		Address: normalizeAddress("thor1source"),
	}
	target := flowRef{
		ID:      "external_address:thor1target:external:2",
		Key:     normalizeAddress("thor1target"),
		Kind:    "external_address",
		Label:   "Target",
		Chain:   "THOR",
		Stage:   "external",
		Depth:   2,
		Address: normalizeAddress("thor1target"),
	}
	seg := projectedSegment{
		Source:       source,
		Target:       target,
		ActionClass:  "swaps",
		ActionKey:    "event.swap",
		ActionLabel:  "Swap",
		ActionDomain: "swaps",
		Asset:        "BTC.BTC",
		AmountRaw:    "100",
		USDSpot:      1,
		TxID:         "ABC123",
		Height:       1,
		Time:         time.Now().UTC(),
		CanonicalKey: canonicalSwapSegmentKey("ABC123", source.Address, target.Address, "BTC.BTC"),
	}

	builder.addProjectedSegment(seg)
	builder.addProjectedSegment(seg)

	if len(builder.edges) != 1 {
		t.Fatalf("expected canonical swap dedupe to keep 1 edge, got %d", len(builder.edges))
	}
	if builder.swapEmitted != 1 || builder.swapDeduped != 1 {
		t.Fatalf("unexpected swap counters emitted=%d deduped=%d", builder.swapEmitted, builder.swapDeduped)
	}
}

func TestAddProjectedSegmentSplitsRebondEdgesByValidator(t *testing.T) {
	builder := &graphBuilder{
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
		seenCanonicalKey: map[string]struct{}{},
	}
	source := flowRef{
		ID:      "external_address:thor1old:external:1",
		Key:     normalizeAddress("thor1old"),
		Kind:    "external_address",
		Label:   "Old Bond Wallet",
		Chain:   "THOR",
		Stage:   "external",
		Depth:   1,
		Address: normalizeAddress("thor1old"),
	}
	target := flowRef{
		ID:      "external_address:thor1new:external:2",
		Key:     normalizeAddress("thor1new"),
		Kind:    "external_address",
		Label:   "New Bond Wallet",
		Chain:   "THOR",
		Stage:   "external",
		Depth:   2,
		Address: normalizeAddress("thor1new"),
	}
	now := time.Now().UTC()

	builder.addProjectedSegment(projectedSegment{
		Source:           source,
		Target:           target,
		ActionClass:      "bonds",
		ActionKey:        "event.rebond",
		ActionLabel:      "Rebond via Validator thor1val...111",
		ActionDomain:     "bonds",
		ValidatorAddress: "thor1validator111",
		ValidatorLabel:   "Validator thor1val...111",
		Asset:            "THOR.RUNE",
		AmountRaw:        "100000000",
		USDSpot:          150,
		TxID:             "TXREBOND1",
		Height:           10,
		Time:             now,
	})
	builder.addProjectedSegment(projectedSegment{
		Source:           source,
		Target:           target,
		ActionClass:      "bonds",
		ActionKey:        "event.rebond",
		ActionLabel:      "Rebond via Validator thor1val...222",
		ActionDomain:     "bonds",
		ValidatorAddress: "thor1validator222",
		ValidatorLabel:   "Validator thor1val...222",
		Asset:            "THOR.RUNE",
		AmountRaw:        "200000000",
		USDSpot:          300,
		TxID:             "TXREBOND2",
		Height:           11,
		Time:             now.Add(time.Minute),
	})

	if len(builder.edges) != 2 {
		t.Fatalf("expected separate rebond edges per validator, got %d", len(builder.edges))
	}
	if len(builder.actions) != 2 {
		t.Fatalf("expected separate supporting actions per validator, got %d", len(builder.actions))
	}
	for _, edge := range builder.edges {
		if strings.TrimSpace(edge.ValidatorAddress) == "" {
			t.Fatalf("expected validator metadata on rebond edge, got %#v", edge)
		}
	}
}

func TestMidgardActionClassForContracts(t *testing.T) {
	swapAction := midgardAction{
		Type: "contract",
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-rujira-fin/trade",
				Msg:          map[string]any{},
			},
		},
	}
	if got := midgardActionClass(swapAction); got != "swaps" {
		t.Fatalf("expected swaps for contract trade, got %s", got)
	}

	liquidityAction := midgardAction{
		Type: "contract",
		Metadata: midgardActionMetadata{
			Contract: &midgardContractMetadata{
				ContractType: "wasm-calc-manager/strategy.update",
				Msg:          map[string]any{},
			},
		},
	}
	if got := midgardActionClass(liquidityAction); got != "liquidity" {
		t.Fatalf("expected liquidity for manager update, got %s", got)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actor-tracker-test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		t.Fatalf("set wal: %v", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("set fk: %v", err)
	}
	if err := initSchema(context.Background(), db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}
