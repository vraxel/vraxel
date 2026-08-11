import { describe, it, expect } from "vitest"
import { buildPageItems } from "../pagination-utils"

describe("buildPageItems", () => {
  it.each([
    [0, 0, []],
    [1, 1, [1]],
    [1, 7, [1, 2, 3, 4, 5, 6, 7]],
    [4, 7, [1, 2, 3, 4, 5, 6, 7]],
    [7, 7, [1, 2, 3, 4, 5, 6, 7]],
  ])("current=%i total=%i shows all (no ellipsis)", (current, total, expected) => {
    expect(buildPageItems(current, total)).toEqual(expected)
  })

  it.each([
    [1, 99, [1, 2, 3, 4, 5, 6, "ellipsis-r", 99]],
    [4, 99, [1, 2, 3, 4, 5, 6, "ellipsis-r", 99]],
    [5, 99, [1, "ellipsis-l", 3, 4, 5, 6, 7, "ellipsis-r", 99]],
    [50, 99, [1, "ellipsis-l", 48, 49, 50, 51, 52, "ellipsis-r", 99]],
    [95, 99, [1, "ellipsis-l", 93, 94, 95, 96, 97, "ellipsis-r", 99]],
    [96, 99, [1, "ellipsis-l", 94, 95, 96, 97, 98, 99]],
    [99, 99, [1, "ellipsis-l", 94, 95, 96, 97, 98, 99]],
  ])("current=%i total=99 produces %j", (current, total, expected) => {
    expect(buildPageItems(current, total)).toEqual(expected)
  })

  it("total=8 (just over threshold) keeps stable width", () => {
    expect(buildPageItems(1, 8)).toEqual([1, 2, 3, 4, 5, 6, "ellipsis-r", 8])
    expect(buildPageItems(4, 8)).toEqual([1, 2, 3, 4, 5, 6, "ellipsis-r", 8])
    expect(buildPageItems(8, 8)).toEqual([1, "ellipsis-l", 3, 4, 5, 6, 7, 8])
  })

  it("always includes both boundaries when total > threshold", () => {
    const items = buildPageItems(50, 100)
    expect(items[0]).toBe(1)
    expect(items[items.length - 1]).toBe(100)
  })

  it("never produces ellipsis adjacent to boundary with no hidden gap", () => {
    // total=8 + current=4: window touches left boundary, should NOT emit ellipsis-l
    const items = buildPageItems(4, 8)
    expect(items.indexOf("ellipsis-l")).toBe(-1)
  })
})
