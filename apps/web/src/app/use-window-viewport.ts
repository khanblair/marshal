import { createRenderEffect, createSignal, onCleanup } from "solid-js";
import { M } from "~/mock";

/**
 * Keeps the store's viewport equal to the window, so the app always fills the window and
 * its size classes (phone, tablet, desktop) follow it. The store's own width-only resize
 * listener stays off because `M._framed` is set; this hook updates width and height together.
 */
export function useWindowViewport(): void {
  const [width, setWidth] = createSignal(window.innerWidth);
  const [height, setHeight] = createSignal(window.innerHeight);

  const onResize = () => {
    setWidth(window.innerWidth);
    setHeight(window.innerHeight);
  };
  window.addEventListener("resize", onResize);
  onCleanup(() => window.removeEventListener("resize", onResize));

  createRenderEffect(() => {
    M._framed = true;
    M._scale = 1;
    M.setViewport(width(), height());
  });
}
