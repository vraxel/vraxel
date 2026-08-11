import type { ResourceDef, ScopeRef } from "@/core/registry/resource"

// Query keys bind cached data to its full identity: module, resource,
// scope, then shape + params. Stale responses can no longer clobber a
// different page/search/scope -- the pre-refactor race class (analysis.md
// 根因 B) becomes structurally impossible. Always build keys through
// this factory; hand-written key arrays are a lint error in W3.
export const qk = {
  module: (module: string) => [module] as const,
  resource: (def: ResourceDef) => [def.module, def.name] as const,
  scope: (def: ResourceDef, s: ScopeRef) => [def.module, def.name, s.ws ?? "", s.ns ?? ""] as const,
  list: (def: ResourceDef, s: ScopeRef, params?: object) =>
    [def.module, def.name, s.ws ?? "", s.ns ?? "", "list", params ?? {}] as const,
  detail: (def: ResourceDef, s: ScopeRef, id: string | number) =>
    [def.module, def.name, s.ws ?? "", s.ns ?? "", "detail", String(id)] as const,
  sub: (def: ResourceDef, s: ScopeRef, id: string | number, sub: string, params?: object) =>
    [
      def.module,
      def.name,
      s.ws ?? "",
      s.ns ?? "",
      "detail",
      String(id),
      sub,
      params ?? {},
    ] as const,
}
