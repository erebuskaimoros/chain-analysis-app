import { useQuery } from "@tanstack/react-query";
import { listActors, listAnnotations, listBlocklist } from "../../lib/api";
import { HealthPanel } from "../health/HealthPanel";

export function OverviewPage() {
  const actorsQuery = useQuery({
    queryKey: ["actors"],
    queryFn: listActors,
  });
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const blocklistQuery = useQuery({
    queryKey: ["blocklist"],
    queryFn: listBlocklist,
  });

  return (
    <div className="page-stack">
      <HealthPanel />
      <section className="panel shell-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Migration</span>
            <h2>Native Workspace</h2>
          </div>
        </div>
        <div className="overview-grid">
          <article>
            <strong>Actors</strong>
            <p>{actorsQuery.data?.length ?? 0} saved actors are available to graph and maintain from the typed UI.</p>
          </article>
          <article>
            <strong>Actor Graph</strong>
            <p>Graphs, expansions, run history, and live holdings refresh now execute from React against `/api/v1` across THOR and MAYA liquidity engines.</p>
          </article>
          <article>
            <strong>Explorer</strong>
            <p>Preview, direction choice, graph paging, and run replay now use the same typed client surface.</p>
          </article>
          <article>
            <strong>Address Metadata</strong>
            <p>
              {annotationsQuery.data?.length ?? 0} annotations and {blocklistQuery.data?.length ?? 0} blocklist entries are
              editable without the legacy workspace.
            </p>
          </article>
        </div>
      </section>
    </div>
  );
}
