import { useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listAnnotations, listBlocklist } from "../../../lib/api";

export function useGraphMetadata() {
  const queryClient = useQueryClient();
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const blocklistQuery = useQuery({
    queryKey: ["blocklist"],
    queryFn: listBlocklist,
  });

  const metadata = useMemo(
    () => ({
      annotations: annotationsQuery.data ?? [],
      blocklist: blocklistQuery.data ?? [],
    }),
    [annotationsQuery.data, blocklistQuery.data]
  );

  async function invalidateAnnotations() {
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
  }

  async function invalidateBlocklist() {
    await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
  }

  return {
    metadata,
    invalidateAnnotations,
    invalidateBlocklist,
    annotationsQuery,
    blocklistQuery,
  };
}
