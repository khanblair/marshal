/* Grids that fit as many columns of at least N px as the width allows, named by that width.
   Complete class names, so Tailwind finds them. The 220 px grids (profile and project fields,
   sessions) have 16 px gaps; every other grid has 12 px. */
export const GRID_MIN_140 = "grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3";
export const GRID_MIN_160 = "grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-3";
export const GRID_MIN_170 = "grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-3";
export const GRID_MIN_200 = "grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3";
export const GRID_MIN_220 = "grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-4";
export const GRID_MIN_240 = "grid grid-cols-[repeat(auto-fit,minmax(240px,1fr))] gap-3";
