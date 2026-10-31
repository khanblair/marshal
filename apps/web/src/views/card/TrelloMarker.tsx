import { IconLabel } from "@marshal/ui";
import { Show } from "solid-js";
import { linkOfCard, markerWord } from "~/data/mappers/trello";
import { type Card, M } from "~/mock";
import { platform } from "~/platform";

const FROM_TRELLO_TITLE =
  "This card came from Trello. It starts only when you start it in Marshal. Open it in Trello.";
const ON_TRELLO_TITLE = "This card is also a Trello card. Open it in Trello.";

/**
 * Says a card is a Trello card, and where: "From Trello" for a card that was first made there, "On
 * Trello" for a Marshal card that was sent to Trello later, with a link to it and the list it was on
 * when Marshal last read the board. Nothing is drawn for a card with no Trello card.
 */
export function TrelloMarker(props: { card: Card }) {
  const link = () => linkOfCard(M.S.trello?.[props.card.p]?.pending, props.card);
  const open = (event: MouseEvent, url: string): void => {
    // The desktop and phone shells have no new tabs, so the system browser opens the address.
    if (!platform().native) return;
    event.preventDefault();
    void platform().openExternal(url);
  };
  return (
    <Show when={link()}>
      {(one) => (
        <span
          data-trello-marker
          class="inline-flex min-w-0 flex-wrap items-center gap-x-3.5 gap-y-1"
        >
          <a
            href={one().url}
            target="_blank"
            rel="noopener noreferrer"
            title={one().fromTrello ? FROM_TRELLO_TITLE : ON_TRELLO_TITLE}
            class="inline-flex items-center gap-1 text-secondary underline"
            onClick={(event) => open(event, one().url)}
          >
            <IconLabel icon="trello">{markerWord(one())}</IconLabel>
          </a>
          <Show when={one().listName}>
            <span title="The Trello list this card was on when Marshal last read the board">
              List: {one().listName}
            </span>
          </Show>
        </span>
      )}
    </Show>
  );
}
