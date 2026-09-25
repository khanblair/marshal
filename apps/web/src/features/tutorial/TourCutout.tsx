import { highlightBox, type TourRect } from "./tour-geometry";

export interface TourCutoutProps {
  rect: TourRect;
}

/**
 * The bright hole in the dim layer: a box around the target whose huge shadow dims
 * everything else. It glides to the next target.
 */
export function TourCutout(props: TourCutoutProps) {
  const box = () => highlightBox(props.rect);
  return (
    <div
      aria-hidden="true"
      class="absolute rounded-md outline-2 outline-ink outline-offset-2 shadow-[0_0_0_9999px_var(--color-tour-dim)] transition-[left,top,width,height] duration-base ease-standard"
      style={{
        left: `${box().x}px`,
        top: `${box().y}px`,
        width: `${box().w}px`,
        height: `${box().h}px`,
      }}
    />
  );
}
