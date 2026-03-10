package app

import "time"

type MidgardPool struct {
	Asset          string `json:"asset"`
	Status         string `json:"status"`
	AssetDepth     string `json:"assetDepth"`
	RuneDepth      string `json:"runeDepth"`
	LiquidityUnits string `json:"liquidityUnits"`
	AssetPrice     string `json:"assetPrice"`
	AssetPriceUSD  string `json:"assetPriceUSD"`
}

type RebondLink struct {
	Height         int64          `json:"height"`
	TxID           string         `json:"tx_id"`
	NodeAddress    string         `json:"node_address"`
	OldBondAddress string         `json:"old_bond_address"`
	NewBondAddress string         `json:"new_bond_address"`
	Data           map[string]any `json:"data"`
}

type Actor struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Color     string         `json:"color"`
	Notes     string         `json:"notes"`
	Addresses []ActorAddress `json:"addresses"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ActorAddress struct {
	ID                int64     `json:"id"`
	ActorID           int64     `json:"actor_id"`
	Address           string    `json:"address"`
	NormalizedAddress string    `json:"normalized_address"`
	ChainHint         string    `json:"chain_hint"`
	Label             string    `json:"label"`
	CreatedAt         time.Time `json:"created_at"`
}

type ActorAddressInput struct {
	Address   string `json:"address"`
	ChainHint string `json:"chain_hint"`
	Label     string `json:"label"`
}

type ActorUpsertRequest struct {
	Name      string              `json:"name"`
	Color     string              `json:"color"`
	Notes     string              `json:"notes"`
	Addresses []ActorAddressInput `json:"addresses"`
}

type ActorTrackerQuery struct {
	ActorIDs          []int64   `json:"actor_ids"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	MaxHops           int       `json:"max_hops"`
	FlowTypes         []string  `json:"flow_types"`
	MinUSD            float64   `json:"min_usd"`
	CollapseExternal  bool      `json:"collapse_external"`
	DisplayMode       string    `json:"display_mode"`
	RequestedAt       time.Time `json:"requested_at"`
	BlocksScanned     int64     `json:"blocks_scanned"`
	CoverageSatisfied bool      `json:"coverage_satisfied"`
}

type ActorTrackerRequest struct {
	ActorIDs         []int64  `json:"actor_ids"`
	StartTime        string   `json:"start_time"`
	EndTime          string   `json:"end_time"`
	MaxHops          int      `json:"max_hops"`
	FlowTypes        []string `json:"flow_types"`
	MinUSD           float64  `json:"min_usd"`
	CollapseExternal bool     `json:"collapse_external"`
	DisplayMode      string   `json:"display_mode"`
}

type ActorTrackerExpandRequest struct {
	ActorIDs         []int64  `json:"actor_ids"`
	Addresses        []string `json:"addresses"`
	StartTime        string   `json:"start_time"`
	EndTime          string   `json:"end_time"`
	FlowTypes        []string `json:"flow_types"`
	MinUSD           float64  `json:"min_usd"`
	CollapseExternal bool     `json:"collapse_external"`
	DisplayMode      string   `json:"display_mode"`
}

type ActorTrackerLiveHoldingsRequest struct {
	Nodes []FlowNode `json:"nodes"`
}

type ActorTrackerLiveHoldingsResponse struct {
	Nodes       []FlowNode `json:"nodes"`
	Warnings    []string   `json:"warnings"`
	RefreshedAt time.Time  `json:"refreshed_at"`
}

type FlowAssetValue struct {
	Asset         string  `json:"asset"`
	AmountRaw     string  `json:"amount_raw"`
	USDSpot       float64 `json:"usd_spot"`
	Direction     string  `json:"direction,omitempty"`
	AssetKind     string  `json:"asset_kind,omitempty"`
	TokenStandard string  `json:"token_standard,omitempty"`
	TokenAddress  string  `json:"token_address,omitempty"`
	TokenSymbol   string  `json:"token_symbol,omitempty"`
	TokenName     string  `json:"token_name,omitempty"`
	TokenDecimals int     `json:"token_decimals,omitempty"`
}

type FlowNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Label     string         `json:"label"`
	Chain     string         `json:"chain"`
	Stage     string         `json:"stage"`
	Depth     int            `json:"depth"`
	ActorIDs  []int64        `json:"actor_ids"`
	Shared    bool           `json:"shared"`
	Collapsed bool           `json:"collapsed"`
	Metrics   map[string]any `json:"metrics"`
}

type FlowEdge struct {
	ID               string           `json:"id"`
	From             string           `json:"from"`
	To               string           `json:"to"`
	ActionClass      string           `json:"action_class"`
	ActionKey        string           `json:"action_key"`
	ActionLabel      string           `json:"action_label"`
	ActionDomain     string           `json:"action_domain"`
	ValidatorAddress string           `json:"validator_address,omitempty"`
	ValidatorLabel   string           `json:"validator_label,omitempty"`
	ContractType     string           `json:"contract_type"`
	ContractProtocol string           `json:"contract_protocol"`
	Assets           []FlowAssetValue `json:"assets"`
	USDSpot          float64          `json:"usd_spot"`
	TxIDs            []string         `json:"tx_ids"`
	Heights          []int64          `json:"heights"`
	ActorIDs         []int64          `json:"actor_ids"`
	Confidence       float64          `json:"confidence"`
}

