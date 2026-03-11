import { useState } from "react";
import {
  cloneGraphFilterState,
  createGraphFilterState,
  graphFiltersAreActive,
  resetGraphFilters,
  setGraphFilterDateValue,
  setGraphFilterNumberValue,
  syncGraphFilterStateWithResponse,
  type GraphFilterState,
  type GraphTxnBucket,
} from "../../../lib/graph";
import type { ActorGraphResponse, AddressExplorerResponse } from "../../../lib/types";

type FilterableGraph = Pick<ActorGraphResponse | AddressExplorerResponse, "nodes" | "edges">;

export function useGraphFilterState() {
  const [graphFilters, setGraphFilters] = useState(createGraphFilterState);
  const filtersActive = graphFiltersAreActive(graphFilters);

  function clearFilterState() {
    setGraphFilters(createGraphFilterState());
  }

  function syncWithGraph(graph: FilterableGraph, reset = false) {
    setGraphFilters((current) => {
      const next = reset ? createGraphFilterState() : cloneGraphFilterState(current);
      syncGraphFilterStateWithResponse(next, graph, { reset });
      return next;
    });
  }

  function toggleTxnType(bucket: GraphTxnBucket, checked: boolean) {
    setGraphFilters((current) => ({
      ...current,
      txnTypes: {
        ...current.txnTypes,
        [bucket]: checked,
      },
    }));
  }

  function toggleChain(chain: string, checked: boolean) {
    setGraphFilters((current) => ({
      ...current,
      selectedChains: checked
        ? [...new Set([...current.selectedChains, chain])].sort()
        : current.selectedChains.filter((item) => item !== chain),
    }));
  }

  function updateDate(field: "startTime" | "endTime", value: string) {
    setGraphFilters((current) => {
      const next = cloneGraphFilterState(current);
      setGraphFilterDateValue(next, field, value);
      return next;
    });
  }

  function updateNumber(field: "minTxnUSD" | "maxTxnUSD", value: string) {
    setGraphFilters((current) => {
      const next = cloneGraphFilterState(current);
      setGraphFilterNumberValue(next, field, value);
      return next;
    });
  }

  function resetAllFilters() {
    setGraphFilters((current) => {
      const next: GraphFilterState = cloneGraphFilterState(current);
      resetGraphFilters(next);
      return next;
    });
  }

  function toggleOpen() {
    setGraphFilters((current) => ({ ...current, isOpen: !current.isOpen }));
  }

  function close() {
    setGraphFilters((current) => ({ ...current, isOpen: false }));
  }

  return {
    graphFilters,
    setGraphFilters,
    filtersActive,
    clearFilterState,
    syncWithGraph,
    toggleTxnType,
    toggleChain,
    updateDate,
    updateNumber,
    resetAllFilters,
    toggleOpen,
    close,
  };
}
