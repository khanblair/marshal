import { render } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { ErrorState } from "./base/ErrorState";
import { NotConnected } from "./base/NotConnected";
import { Skeleton } from "./base/Skeleton";
import { SkeletonCard } from "./base/SkeletonCard";
import { SkeletonGroup } from "./base/SkeletonGroup";
import { SkeletonLines } from "./base/SkeletonLines";
import { SkeletonRow } from "./base/SkeletonRow";
import { ConnectionLost } from "./layout/ConnectionLost";
import { OfflineBanner } from "./layout/OfflineBanner";
import { SignIn } from "./layout/SignIn";

/*
 * The connection states share one set of copy and class rules (ui-rules 5.6 and 6). Each
 * component's own test covers its behavior; this one checks the rules on all of them at once.
 */

const noop = () => {};

const states: [string, () => JSX.Element][] = [
  [
    "ConnectionLost",
    () => <ConnectionLost onRetry={noop} retryInSeconds={4} details="ERR_REFUSED" />,
  ],
  ["ConnectionLost on a phone", () => <ConnectionLost onRetry={noop} phone />],
  ["OfflineBanner", () => <OfflineBanner retryInSeconds={4} />],
  ["SignIn", () => <SignIn onSubmit={noop} error="That token is not right." />],
  [
    "NotConnected",
    () => (
      <NotConnected service="GitHub" reason="Connect it to open pull requests." onConnect={noop} />
    ),
  ],
  [
    "ErrorState",
    () => <ErrorState message="Couldn't load the activity." details="ERR_TIMEOUT" onRetry={noop} />,
  ],
  [
    "the skeletons",
    () => (
      <SkeletonGroup>
        <Skeleton />
        <SkeletonLines />
        <SkeletonRow />
        <SkeletonCard />
      </SkeletonGroup>
    ),
  ],
];

/** Every class attribute in the tree, split into single classes. */
function classesIn(root: HTMLElement): string[] {
  return Array.from(root.querySelectorAll("[class]"))
    .concat(root)
    .flatMap((element) => (element.getAttribute("class") ?? "").split(/\s+/));
}

describe.each(states)("%s", (_name, view) => {
  it("uses sentence case: no uppercase class and no word of three or more capitals", () => {
    const { container } = render(view);
    expect(
      classesIn(container).filter((name) => /(^|:)(uppercase|capitalize)$/.test(name)),
    ).toEqual([]);
    expect(container.textContent).not.toMatch(/\b[A-Z]{3,}\b/);
  });

  it("joins nothing with dots or dashes, and shows no stack trace", () => {
    const { container } = render(view);
    expect(container.textContent).not.toMatch(/ [·•—–-] /);
    expect(container.textContent).not.toMatch(/\bat \S+ \(|Error:|\.tsx?:\d+/);
  });

  it("uses tokens only: no raw color in any class", () => {
    const { container } = render(view);
    expect(classesIn(container).filter((name) => name.includes("#"))).toEqual([]);
  });
});
