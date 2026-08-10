import { describe, expect, it } from "vitest"
import { toSearchParams } from "../search-params"

describe("toSearchParams", () => {
  it("returns undefined for missing params", () => {
    expect(toSearchParams(undefined)).toBeUndefined()
  })

  it("serializes strings, numbers and booleans; skips empty values", () => {
    const sp = toSearchParams({
      page: 2,
      pageSize: 20,
      search: "db-1",
      includeDeleted: false,
      phase: "",
      owner: undefined,
      rack: null,
    })!
    expect(sp.toString()).toBe("page=2&pageSize=20&search=db-1&includeDeleted=false")
  })

  it("repeats array keys and skips empty array items", () => {
    const sp = toSearchParams({ status: ["running", "", "failed"], ids: [1, 2] })!
    expect(sp.getAll("status")).toEqual(["running", "failed"])
    expect(sp.getAll("ids")).toEqual(["1", "2"])
  })
})
