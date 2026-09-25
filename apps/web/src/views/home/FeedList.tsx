import { FeedItem } from "@marshal/ui";
import { For } from "solid-js";
import { type FeedItem as FeedEntry, M } from "~/mock";
import { feedIcon, feedProjectName, openFeedItem } from "./feed-row";

function FeedRow(props: { entry: FeedEntry }) {
  const icon = () => feedIcon(props.entry);
  return (
    <FeedItem
      icon={icon().name}
      iconColor={icon().color}
      project={feedProjectName(props.entry)}
      when={M.rel(props.entry.ts)}
      whenTitle={M.full(props.entry.ts)}
      onClick={() => openFeedItem(props.entry)}
    >
      {props.entry.text}
    </FeedItem>
  );
}

/** Activity entries as a live list, on Home and on the Recent activity page. */
export function FeedList(props: { entries: readonly FeedEntry[] }) {
  return (
    <ol aria-live="polite" class="m-0 p-0 list-none">
      <For each={props.entries}>{(entry) => <FeedRow entry={entry} />}</For>
    </ol>
  );
}
