import { toLocalInputValue } from "../../lib/format";
import {
  GRAPH_FILTER_TXN_TYPES,
  formatGraphFilterNumber,
  type GraphFilterState,
  type GraphTxnBucket,
} from "../../lib/graph";

interface GraphFilterPopoverProps {
  filterState: GraphFilterState;
  onToggleTxnType: (bucket: GraphTxnBucket, checked: boolean) => void;
  onToggleChain: (chain: string, checked: boolean) => void;
  labelCategories?: string[];
  onToggleLabelCategory?: (category: string, visible: boolean) => void;
  onStartTimeChange: (value: string) => void;
  onEndTimeChange: (value: string) => void;
  onMinUSDChange: (value: string) => void;
  onMaxUSDChange: (value: string) => void;
  onReset: () => void;
}

export function GraphFilterPopover({
  filterState,
  onToggleTxnType,
  onToggleChain,
  labelCategories = [],
  onToggleLabelCategory,
  onStartTimeChange,
  onEndTimeChange,
  onMinUSDChange,
  onMaxUSDChange,
  onReset,
}: GraphFilterPopoverProps) {
  const startValue = filterState.startTime ? toLocalInputValue(new Date(filterState.startTime)) : "";
  const endValue = filterState.endTime ? toLocalInputValue(new Date(filterState.endTime)) : "";
  const minTxnUSDValue = formatGraphFilterNumber(filterState.minTxnUSD);
  const maxTxnUSDValue = formatGraphFilterNumber(filterState.maxTxnUSD);

  return (
    <>
      <div className="graph-filter-head">
        <strong>Filters</strong>
        <button type="button" className="button secondary slim" onClick={onReset}>
          Reset
        </button>
      </div>

      <div className="graph-filter-section">
        <div className="graph-filter-section-title">Txn Types</div>
        <div className="graph-filter-options">
          {GRAPH_FILTER_TXN_TYPES.map((item) => (
            <label key={item.key} className="graph-filter-option">
              <input
                type="checkbox"
                checked={filterState.txnTypes[item.key] !== false}
                onChange={(event) => onToggleTxnType(item.key, event.target.checked)}
              />
              <span>{item.label}</span>
            </label>
          ))}
        </div>
      </div>

      <div className="graph-filter-section">
        <div className="graph-filter-section-title">Chains Shown</div>
        <div className="graph-filter-options graph-filter-options-scroll">
          {filterState.availableChains.length ? (
            filterState.availableChains.map((chain) => (
              <label key={chain} className="graph-filter-option">
                <input
                  type="checkbox"
                  checked={filterState.selectedChains.includes(chain)}
                  onChange={(event) => onToggleChain(chain, event.target.checked)}
                />
                <span>{chain}</span>
              </label>
            ))
          ) : (
            <div className="graph-filter-empty">No chains loaded.</div>
          )}
        </div>
      </div>

      {labelCategories.length && onToggleLabelCategory ? (
        <div className="graph-filter-section">
          <div className="graph-filter-section-title">Labeled Entities Shown</div>
          <div className="graph-filter-options graph-filter-options-scroll">
            {labelCategories.map((category) => (
              <label key={category} className="graph-filter-option">
                <input
                  type="checkbox"
                  checked={!(filterState.hiddenLabelCategories ?? []).includes(category)}
                  onChange={(event) => onToggleLabelCategory(category, event.target.checked)}
                />
                <span className={`label-category-badge label-category-${category}`}>{category.replace(/_/g, " ")}</span>
              </label>
            ))}
          </div>
        </div>
      ) : null}

      <div className="graph-filter-section">
        <div className="graph-filter-section-title">Time Window</div>
        <label className="graph-filter-field">
          <span>Start</span>
          <input type="datetime-local" value={startValue} onChange={(event) => onStartTimeChange(event.target.value)} />
        </label>
        <label className="graph-filter-field">
          <span>End</span>
          <input type="datetime-local" value={endValue} onChange={(event) => onEndTimeChange(event.target.value)} />
        </label>
      </div>

      <div className="graph-filter-section">
        <div className="graph-filter-section-title">Txn Value ($)</div>
        <label className="graph-filter-field">
          <span>Min</span>
          <input type="number" min="0" step="any" value={minTxnUSDValue} onChange={(event) => onMinUSDChange(event.target.value)} />
        </label>
        <label className="graph-filter-field">
          <span>Max</span>
          <input type="number" min="0" step="any" value={maxTxnUSDValue} onChange={(event) => onMaxUSDChange(event.target.value)} />
        </label>
      </div>
    </>
  );
}
