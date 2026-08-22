import { useEffect, useRef, useState } from "react"

type NullableNum = number | null | undefined

const DURATION_MS = 600

/**
 * Smoothly interpolates chart series values from the previous snapshot to
 * the new one over ~600ms (cubic ease-out). Null gaps are preserved: a
 * bucket that is null in the target stays null throughout the animation.
 *
 * transitionKey is what triggers a new animation -- typically a stringified
 * version of the data identity (e.g. the query window or a counter).
 */
export function useAnimatedSeries(target: NullableNum[], transitionKey: string): NullableNum[] {
  const [series, setSeries] = useState(target)
  const current = useRef(target)

  useEffect(() => {
    let frame = 0
    const publish = (next: NullableNum[]) => {
      current.current = next
      setSeries(next)
    }

    const reduceMotion =
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches

    if (reduceMotion || target.length === 0 || current.current.length === 0) {
      publish(target)
      return
    }

    const prev = fit(current.current, target.length)
    if (same(prev, target)) {
      publish(target)
      return
    }

    const t0 = performance.now()
    const animate = (now: number) => {
      const elapsed = Math.min(Math.max((now - t0) / DURATION_MS, 0), 1)
      const eased = 1 - (1 - elapsed) ** 3
      publish(interpolate(prev, target, eased))
      if (elapsed < 1) frame = requestAnimationFrame(animate)
    }
    frame = requestAnimationFrame(animate)
    return () => cancelAnimationFrame(frame)
  }, [transitionKey])

  return series
}

function interpolate(from: NullableNum[], to: NullableNum[], t: number): NullableNum[] {
  const src = fit(from, to.length)
  return to.map((v, i) => {
    if (typeof v !== "number") return v
    const s = src[i]
    if (typeof s !== "number") return v
    return s + (v - s) * t
  })
}

function fit(series: NullableNum[], length: number): NullableNum[] {
  if (length === 0) return []
  if (series.length === 0) return new Array(length).fill(null) as NullableNum[]
  if (series.length === length) return series.slice()
  if (series.length === 1 || length === 1) return new Array(length).fill(series.at(-1))

  return Array.from({ length }, (_, i) => {
    const pos = (i / (length - 1)) * (series.length - 1)
    const lo = Math.floor(pos)
    const hi = Math.min(Math.ceil(pos), series.length - 1)
    const a = series[lo]
    const b = series[hi]
    if (typeof a !== "number" || typeof b !== "number") return b
    return a + (b - a) * (pos - lo)
  })
}

function same(a: NullableNum[], b: NullableNum[]): boolean {
  if (a.length !== b.length) return false
  return a.every((v, i) => v === b[i])
}
