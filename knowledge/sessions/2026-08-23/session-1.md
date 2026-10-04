# Session 1 - Memoless Registration Incident Analysis

> Date: 2026-08-23
> Focus: Determine whether the approximately 15,500 fee-free memoless registrations were a DoS attack

## Conclusion

The activity was deliberate DoS-style/griefing abuse, not organic memoless use,
but it did not successfully deny THORChain base-layer service. It exploited an
authz ante-handler gap to remove the native fee, generated thousands of unused
reference records, and continued until the operational halt. The reference
range never approached exhaustion and block production did not materially
degrade. Whether the actor was malicious, white-hat, or running an unauthorized
stress demonstration cannot be determined from chain data alone.

## Root Cause

- Mainnet was running v3.19.3.
- A direct `MsgDeposit` calls `DepositAnteHandler`, which deducts the native
  transaction fee.
- `MsgExec` is separately allowlisted by the THORNode ante switch and returns
  without recursively applying the wrapped message's ante handler.
- The sender used a self-addressed `MsgExec` containing a zero-RUNE
  `MsgDeposit` with a `REFERENCE:...` memo. The registration handler executed,
  but the ordinary 0.02 RUNE native fee did not.
- `MemolessTxnCost` is a different optional registration surcharge and has a
  compiled/default value of zero. The incident bypassed the nonzero ordinary
  native fee.

## Timeline and Counts

- Sender shorthand: automated sender `...fl23`.
- Height `27,516,047`, 2026-08-21 06:35:46 UTC: sequence 3 sent a three-memo
  ETH probe. Because the three inner deposits collided on one effective
  reference inside the authz execution, the last memo remained in ETH slot
  `03619`.
- Height `27,516,465`, 07:20:04 UTC: sequence 4 registered THOR.RUNE slot
  `00329` without the native fee.
- Height `27,516,479`: sequence 5 used that slot in a successful memoless
  3.00000329 RUNE-to-LTC swap. This direct `MsgDeposit` did pay 0.02 RUNE.
- Height `27,516,564`, 07:30:49 UTC: sequence 6 began the main flood.
- Sequences 6 through 15,487 were 15,482 successful single-reference envelopes,
  ending at ETH slot `19149`, height `27,522,443`, transaction index 5, at
  18:01:28 UTC.
- Including the two effective probe registrations gives at least 15,484
  fee-free slots and 309.68 RUNE in foregone native fees.
- The third `HaltMemoless=1` vote landed in the same block at transaction index
  6. Sequences 15,488 through 15,492 were then included but failed with code 99
  because memoless transactions were halted.

## Payload and Use Evidence

- The main run registered ETH memos that all swapped to BTC.
- A deterministic 1,031-record stratified sample contained 1,031 unique memos,
  388 distinct BTC destinations, pseudo-random-looking limits from 103 to 9,890,
  and no used references.
- In the sample, 986 memos used streaming suffix `/1/2`, 9 used `/1/3`, and 36
  had no streaming suffix, consistent with an automated generator being tuned.
- The proof phase and later unused flood show the sender knew how the feature
  worked. The unusual self-authz wrapper has no ordinary integration need and
  specifically bypassed the ante fee.

## Availability Impact

- Main run: 15,482 registrations across 5,880 blocks, averaging 2.63 per block
  or 0.409 per second.
- At the halt, account sequence 2,107 was the first registration still inside
  the 3,600-block TTL. Sequences 2,107 through 15,487 equal 13,381 live attacker
  slots, or 13.38% of the 99,999-slot ETH reference space.
- A 120-block stratified sample during the run attributed 12.46% of transactions
  and 2.75% of transaction bytes to the sender. The main transactions were
  approximately 400 bytes each, about 6.2 MB total, and reported 100,600 gas
  apiece.
- Mean block interval was 6.44 seconds during the run and 6.48 seconds over the
  equal post-halt window. The chain did not stall and the reference range did
  not exhaust.
- State impact outlives the TTL because reference records remain until reuse and
  registration-hash aliases are intentionally retained. Continued unpriced use
  is therefore a state-bloat risk even when slots expire.

## Assessment

- High confidence: automated, intentional abuse rather than organic traffic.
- High confidence: a fee-bypass and state/transaction spam event.
- High confidence: not a successful base-chain DoS and not a successful slot
  exhaustion attack.
- Medium confidence: the objective was DoS/griefing or an intentional stress
  demonstration. On-chain evidence cannot establish the actor's off-chain
  authorization or malicious intent.
- The defensive halt caused a real, scoped denial of memoless registration and
  use. It did not halt the base chain or ordinary memo-bearing swaps.

## Sources Consulted

- Deployed v3.19.3 `x/thorchain/ante.go`, `handler_deposit.go`,
  `handler_reference_memo.go`, keeper reference storage, and constants.
- Liquify THORNode/RPC primary chain data for reference records, exact
  transactions, block headers, account state, version, Mimir state, and the
  successful proof swap.
- THORChain developer documentation for reference lifecycle, TTL, range,
  registration cost, and halt semantics.

## Files Changed

- Shared `knowledge/projects/thornode.md`: added the durable incident synthesis.
- Shared `knowledge/log.md`: appended the incident-analysis entry.
- This session note and the repo-local session index.

## Follow-up

- Keep memoless halted until wrapped native messages cannot skip their
  message-specific ante handling or the authz surface is removed.
- Add regression coverage for self-authz `MsgExec{MsgDeposit}` fee deduction and
  recursive enforcement of the one-deposit rule.
- Treat the fee bypass as broader than memoless: any authz-wrapped native
  deposit can avoid `DepositAnteHandler`; `HaltMemoless` only contains the path
  observed here.
