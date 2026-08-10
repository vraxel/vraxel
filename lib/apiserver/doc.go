// Package apiserver is Vraxel's REST framework: a resource definition
// is the single source of truth from which routes, permission codes,
// audit metadata and OpenAPI schemas are derived at registration time.
//
// Design:
//
//   - ResourceDef[T] + Ops[T]: explicit typed handler fields; a nil
//     field means the operation is not exposed.
//   - Routing uses the stdlib http.ServeMux (method+pattern matching).
//     Duplicate or ambiguous patterns panic at registration -- fail-fast
//     by construction.
//   - Route-level middleware (authorization, audit) is composed into each
//     route's handler chain AT REGISTRATION TIME from the same metadata
//     that produced the route. There is no URL reverse-parsing and no
//     fail-open: an unregistered path is a plain 404.
//   - RBAC scope (platform / workspace / namespace) follows registration
//     depth; permission codes ({module}:{resource}:{verb}) fall out of
//     the same metadata and are synced to the permissions table at boot.
//
// Response serialization, request decoding and the WebSocket upgrade are
// delegated to lib/rest's exported helpers; WebSocket and raw-streaming
// actions (WSAction / RawAction) mount through the same registration
// path as plain resources.
package apiserver
