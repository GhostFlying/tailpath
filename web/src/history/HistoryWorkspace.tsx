import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import type { HistoryWindow } from "../api/types";
import { WorkspaceTopbar } from "../components/WorkspaceTopbar";
import { HistoryDetail } from "./HistoryDetail";
import { HistoryEdgeList } from "./HistoryEdgeList";
import { HistoryFilters } from "./HistoryFilters";
import {
  historySearchString,
  parseHistoryURL,
  updateHistoryURL,
  type HistoryURLState,
} from "./historyUrl";
import { useHistoryDetail, useHistoryIndex } from "./useHistoryData";
import "./history.css";

export default function HistoryWorkspace() {
  const { edgeId } = useParams<{ edgeId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const searchKey = searchParams.toString();
  const query = useMemo(
    () => parseHistoryURL(new URLSearchParams(searchKey)),
    [searchKey],
  );
  const isMobile = useMediaQuery("(max-width: 620px)");
  const index = useHistoryIndex(query, !(isMobile && edgeId));
  const detail = useHistoryDetail(edgeId, query.window);

  useEffect(() => {
    if (
      isMobile ||
      edgeId ||
      index.loading ||
      index.error ||
      !index.page?.edges[0]
    ) {
      return;
    }
    navigate(
      `/history/edges/${encodeURIComponent(index.page.edges[0].edgeId)}${searchKey ? `?${searchKey}` : ""}`,
      { replace: true },
    );
  }, [
    edgeId,
    index.error,
    index.loading,
    index.page,
    isMobile,
    navigate,
    searchKey,
  ]);

  function updateFilters(update: Partial<HistoryURLState>) {
    const next = updateHistoryURL(searchParams, update);
    if (("nodeId" in update || "path" in update) && edgeId) {
      navigate(`/history${next.toString() ? `?${next}` : ""}`);
      return;
    }
    setSearchParams(next);
  }

  function selectEdge(selectedEdgeID: string) {
    navigate(
      `/history/edges/${encodeURIComponent(selectedEdgeID)}${searchKey ? `?${searchKey}` : ""}`,
    );
  }

  function backToList() {
    const search = historySearchString({ ...query, cursor: "" });
    navigate(`/history${search ? `?${search}` : ""}`);
  }

  const ready = isMobile && edgeId ? !detail.loading : !index.loading;

  return (
    <main
      className={`history-shell ${edgeId ? "has-detail" : ""}`}
      data-history-ready={String(ready)}
    >
      <WorkspaceTopbar className="history-app-topbar" />
      <HistoryFilters
        state={query}
        nodes={index.nodes?.nodes ?? []}
        onChange={updateFilters}
      />
      <div className="history-workspace">
        <HistoryEdgeList
          page={index.page}
          selectedEdgeID={edgeId}
          loading={index.loading}
          error={index.error}
          onSelect={selectEdge}
          onRetry={index.retryRequest}
          onNextPage={(cursor) => {
            const next = updateHistoryURL(searchParams, { cursor });
            navigate(`/history${next.toString() ? `?${next}` : ""}`);
          }}
        />
        <HistoryDetail
          history={detail.history}
          loading={detail.loading}
          error={detail.error}
          window={query.window}
          onBack={backToList}
          onRetry={detail.retryRequest}
          onWindowChange={(window: HistoryWindow) => updateFilters({ window })}
        />
      </div>
    </main>
  );
}

function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(() =>
    typeof window === "undefined" ? false : window.matchMedia(query).matches,
  );
  useEffect(() => {
    const media = window.matchMedia(query);
    const update = () => setMatches(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [query]);
  return matches;
}
