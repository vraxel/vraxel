import { describe, expect, it } from "vitest"
import zhCN from "../locales/zh-CN"
import enUS from "../locales/en-US"

// The type system guarantees en-US has every zh-CN key (satisfies
// Record<MessageKey, string>) but not the reverse: excess-property
// checking does not apply through object spreads, so an en-US-only
// orphan key compiles fine. This test closes that half.
describe("locale key parity", () => {
  it("zh-CN and en-US have identical key sets", () => {
    const zh = new Set(Object.keys(zhCN))
    const en = new Set(Object.keys(enUS))
    const missingInEn = [...zh].filter((k) => !en.has(k))
    const orphanInEn = [...en].filter((k) => !zh.has(k))
    expect(missingInEn, `en-US missing keys: ${missingInEn.slice(0, 10).join(", ")}`).toEqual([])
    expect(orphanInEn, `en-US orphan keys: ${orphanInEn.slice(0, 10).join(", ")}`).toEqual([])
  })
})
