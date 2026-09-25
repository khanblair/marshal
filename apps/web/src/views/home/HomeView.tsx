import { ActivitySection } from "./ActivitySection";
import { AwakeSection } from "./AwakeSection";
import { CiSection } from "./CiSection";
import { HomeCharts } from "./HomeCharts";
import { HomeTiles } from "./HomeTiles";
import { pagePadding, twoColumns } from "./home-layout";
import { NeedsSection } from "./NeedsSection";
import { TodaySection } from "./TodaySection";

/** Two sections side by side from 900 px, the first one wider. */
function sectionPair(): string {
  return `grid items-start gap-y-8 gap-x-10 ${twoColumns() ? "grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]" : "grid-cols-1"}`;
}

/**
 * The home dashboard: the summary numbers with two charts, the cards that need
 * you, recent activity, what is coming up today, the agents awake, and CI health.
 */
export function HomeView() {
  return (
    <div class="absolute inset-0 overflow-y-auto overflow-x-hidden">
      <div class={`max-w-[1240px] mx-auto flex flex-col gap-8 ${pagePadding()}`}>
        <section data-tour="tiles" aria-label="Summary" class="flex flex-col gap-6">
          <HomeTiles />
          <HomeCharts />
        </section>
        <NeedsSection />
        <div class={sectionPair()}>
          <ActivitySection />
          <TodaySection />
        </div>
        <div class={sectionPair()}>
          <AwakeSection />
          <CiSection />
        </div>
      </div>
    </div>
  );
}
