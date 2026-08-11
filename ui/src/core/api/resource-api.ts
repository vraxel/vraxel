import type { Options } from "ky"
import { api, apiRequest } from "./client"
import { toSearchParams } from "./search-params"
import { resourcePath, type ResourceDef, type ScopeRef } from "@/core/registry/resource"
import type { ListParams, TypeMeta } from "./types"

export interface ListResponse<T> extends TypeMeta {
  items: T[]
  totalCount: number
}

// Per-verb request options (timeout etc.). Real CRUD is rarely fully
// standard -- hosts.delete carries preserveVM + a 90s timeout -- so every
// derived verb accepts extra ky options at the call site via these
// defaults, and modules can still replace whole verbs by spreading.
interface VerbOptions {
  timeout?: number
}

export interface ResourceApiOptions {
  list?: VerbOptions
  get?: VerbOptions
  create?: VerbOptions
  update?: VerbOptions
  patch?: VerbOptions
  delete?: VerbOptions
  deleteCollection?: VerbOptions
}

function kyOpts(v?: VerbOptions): Options {
  return v?.timeout != null ? { timeout: v.timeout } : {}
}

// Delete/action handlers reply 204 with an empty body (lib/apiserver
// wrapDelete); ky's .json() would JSON.parse("") and throw SyntaxError,
// so read text and only parse when a body is present.
async function jsonOrEmpty<T>(resp: Promise<Response>): Promise<T> {
  const r = await resp
  const text = await r.text()
  return (text === "" ? undefined : JSON.parse(text)) as T
}

/**
 * Standard CRUD over one resource, scope passed per call (collapses the
 * old platform/workspace/namespace function triplets). Custom verbs,
 * actions and sub-resources are declared separately -- see defineAction /
 * defineVerb / defineSubApi.
 */
export function defineResourceApi<
  T,
  TList = ListResponse<T>,
  ListP extends object = ListParams,
  CreateP = Partial<T>,
  UpdateP = Partial<T>,
  TDeleteCollection = void,
>(def: ResourceDef, opts: ResourceApiOptions = {}) {
  return {
    def,
    list: (s: ScopeRef, params?: ListP) =>
      apiRequest<TList>(
        api
          .get(resourcePath(def, s), { searchParams: toSearchParams(params), ...kyOpts(opts.list) })
          .json(),
      ),
    get: (s: ScopeRef, id: string | number) =>
      apiRequest<T>(api.get(resourcePath(def, s, id), kyOpts(opts.get)).json()),
    create: (s: ScopeRef, body: CreateP) =>
      apiRequest<T>(api.post(resourcePath(def, s), { json: body, ...kyOpts(opts.create) }).json()),
    update: (s: ScopeRef, id: string | number, body: UpdateP) =>
      apiRequest<T>(
        api.put(resourcePath(def, s, id), { json: body, ...kyOpts(opts.update) }).json(),
      ),
    patch: (s: ScopeRef, id: string | number, body: Partial<T> | object) =>
      apiRequest<T>(
        api.patch(resourcePath(def, s, id), { json: body, ...kyOpts(opts.patch) }).json(),
      ),
    delete: (s: ScopeRef, id: string | number, params?: object) =>
      apiRequest<void>(
        jsonOrEmpty(
          api.delete(resourcePath(def, s, id), {
            searchParams: toSearchParams(params),
            ...kyOpts(opts.delete),
          }),
        ),
      ),
    // Backend CollectionDeleter: DELETE /{resource} with an ids body.
    deleteCollection: (s: ScopeRef, ids: string[]) =>
      apiRequest<TDeleteCollection>(
        jsonOrEmpty(
          api.delete(resourcePath(def, s), { json: { ids }, ...kyOpts(opts.deleteCollection) }),
        ),
      ),
  }
}

/** Mutating action: POST /{resource}/{id}/{action} (backend Actions[]). */
export function defineAction<TBody = void, TResp = unknown>(def: ResourceDef, action: string) {
  return (s: ScopeRef, id: string | number, body?: TBody) =>
    apiRequest<TResp>(
      jsonOrEmpty(
        api.post(resourcePath(def, s, id, action), body === undefined ? {} : { json: body }),
      ),
    )
}

/** Read-only custom verb: GET /{resource}/{id}:{verb} (backend CustomVerbs[]). */
export function defineVerb<TResp>(def: ResourceDef, verb: string) {
  return (s: ScopeRef, id: string | number, params?: object) =>
    apiRequest<TResp>(
      api.get(resourcePath(def, s, id, { verb }), { searchParams: toSearchParams(params) }).json(),
    )
}

/** Nested CRUD under a parent item: /{parent}/{id}/{sub}... (backend SubResources[]). */
export function defineSubApi<T, TList = ListResponse<T>>(def: ResourceDef, sub: string) {
  return {
    list: (s: ScopeRef, parentId: string | number, params?: object) =>
      apiRequest<TList>(
        api
          .get(resourcePath(def, s, parentId, sub), { searchParams: toSearchParams(params) })
          .json(),
      ),
    get: (s: ScopeRef, parentId: string | number, id: string | number) =>
      apiRequest<T>(api.get(resourcePath(def, s, parentId, sub, id)).json()),
    create: (s: ScopeRef, parentId: string | number, body: object) =>
      apiRequest<T>(api.post(resourcePath(def, s, parentId, sub), { json: body }).json()),
    update: (s: ScopeRef, parentId: string | number, id: string | number, body: object) =>
      apiRequest<T>(api.put(resourcePath(def, s, parentId, sub, id), { json: body }).json()),
    delete: (s: ScopeRef, parentId: string | number, id: string | number) =>
      apiRequest<void>(jsonOrEmpty(api.delete(resourcePath(def, s, parentId, sub, id)))),
  }
}
