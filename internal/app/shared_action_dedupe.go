package app

import "strings"

// midgardActionLedger tracks Midgard actions across the traced (frontier)
// addresses of one graph build. Each frontier keeps only the projected
// segments that touch it, so an action returned by several traced addresses'
// histories (for example a symmetric add-liquidity with a RUNE leg and an
// asset leg owned by the same actor) is projected once per frontier. The ledger
// keeps those projections from counting any segment twice.
type midgardActionLedger struct {
	// suppressed holds actions dropped by a graph-wide rule (refund, liquidity
	// fee, contract sub-execution, Rujira trace). These are properties of the
	// action, so the decision applies to every frontier.
	suppressed map[string]struct{}
	projected  map[string]*midgardActionProjection
}

type midgardActionProjection struct {
	// stitched is the action as first emitted, after external-transfer
	// stitching. Later frontiers re-project this form so every projection of
	// the action resolves its legs to the same addresses and segment
	// identities stay comparable.
	stitched midgardAction
	// emitted counts the segments already added to the graph by identity. A
	// count rather than a set keeps segments that legitimately repeat within
	// one projection.
	emitted map[string]int
}

func newMidgardActionLedger() *midgardActionLedger {
	return &midgardActionLedger{
		suppressed: map[string]struct{}{},
		projected:  map[string]*midgardActionProjection{},
	}
}

func (l *midgardActionLedger) isSuppressed(key string) bool {
	_, ok := l.suppressed[key]
	return ok
}

func (l *midgardActionLedger) suppress(key string) {
	l.suppressed[key] = struct{}{}
}

// projectedAction returns the stitched form of an action that an earlier
// frontier already emitted segments for.
func (l *midgardActionLedger) projectedAction(key string) (midgardAction, bool) {
	projection, ok := l.projected[key]
	if !ok {
		return midgardAction{}, false
	}
	return projection.stitched, true
}

// claim records the frontier's segments for an action and returns only those
// not already emitted for the action by another frontier. A segment touching
// two traced addresses (an A -> B transfer where both are traced) therefore
// reaches the graph once. The first non-empty claim fixes the stitched form
// that later frontiers re-project.
func (l *midgardActionLedger) claim(key string, stitched midgardAction, segments []projectedSegment) []projectedSegment {
	if len(segments) == 0 {
		return nil
	}
	projection, ok := l.projected[key]
	if !ok {
		projection = &midgardActionProjection{
			stitched: cloneMidgardActionLegs(stitched),
			emitted:  map[string]int{},
		}
		l.projected[key] = projection
	}
	counts := make(map[string]int, len(segments))
	fresh := make([]projectedSegment, 0, len(segments))
	for _, segment := range segments {
		identity := projectedSegmentIdentity(segment)
		counts[identity]++
		if counts[identity] > projection.emitted[identity] {
			fresh = append(fresh, segment)
		}
	}
	for identity, count := range counts {
		if count > projection.emitted[identity] {
			projection.emitted[identity] = count
		}
	}
	return fresh
}

// projectedSegmentIdentity names a segment by the graph data it contributes.
// Nodes are keyed the way ensureNode merges them, so the same segment
// projected from frontiers at different depths has the same identity.
func projectedSegmentIdentity(segment projectedSegment) string {
	return strings.Join([]string{
		firstNonEmpty(segment.Source.Key, segment.Source.ID),
		firstNonEmpty(segment.Target.Key, segment.Target.ID),
		firstNonEmpty(segment.ActionKey, segment.ActionClass),
		normalizeAddress(segment.ValidatorAddress),
		normalizeSourceProtocol(segment.SourceProtocol),
		strings.TrimSpace(segment.TxID),
		normalizeAsset(segment.Asset),
		strings.TrimSpace(segment.AmountRaw),
		normalizeAsset(segment.SwapInAsset),
		strings.TrimSpace(segment.SwapInAmountRaw),
		normalizeAsset(segment.SwapOutAsset),
		strings.TrimSpace(segment.SwapOutAmountRaw),
	}, "|")
}

// cloneMidgardActionLegs copies the leg slices so later stitching of the
// caller's action (which rewrites legs in place) cannot alter the cached form.
func cloneMidgardActionLegs(action midgardAction) midgardAction {
	action.In = append([]midgardActionLeg(nil), action.In...)
	action.Out = append([]midgardActionLeg(nil), action.Out...)
	return action
}

// reprojectMidgardAction projects an action that is already in the graph
// through another frontier. Diagnostics counted by the first projection are
// not counted again.
func (b *graphBuilder) reprojectMidgardAction(action midgardAction, baseDepth int) ([]projectedSegment, []string) {
	feeActionDrop := b.feeActionDrop
	segments, _, warnings := b.projectMidgardAction(action, baseDepth)
	b.feeActionDrop = feeActionDrop
	return segments, warnings
}
