import { describe, it, expect } from "vitest"
import { ApiError, formApiErrorMessage } from "../client"

// Identity i18n stub: returns the key verbatim unless it matches one of the
// reason-only keys covered by the bug-#219 contract. The translated headline
// only matters for the asserted shape; we test that the raw backend message
// is preserved on a second line.
function t(key: string, params?: Record<string, string | number>): string {
  if (key === "api.error.conflict") return "操作冲突"
  if (key === "api.error.badRequest") return "请求参数错误"
  if (key === "api.error.internalError") return "服务器内部错误"
  if (key === "api.error.notFound") return `${params?.resource ?? ""} 不存在`.trim()
  // Generic interpolation so detail-level keys like
  // "api.validation.required" surface the {field} param in test output.
  if (params) {
    let out = key
    for (const [p, v] of Object.entries(params)) {
      out = out.replace(new RegExp(`\\{${p}\\}`, "g"), String(v))
    }
    return params.field ? `${out}[${params.field}]` : out
  }
  return key
}

describe("formApiErrorMessage (bug #219)", () => {
  it("preserves raw backend message on a second line when translation is reason-only", () => {
    // The MySQL duplicate-name case: backend returns " uk_mysql_instances_workspace already exists"
    // (leading space because domainErr drops the resource arg). No prefix/exact map matches, so the
    // reason fallback would otherwise drop err.message entirely.
    const err = new ApiError({
      apiVersion: "",
      kind: "Status",
      status: 409,
      reason: "Conflict",
      message: " uk_mysql_instances_workspace already exists",
    })
    const out = formApiErrorMessage(err, t)
    expect(out).toBe("操作冲突\n uk_mysql_instances_workspace already exists")
  })

  it("uses raw message when no translation key matches at all", () => {
    const err = new ApiError({
      apiVersion: "",
      kind: "Status",
      status: 500,
      reason: "SomeUnknownReason",
      message: "kernel panic: ENOMEM",
    })
    expect(formApiErrorMessage(err, t)).toBe("kernel panic: ENOMEM")
  })

  it("returns a single-line headline when a specific message-level map matches", () => {
    // "cannot delete workspace: user(2)" hits the prefix map ->
    // "api.error.cannotDeleteWorkspace" with the suffix appended inline.
    const err = new ApiError({
      apiVersion: "",
      kind: "Status",
      status: 409,
      reason: "Conflict",
      message: "cannot delete workspace: user(2)",
    })
    const out = formApiErrorMessage(err, t)
    // Headline plus the cleanly-extracted suffix; no embedded "\n".
    expect(out).not.toContain("\n")
  })

  it("surfaces the first field-level detail when err.details is non-empty", () => {
    const err = new ApiError({
      apiVersion: "",
      kind: "Status",
      status: 400,
      reason: "BadRequest",
      message: "validation failed",
      details: [
        { field: "spec.name", message: "is required" },
        { field: "spec.port", message: "must be between 1 and 65535" },
      ],
    })
    const out = formApiErrorMessage(err, t)
    // The first detail wins; translateDetailMessage maps "is required" to a key
    // and the returned text references the field. We assert the field label
    // ("name", stripped of "spec.") is present.
    expect(out).toContain("name")
  })

  it("falls back to api.error.internalError for non-ApiError exceptions", () => {
    expect(formApiErrorMessage(new Error("boom"), t)).toBe("服务器内部错误")
    expect(formApiErrorMessage(undefined, t)).toBe("服务器内部错误")
    expect(formApiErrorMessage(null, t)).toBe("服务器内部错误")
  })

  it("interpolates {resource} into the headline when resourceKey is provided", () => {
    const err = new ApiError({
      apiVersion: "",
      kind: "Status",
      status: 404,
      reason: "NotFound",
      message: " resource not found",
    })
    const out = formApiErrorMessage(err, t, "mysql.title")
    // The resource template "{resource} 不存在" interpolates the key arg through t().
    expect(out.startsWith("mysql.title 不存在")).toBe(true)
    // Raw message is also preserved (reason-only fallback).
    expect(out).toContain(" resource not found")
  })
})
