import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { addCaseItem, listCases } from "../../lib/api";

// PinToCase adds a saved trace to an investigation case.
export function PinToCase({ runID }: { runID: number }) {
  const queryClient = useQueryClient();
  const casesQuery = useQuery({ queryKey: ["cases"], queryFn: listCases });
  const [caseID, setCaseID] = useState("");
  const [status, setStatus] = useState("");
  const cases = casesQuery.data ?? [];
  if (!cases.length) {
    return <span className="form-message">Create a case on the Cases page to pin this trace.</span>;
  }
  const selected = caseID || String(cases[0].id);
  return (
    <div className="button-row trace-pin">
      <label className="field inline-field">
        <span>Case</span>
        <select value={selected} onChange={(event) => setCaseID(event.target.value)}>
          {cases.map((item) => (
            <option key={item.id} value={item.id}>
              {item.title}
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        className="button secondary"
        onClick={async () => {
          try {
            await addCaseItem(Number(selected), "trace_run", String(runID));
            setStatus("Pinned.");
            await queryClient.invalidateQueries({ queryKey: ["cases"] });
          } catch (error) {
            setStatus(error instanceof Error ? error.message : "Could not pin the trace.");
          }
        }}
      >
        Pin trace to case
      </button>
      {status ? <span className="form-message">{status}</span> : null}
    </div>
  );
}
