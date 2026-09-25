import { ShowMoreFooter } from "@marshal/ui";
import { M } from "~/mock";
import { FeedList } from "./FeedList";
import { viewAllActivity } from "./home-actions";
import { createShowAll } from "./use-show-all";

/** Home's "Recent activity": the latest feed entries, five until "Show all". */
export function ActivitySection() {
  const rows = createShowAll(() => M.S.feed);
  return (
    <section data-tour="activity" aria-labelledby="h-act" class="flex flex-col">
      <h2 id="h-act" class="m-0 mb-2 text-subtitle leading-5.5 font-semibold">
        Recent activity
      </h2>
      <FeedList entries={rows.visible()} />
      <ShowMoreFooter
        total={M.S.feed.length}
        expanded={rows.expanded()}
        onToggle={rows.toggle}
        viewAllLabel="View all activity"
        onViewAll={viewAllActivity}
      />
    </section>
  );
}
