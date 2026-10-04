package app

import (
	"sort"
	"strings"
)

func canonicalizeMidgardLookupActions(actions []midgardAction) []midgardAction {
	if len(actions) < 2 {
		return actions
	}
	out := make([]midgardAction, 0, len(actions))
	for i, action := range actions {
		if isMidgardShadowSendActionForLookup(i, actions) {
			continue
		}
		out = append(out, action)
	}
	return out
}

func isMidgardShadowSendActionForLookup(index int, actions []midgardAction) bool {
	if index < 0 || index >= len(actions) {
		return false
	}
	candidate := actions[index]
	if strings.ToLower(strings.TrimSpace(candidate.Type)) != "send" {
		return false
	}
	candidateOut := midgardLookupLegSignatures(candidate.Out)
	if len(candidateOut) == 0 {
		return false
	}
	candidateIn := midgardLookupLegSignatures(candidate.In)
	for i, other := range actions {
		if i == index {
			continue
		}
		if midgardActionClass(other) == "transfers" {
			continue
		}
		if strings.TrimSpace(candidate.Height) != "" && strings.TrimSpace(other.Height) != "" && candidate.Height != other.Height {
			continue
		}
		if strings.TrimSpace(candidate.Date) != "" && strings.TrimSpace(other.Date) != "" && candidate.Date != other.Date {
			continue
		}
		if !hasMidgardLookupLegIntersection(candidateOut, midgardLookupLegSignatures(other.Out)) {
			continue
		}
		otherIn := midgardLookupLegSignatures(other.In)
		if len(candidateIn) > 0 && len(otherIn) > 0 && !hasMidgardLookupLegIntersection(candidateIn, otherIn) {
			continue
		}
		return true
	}
	return false
}

func midgardLookupLegSignatures(legs []midgardActionLeg) map[string]struct{} {
	if len(legs) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, leg := range legs {
		sig := midgardLookupLegSignature(leg)
		if sig == "" {
			continue
		}
		out[sig] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func midgardLookupLegSignature(leg midgardActionLeg) string {
	address := normalizeAddress(leg.Address)
	txID := cleanTxID(leg.TxID)
	coinSigs := make([]string, 0, len(leg.Coins))
	for _, coin := range leg.Coins {
		asset := normalizeAsset(coin.Asset)
		amount := strings.TrimSpace(coin.Amount)
		if asset == "" && amount == "" {
			continue
		}
		coinSigs = append(coinSigs, asset+":"+amount)
	}
	sort.Strings(coinSigs)
	if address == "" && txID == "" && len(coinSigs) == 0 {
		return ""
	}
	return strings.Join([]string{address, txID, strings.Join(coinSigs, ",")}, "|")
}

func hasMidgardLookupLegIntersection(left, right map[string]struct{}) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for key := range left {
		if _, ok := right[key]; ok {
			return true
		}
	}
	return false
}
