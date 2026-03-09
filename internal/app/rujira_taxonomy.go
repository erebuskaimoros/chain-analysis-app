package app

import "strings"

type actionDescriptor struct {
	ActionClass      string
	ActionKey        string
	ActionLabel      string
	ActionDomain     string
	ContractType     string
	ContractProtocol string
}

type contractCallDescriptor struct {
	ContractType string
	Contract     string
	Call         string
	Key          string
	Label        string
	Domain       string
	Access       string
	ActionClass  string
	FundsMove    bool
	UserFacing   bool
}

type contractFamilyDescriptor struct {
	Protocol string
	Domain   string
	Access   string
}

var contractFamilyDescriptors = map[string]contractFamilyDescriptor{
	"calc-manager":          {Protocol: "CALC Manager", Domain: "automation", Access: "admin-control"},
	"calc-strategy":         {Protocol: "CALC Strategy", Domain: "automation", Access: "internal-pipeline"},
	"rujira-account":        {Protocol: "Rujira Account", Domain: "utility", Access: "admin-control"},
	"rujira-bow":            {Protocol: "Rujira BOW", Domain: "liquidity", Access: "public-user"},
	"rujira-brune":          {Protocol: "Rujira Brune", Domain: "ops", Access: "role-gated"},
	"rujira-demo":           {Protocol: "Rujira Demo", Domain: "utility", Access: "admin-control"},
	"rujira-fin":            {Protocol: "Rujira FIN", Domain: "trade", Access: "public-user"},
	"rujira-ghost-credit":   {Protocol: "Rujira Ghost Credit", Domain: "credit", Access: "public-user"},
	"rujira-ghost-mint":     {Protocol: "Rujira Ghost Mint", Domain: "credit", Access: "role-gated"},
	"rujira-ghost-vault":    {Protocol: "Rujira Ghost Vault", Domain: "credit", Access: "public-user"},
	"rujira-merge":          {Protocol: "Rujira Merge", Domain: "redemption", Access: "public-user"},
	"rujira-mint":           {Protocol: "Rujira Mint", Domain: "utility", Access: "admin-control"},
	"rujira-pilot":          {Protocol: "Rujira Pilot", Domain: "auction", Access: "public-user"},
	"rujira-revenue":        {Protocol: "Rujira Revenue", Domain: "revenue", Access: "role-gated"},
	"rujira-staking":        {Protocol: "Rujira Staking", Domain: "staking", Access: "public-user"},
	"rujira-template":       {Protocol: "Rujira Template", Domain: "utility", Access: "admin-control"},
	"rujira-thorchain-swap": {Protocol: "Rujira THORChain Swap", Domain: "routing", Access: "role-gated"},
}

