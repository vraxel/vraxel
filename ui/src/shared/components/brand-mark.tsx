/**
 * Vraxel's mark: an isometric voxel with the top-right sub-cube scooped out.
 * The cube says "voxel", which is what the name is built from; the notch says
 * the thing is made of separable units, which is what the platform does.
 *
 * Drawn in `currentColor` at three opacities rather than three hard-coded
 * hexes, so a single `text-*` class drives it and it stays correct on a brand
 * fill, on white, and while disabled.
 *
 * Geometry: a 32x32 isometric cube subdivided 2x2x2, so every vertex lands on
 * the half-unit grid and the mark stays crisp from 16px up. Paint order is
 * back-to-front -- the cavity is laid down first and the solid faces are drawn
 * over it, which is what clips the floor to the notch opening.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden="true" className={className}>
      {/* cavity floor, one sub-cube down */}
      <path d="M16 16 L22.5 12.25 L29 16 L22.5 19.75 Z" fill="currentColor" opacity="0.45" />
      {/* cavity wall left behind by the removed sub-cube */}
      <path d="M16 11.5 L22.5 7.75 L22.5 12.25 L16 16 Z" fill="currentColor" />
      {/* top face, minus the removed quadrant */}
      <path
        d="M16 4 L3 11.5 L16 19 L22.5 15.25 L16 11.5 L22.5 7.75 Z"
        fill="currentColor"
        opacity="0.45"
      />
      {/* left face */}
      <path d="M3 11.5 L16 19 L16 28 L3 20.5 Z" fill="currentColor" opacity="0.7" />
      {/* right face, notched where the sub-cube was */}
      <path d="M22.5 15.25 L16 19 L16 28 L29 20.5 L29 16 L22.5 19.75 Z" fill="currentColor" />
    </svg>
  )
}
