/** Key for the line styles, pinned to the left edge while the grid scrolls sideways. */
export function TimelineLegend() {
  return (
    <div class="sticky left-0 flex flex-wrap gap-x-5 gap-y-2 py-3 px-4 text-caption leading-4 text-secondary max-w-[100vw]">
      <span class="inline-flex items-center gap-1.5">
        <span class="w-4.5 border-t-[1.5px] border-secondary" />
        Depends on
      </span>
      <span class="inline-flex items-center gap-1.5 text-status-danger-text">
        <span class="w-4.5 border-t-[1.5px] border-status-danger-solid [border-top-style:dashed]" />
        Broken dependency
      </span>
      <span>Drag a bar to change its planned dates.</span>
    </div>
  );
}