type SupportingAction struct {
	TxID             string    `json:"tx_id"`
	ActionClass      string    `json:"action_class"`
	ActionKey        string    `json:"action_key"`
	ActionLabel      string    `json:"action_label"`
	ActionDomain     string    `json:"action_domain"`
	ValidatorAddress string    `json:"validator_address,omitempty"`
	ValidatorLabel   string    `json:"validator_label,omitempty"`
	ContractType     string    `json:"contract_type"`
	ContractProtocol string    `json:"contract_protocol"`
	PrimaryAsset     string    `json:"primary_asset"`
	AssetKind        string    `json:"asset_kind,omitempty"`
	TokenStandard    string    `json:"token_standard,omitempty"`
	TokenAddress     string    `json:"token_address,omitempty"`
	TokenSymbol      string    `json:"token_symbol,omitempty"`
	TokenName        string    `json:"token_name,omitempty"`
	TokenDecimals    int       `json:"token_decimals,omitempty"`
	AmountRaw        string    `json:"amount_raw"`
	USDSpot          float64   `json:"usd_spot"`
	Height           int64     `json:"height"`
	Time             time.Time `json:"time"`
	FromNode         string    `json:"from_node"`
	ToNode           string    `json:"to_node"`
	ActorIDs         []int64   `json:"actor_ids"`
}

type ActorTrackerResponse struct {
	Query             ActorTrackerQuery  `json:"query"`
	Actors            []Actor            `json:"actors"`
	Stats             map[string]any     `json:"stats"`
	Warnings          []string           `json:"warnings"`
	Nodes             []FlowNode         `json:"nodes"`
	Edges             []FlowEdge         `json:"edges"`
	SupportingActions []SupportingAction `json:"supporting_actions"`
}

type AddressExplorerRequest struct {
	Address   string   `json:"address"`
	FlowTypes []string `json:"flow_types"`
	MinUSD    float64  `json:"min_usd"`
	Mode      string   `json:"mode"`       // "preview" or "graph"
	Direction string   `json:"direction"`  // "newest" or "oldest"
	Offset    int      `json:"offset"`     // Midgard page offset (0 = first batch)
	BatchSize int      `json:"batch_size"` // pages per batch, default 10 (500 txns)
}

type AddressExplorerSeedSummary struct {
	Chain                 string `json:"chain"`
	Address               string `json:"address"`
	Active                bool   `json:"active"`
	MidgardActionCount    int    `json:"midgard_action_count"`
	ExternalTransferCount int    `json:"external_transfer_count"`
}

type AddressExplorerResponse struct {
	Mode              string                       `json:"mode"`
	RawAddress        string                       `json:"raw_address"`
	Address           string                       `json:"address"`
	Query             AddressExplorerQuery         `json:"query"`
	Stats             map[string]any               `json:"stats"`
	Warnings          []string                     `json:"warnings"`
	Nodes             []FlowNode                   `json:"nodes"`
	Edges             []FlowEdge                   `json:"edges"`
	SupportingActions []SupportingAction           `json:"supporting_actions"`
	LoadedActions     int                          `json:"loaded_actions"`
	HasMore           bool                         `json:"has_more"`
	NextOffset        int                          `json:"next_offset"`
	TotalEstimate     int                          `json:"total_estimate"` // -1 if unknown
	DirectionRequired bool                         `json:"direction_required"`
	ActiveChains      []string                     `json:"active_chains"`
	SeedSummaries     []AddressExplorerSeedSummary `json:"seed_summaries"`
	RunLabel          string                       `json:"run_label"`
}

type AddressExplorerQuery struct {
	Address   string   `json:"address"`
	FlowTypes []string `json:"flow_types"`
	MinUSD    float64  `json:"min_usd"`
	Mode      string   `json:"mode"`
	Direction string   `json:"direction"`
	Offset    int      `json:"offset"`
	BatchSize int      `json:"batch_size"`
}

const (
	GraphRunTypeActorTracker    = "actor_tracker"
	GraphRunTypeAddressExplorer = "address_explorer"
)

type GraphRun struct {
	ID         int64               `json:"id"`
	RunType    string              `json:"run_type,omitempty"`
	Request    ActorTrackerRequest `json:"request"`
	ActorNames string              `json:"actor_names"`
	NodeCount  int                 `json:"node_count"`
	EdgeCount  int                 `json:"edge_count"`
	CreatedAt  time.Time           `json:"created_at"`
}

type AddressExplorerRun struct {
	ID        int64                  `json:"id"`
	RunType   string                 `json:"run_type,omitempty"`
	Request   AddressExplorerRequest `json:"request"`
	Summary   string                 `json:"summary"`
	NodeCount int                    `json:"node_count"`
	EdgeCount int                    `json:"edge_count"`
	CreatedAt time.Time              `json:"created_at"`
}

type FlowSegment struct {
	Source      string
	Target      string
	ActionClass string
	Asset       string
	AmountRaw   string
	USDSpot     float64
	TxID        string
	Height      int64
	Time        time.Time
	Confidence  float64
	ActorIDs    []int64
	Stage       string
}