var contractCallDescriptors = map[string]contractCallDescriptor{
	"wasm-calc-manager/strategy.create":    {ContractType: "wasm-calc-manager/strategy.create", Contract: "CALC Manager", Call: "strategy.create", Key: "calc.manager.strategy.create", Label: "CALC Manager strategy.create", Domain: "automation", Access: "admin-control", ActionClass: "liquidity", UserFacing: true},
	"wasm-calc-manager/strategy.update":    {ContractType: "wasm-calc-manager/strategy.update", Contract: "CALC Manager", Call: "strategy.update", Key: "calc.manager.strategy.update", Label: "CALC Manager strategy.update", Domain: "automation", Access: "internal-pipeline", ActionClass: "liquidity"},
	"wasm-calc-strategy/execute":           {ContractType: "wasm-calc-strategy/execute", Contract: "CALC Strategy", Call: "execute", Key: "calc.strategy.execute", Label: "CALC Strategy execute", Domain: "automation", Access: "internal-pipeline", ActionClass: "liquidity"},
	"wasm-calc-strategy/init":              {ContractType: "wasm-calc-strategy/init", Contract: "CALC Strategy", Call: "init", Key: "calc.strategy.init", Label: "CALC Strategy init", Domain: "automation", Access: "admin-control", ActionClass: "transfers"},
	"wasm-calc-strategy/process":           {ContractType: "wasm-calc-strategy/process", Contract: "CALC Strategy", Call: "process", Key: "calc.strategy.process", Label: "CALC Strategy process", Domain: "automation", Access: "internal-pipeline", ActionClass: "liquidity"},
	"wasm-calc-strategy/process.reply":     {ContractType: "wasm-calc-strategy/process.reply", Contract: "CALC Strategy", Call: "process.reply", Key: "calc.strategy.process.reply", Label: "CALC Strategy process.reply", Domain: "automation", Access: "internal-pipeline", ActionClass: "liquidity"},
	"wasm-calc-strategy/update":            {ContractType: "wasm-calc-strategy/update", Contract: "CALC Strategy", Call: "update", Key: "calc.strategy.update", Label: "CALC Strategy update", Domain: "automation", Access: "internal-pipeline", ActionClass: "liquidity"},
	"wasm-rujira-bow/deposit":              {ContractType: "wasm-rujira-bow/deposit", Contract: "Rujira BOW", Call: "deposit", Key: "rujira.bow.deposit", Label: "Rujira BOW deposit", Domain: "liquidity", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-bow/swap":                 {ContractType: "wasm-rujira-bow/swap", Contract: "Rujira BOW", Call: "swap", Key: "rujira.bow.swap", Label: "Rujira BOW swap", Domain: "liquidity", Access: "public-user", ActionClass: "swaps", FundsMove: true, UserFacing: true},
	"wasm-rujira-bow/withdraw":             {ContractType: "wasm-rujira-bow/withdraw", Contract: "Rujira BOW", Call: "withdraw", Key: "rujira.bow.withdraw", Label: "Rujira BOW withdraw", Domain: "liquidity", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-fin/market-maker.fee":     {ContractType: "wasm-rujira-fin/market-maker.fee", Contract: "Rujira FIN", Call: "market-maker.fee", Key: "rujira.fin.market_maker.fee", Label: "Rujira FIN market-maker.fee", Domain: "trade", Access: "internal-pipeline", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-fin/order.create":         {ContractType: "wasm-rujira-fin/order.create", Contract: "Rujira FIN", Call: "order.create", Key: "rujira.fin.order.create", Label: "Rujira FIN order.create", Domain: "trade", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-fin/trade":                {ContractType: "wasm-rujira-fin/trade", Contract: "Rujira FIN", Call: "trade", Key: "rujira.fin.trade", Label: "Rujira FIN trade", Domain: "trade", Access: "internal-pipeline", ActionClass: "swaps", FundsMove: true},
	"wasm-rujira-ghost-credit/account":     {ContractType: "wasm-rujira-ghost-credit/account", Contract: "Rujira Ghost Credit", Call: "account", Key: "rujira.ghost.credit.account", Label: "Rujira Ghost Credit account", Domain: "credit", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-ghost-credit/create":      {ContractType: "wasm-rujira-ghost-credit/create", Contract: "Rujira Ghost Credit", Call: "create", Key: "rujira.ghost.credit.create", Label: "Rujira Ghost Credit create", Domain: "credit", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-ghost-credit/liquidate":   {ContractType: "wasm-rujira-ghost-credit/liquidate", Contract: "Rujira Ghost Credit", Call: "liquidate", Key: "rujira.ghost.credit.liquidate", Label: "Rujira Ghost Credit liquidate", Domain: "credit", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-ghost-mint/borrow":        {ContractType: "wasm-rujira-ghost-mint/borrow", Contract: "Rujira Ghost Mint", Call: "borrow", Key: "rujira.ghost.mint.borrow", Label: "Rujira Ghost Mint borrow", Domain: "credit", Access: "role-gated", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-ghost-mint/repay":         {ContractType: "wasm-rujira-ghost-mint/repay", Contract: "Rujira Ghost Mint", Call: "repay", Key: "rujira.ghost.mint.repay", Label: "Rujira Ghost Mint repay", Domain: "credit", Access: "role-gated", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-ghost-vault/borrow":       {ContractType: "wasm-rujira-ghost-vault/borrow", Contract: "Rujira Ghost Vault", Call: "borrow", Key: "rujira.ghost.vault.borrow", Label: "Rujira Ghost Vault borrow", Domain: "credit", Access: "role-gated", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-ghost-vault/repay":        {ContractType: "wasm-rujira-ghost-vault/repay", Contract: "Rujira Ghost Vault", Call: "repay", Key: "rujira.ghost.vault.repay", Label: "Rujira Ghost Vault repay", Domain: "credit", Access: "role-gated", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-merge/deposit":            {ContractType: "wasm-rujira-merge/deposit", Contract: "Rujira Merge", Call: "deposit", Key: "rujira.merge.deposit", Label: "Rujira Merge deposit", Domain: "redemption", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-merge/withdraw":           {ContractType: "wasm-rujira-merge/withdraw", Contract: "Rujira Merge", Call: "withdraw", Key: "rujira.merge.withdraw", Label: "Rujira Merge withdraw", Domain: "redemption", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-pilot/order.create":       {ContractType: "wasm-rujira-pilot/order.create", Contract: "Rujira Pilot", Call: "order.create", Key: "rujira.pilot.order.create", Label: "Rujira Pilot order.create", Domain: "auction", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-pilot/swap":               {ContractType: "wasm-rujira-pilot/swap", Contract: "Rujira Pilot", Call: "swap", Key: "rujira.pilot.swap", Label: "Rujira Pilot swap", Domain: "auction", Access: "public-user", ActionClass: "swaps", FundsMove: true, UserFacing: true},
	"wasm-rujira-revenue/run":              {ContractType: "wasm-rujira-revenue/run", Contract: "Rujira Revenue", Call: "run", Key: "rujira.revenue.run", Label: "Rujira Revenue run", Domain: "revenue", Access: "role-gated", ActionClass: "liquidity", FundsMove: true},
	"wasm-rujira-staking/account.bond":     {ContractType: "wasm-rujira-staking/account.bond", Contract: "Rujira Staking", Call: "account.bond", Key: "rujira.staking.account.bond", Label: "Rujira Staking account.bond", Domain: "staking", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-staking/account.claim":    {ContractType: "wasm-rujira-staking/account.claim", Contract: "Rujira Staking", Call: "account.claim", Key: "rujira.staking.account.claim", Label: "Rujira Staking account.claim", Domain: "staking", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-staking/account.withdraw": {ContractType: "wasm-rujira-staking/account.withdraw", Contract: "Rujira Staking", Call: "account.withdraw", Key: "rujira.staking.account.withdraw", Label: "Rujira Staking account.withdraw", Domain: "staking", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-staking/liquid.bond":      {ContractType: "wasm-rujira-staking/liquid.bond", Contract: "Rujira Staking", Call: "liquid.bond", Key: "rujira.staking.liquid.bond", Label: "Rujira Staking liquid.bond", Domain: "staking", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-staking/liquid.unbond":    {ContractType: "wasm-rujira-staking/liquid.unbond", Contract: "Rujira Staking", Call: "liquid.unbond", Key: "rujira.staking.liquid.unbond", Label: "Rujira Staking liquid.unbond", Domain: "staking", Access: "public-user", ActionClass: "liquidity", FundsMove: true, UserFacing: true},
	"wasm-rujira-thorchain-swap/swap":      {ContractType: "wasm-rujira-thorchain-swap/swap", Contract: "Rujira THORChain Swap", Call: "swap", Key: "rujira.thorchain_swap.swap", Label: "Rujira THORChain Swap swap", Domain: "routing", Access: "role-gated", ActionClass: "swaps", FundsMove: true},
}

func describeEventType(eventType string) actionDescriptor {
	key := normalizeActionKey(eventType)
	return actionDescriptor{
		ActionClass:  eventActionClass(eventType),
		ActionKey:    "event." + key,
		ActionLabel:  humanizeActionKey(key),
		ActionDomain: eventActionClass(eventType),
	}
}

func describeMidgardAction(action midgardAction) actionDescriptor {
	actionType := strings.ToLower(strings.TrimSpace(action.Type))
	if actionType == "contract" {
		return describeMidgardContractAction(action)
	}

	key := normalizeActionKey(action.Type)
	class := describeMidgardActionClass(action)
	return actionDescriptor{
		ActionClass:  class,
		ActionKey:    "midgard." + key,
		ActionLabel:  humanizeActionKey(key),
		ActionDomain: class,
	}
}

func describeMidgardContractAction(action midgardAction) actionDescriptor {
	contractType := ""
	if action.Metadata.Contract != nil {
		contractType = action.Metadata.Contract.ContractType
	}
	call := lookupContractCallDescriptor(contractType)
	class := call.ActionClass
	if class == "" {
		class = inferContractActionClass(action)
	}
	if class == "" {
		class = "liquidity"
	}
	key := call.Key
	if key == "" {
		key = "contract." + normalizeActionKey(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(contractType)), "wasm-"))
	}
	label := call.Label
	if label == "" {
		label = humanizeContractType(contractType)
	}
	return actionDescriptor{
		ActionClass:      class,
		ActionKey:        key,
		ActionLabel:      label,
		ActionDomain:     firstNonEmpty(call.Domain, class),
		ContractType:     strings.TrimSpace(contractType),
		ContractProtocol: call.Contract,
	}
}

func lookupContractCallDescriptor(contractType string) contractCallDescriptor {
	normalized := strings.ToLower(strings.TrimSpace(contractType))
	if desc, ok := contractCallDescriptors[normalized]; ok {
		return desc
	}

	family, call := splitContractType(normalized)
	familyMeta := contractFamilyDescriptors[family]
	protocol := familyMeta.Protocol
	if protocol == "" {
		protocol = humanizeActionKey(strings.ReplaceAll(family, "-", " "))
	}
	actionClass := ""
	if normalized != "" {
		actionClass = inferContractActionClassFromType(normalized)
	}
	desc := contractCallDescriptor{
		ContractType: normalized,
		Contract:     protocol,
		Call:         call,
		Key:          buildContractActionKey(family, call),
		Label:        buildContractActionLabel(protocol, call),
		Domain:       familyMeta.Domain,
		Access:       familyMeta.Access,
		ActionClass:  actionClass,
	}
	return desc
}

func splitContractType(contractType string) (string, string) {
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(contractType)), "wasm-")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	family := strings.TrimSpace(parts[0])
	call := ""
	if len(parts) == 2 {
		call = strings.TrimSpace(parts[1])
	}
	return family, call
}

func buildContractActionKey(family, call string) string {
	family = strings.ReplaceAll(strings.TrimSpace(family), "-", ".")
	call = normalizeActionKey(call)
	switch {
	case family == "" && call == "":
		return "contract.unknown"
	case family == "":
		return "contract." + call
	case call == "":
		return family
	default:
		return family + "." + call
	}
}

func buildContractActionLabel(protocol, call string) string {
	protocol = strings.TrimSpace(protocol)
	call = strings.TrimSpace(call)
	switch {
	case protocol == "" && call == "":
		return "Contract"
	case protocol == "":
		return humanizeActionKey(call)
	case call == "":
		return protocol
	default:
		return protocol + " " + call
	}
}

func normalizeActionKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", ".", "/", ".", "_", ".", "-", ".")
	value = replacer.Replace(value)
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", ".")
	}
	return strings.Trim(value, ".")
}

func humanizeActionKey(value string) string {
	normalized := normalizeActionKey(value)
	parts := strings.FieldsFunc(normalized, func(r rune) bool { return r == '.' })
	if len(parts) == 0 {
		return "Unknown"
	}
	for i, part := range parts {
		switch part {
		case "api":
			parts[i] = "API"
		case "bow":
			parts[i] = "BOW"
		case "calc":
			parts[i] = "CALC"
		case "fin":
			parts[i] = "FIN"
		case "thorchain":
			parts[i] = "THORChain"
		default:
			if part == "" {
				continue
			}
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func humanizeContractType(contractType string) string {
	family, call := splitContractType(contractType)
	familyMeta := contractFamilyDescriptors[family]
	return buildContractActionLabel(firstNonEmpty(familyMeta.Protocol, humanizeActionKey(family)), call)
}

func describeMidgardActionClass(action midgardAction) string {
	t := strings.ToLower(strings.TrimSpace(action.Type))
	switch {
	case strings.Contains(t, "bond") || t == "leave" || t == "slash" || t == "rewards":
		return "bonds"
	case strings.Contains(t, "swap") || t == "trade":
		return "swaps"
	case strings.Contains(t, "liquidity") || strings.Contains(t, "rune_pool") || strings.Contains(t, "loan") || t == "refund":
		return "liquidity"
	case strings.Contains(t, "contract"):
		return inferContractActionClass(action)
	default:
		return "transfers"
	}
}

func inferContractActionClassFromType(contractType string) string {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	switch {
	case strings.Contains(contractType, "swap"), strings.Contains(contractType, "trade"), strings.Contains(contractType, "thorchain-swap"):
		return "swaps"
	case strings.Contains(contractType, "bond"), strings.Contains(contractType, "rebond"), strings.Contains(contractType, "unbond"), strings.Contains(contractType, "leave"), strings.Contains(contractType, "slash"), strings.Contains(contractType, "reward"):
		return "bonds"
	case strings.Contains(contractType, "liquidity"), strings.Contains(contractType, "pool"), strings.Contains(contractType, "vault"), strings.Contains(contractType, "deposit"), strings.Contains(contractType, "withdraw"), strings.Contains(contractType, "borrow"), strings.Contains(contractType, "repay"), strings.Contains(contractType, "strategy"), strings.Contains(contractType, "manager"), strings.Contains(contractType, "order"):
		return "liquidity"
	default:
		return "liquidity"
	}
}
