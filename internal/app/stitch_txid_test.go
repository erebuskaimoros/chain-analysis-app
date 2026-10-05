package app

import (
	"testing"
	"time"
)

// Reproduces the Bitget exploiter graph: each ETH→BTC swap's inbound deposit
// shows up twice, once inside the swap and once as a plain ETH transfer to a
// vault, because the EVM tracker reports the hash as 0x-prefixed and the
// vault has since rotated out of inbound_addresses.
func TestStitchConsumesUserDepositWithPrefixedEVMTxID(t *testing.T) {
	const (
		user      = "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"
		oldVault  = "0x0dac1fb302bc428d8dc272a511e7c2f0d2a1c128"
		recipient = "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"
		inHash    = "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F"
		outHash   = "9CD94A8E5734DD6E4C77D0EB400F1C64935BE17AFC062185CB26264004E55CA9"
	)
	builder := &graphBuilder{
		protocols: protocolDirectory{AddressKinds: map[string]protocolAddress{}},
		prices: priceBook{AssetUSD: map[string]float64{
			"ETH.ETH": 2600,
			"BTC.BTC": 84000,
		}},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}
	action := midgardAction{
		Date:   "1790567751872949022",
		Height: "28010713",
		Type:   "swap",
		Status: "success",
		Pools:  []string{"ETH.ETH", "BTC.BTC"},
		In: []midgardActionLeg{{
			Address: user,
			TxID:    inHash,
			Coins:   []midgardActionCoin{{Asset: "ETH.ETH", Amount: "100000000"}},
		}},
		Out: []midgardActionLeg{{
			Address: recipient,
			TxID:    outHash,
			Coins:   []midgardActionCoin{{Asset: "BTC.BTC", Amount: "3166890"}},
		}},
	}
	deposit := externalTransfer{
		Chain:     "ETH",
		Asset:     "ETH.ETH",
		AssetKind: "native",
		AmountRaw: "100000000",
		From:      user,
		To:        oldVault,
		TxID:      "0x" + "a732e09aab76a571d768c2b5b4e2f0e1e5b1a9c3d4e5f60718293a4b5c6d7e8f",
		Time:      time.Unix(1790567723, 0).UTC(),
	}

	if got, want := cleanTxID(deposit.TxID), cleanTxID(inHash); got != want {
		t.Fatalf("expected the EVM hash to normalise to Midgard's form: got %q, want %q", got, want)
	}

	segments, _, _, consumed := builder.projectMidgardActionWithExternal(action, 1, []externalTransfer{deposit})
	if len(segments) != 1 {
		t.Fatalf("expected one swap segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress(user) || segments[0].Target.Address != normalizeAddress(recipient) {
		t.Fatalf("unexpected swap path: %#v", segments[0])
	}
	if _, ok := consumed[externalTransferKey(deposit)]; !ok {
		t.Fatal("expected the user's deposit to the rotated vault to be consumed by the swap, so it is not drawn as a separate transfer")
	}
}

// Reproduces the outbound half of the Bitget exploiter graph: each swap's BTC
// payout shows up twice, once as the swap edge and once as a plain BTC
// transfer from a vault that has since rotated out of inbound_addresses.
func TestStitchConsumesOutboundPayoutFromRotatedVault(t *testing.T) {
	const (
		user      = "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3"
		oldVault  = "bc1q0h3h0t48v60rx9cg88fq0gckmhh2mxu5lh6dcs"
		recipient = "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f"
		inHash    = "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F"
		outHash   = "9CD94A8E5734DD6E4C77D0EB400F1C64935BE17AFC062185CB26264004E55CA9"
	)
	builder := &graphBuilder{
		protocols: protocolDirectory{AddressKinds: map[string]protocolAddress{}},
		prices: priceBook{AssetUSD: map[string]float64{
			"ETH.ETH": 2600,
			"BTC.BTC": 84000,
		}},
		allowedFlowTypes: flowTypeSet([]string{"swaps", "liquidity", "transfers"}),
		nodes:            map[string]*FlowNode{},
		edges:            map[string]*FlowEdge{},
		actions:          map[string]*SupportingAction{},
	}
	action := midgardAction{
		Date:   "1790567751872949022",
		Height: "28010713",
		Type:   "swap",
		Status: "success",
		Pools:  []string{"ETH.ETH", "BTC.BTC"},
		In: []midgardActionLeg{{
			Address: user,
			TxID:    inHash,
			Coins:   []midgardActionCoin{{Asset: "ETH.ETH", Amount: "100000000"}},
		}},
		Out: []midgardActionLeg{{
			Address: recipient,
			TxID:    outHash,
			Coins:   []midgardActionCoin{{Asset: "BTC.BTC", Amount: "3166890"}},
		}},
	}
	// Esplora reports the hash in lower case.
	payout := externalTransfer{
		Chain:     "BTC",
		Asset:     "BTC.BTC",
		AssetKind: "native",
		AmountRaw: "3166890",
		From:      oldVault,
		To:        recipient,
		TxID:      "9cd94a8e5734dd6e4c77d0eb400f1c64935be17afc062185cb26264004e55ca9",
		Time:      time.Unix(1790567800, 0).UTC(),
	}
	// The same outbound tx also returns the vault's change; that is not the
	// recipient's payout and must stay.
	change := payout
	change.To = "bc1qchangeaddressxxxxxxxxxxxxxxxxxxxxxxxxxx"
	change.AmountRaw = "51234567"
	// A transfer to the recipient in the same tx with a different amount is
	// not this payout either.
	other := payout
	other.From = "bc1qunrelatedsenderxxxxxxxxxxxxxxxxxxxxxxx"
	other.AmountRaw = "9000000"

	segments, _, _, consumed := builder.projectMidgardActionWithExternal(action, 1, []externalTransfer{payout, change, other})
	if len(segments) != 1 {
		t.Fatalf("expected one swap segment, got %d", len(segments))
	}
	if segments[0].Source.Address != normalizeAddress(user) || segments[0].Target.Address != normalizeAddress(recipient) {
		t.Fatalf("unexpected swap path: %#v", segments[0])
	}
	if _, ok := consumed[externalTransferKey(payout)]; !ok {
		t.Fatal("expected the payout from the rotated vault to be consumed by the swap, so it is not drawn as a separate transfer")
	}
	if _, ok := consumed[externalTransferKey(change)]; ok {
		t.Fatal("the vault's change output is not the swap payout and must not be consumed")
	}
	if _, ok := consumed[externalTransferKey(other)]; ok {
		t.Fatal("a transfer with a different amount is not the swap payout and must not be consumed")
	}
}
