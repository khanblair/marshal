import { NotConnected } from "@marshal/ui";
import { openIntegrations } from "./home-actions";

/**
 * What Home's CI health and its "all" page show while no project has CI data: the daemon reads CI
 * from GitHub, which is not connected yet. It draws no rows and no state, only what is missing.
 */
export function CiNotConnected() {
  return (
    <NotConnected
      service="GitHub"
      reason="CI runs appear here once GitHub is connected."
      onConnect={openIntegrations}
    />
  );
}
