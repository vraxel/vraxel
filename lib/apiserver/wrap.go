package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/rest"
	"vraxel.io/vraxel/lib/runtime"
)

// listEnvelope is the wire shape of every v1 list response:
// {kind, apiVersion, items, totalCount}. Ops.List returns the pure
// list.Result[T]; the wrapper folds it into this envelope so modules no
// longer hand-write per-resource XxxList types (they may keep them for
// docs, but the wire comes from here).
type listEnvelope[T any] struct {
	runtime.TypeMeta `json:",inline"`
	Items            []T   `json:"items"`
	TotalCount       int64 `json:"totalCount"`
}

func (l *listEnvelope[T]) GetTypeMeta() *runtime.TypeMeta { return &l.TypeMeta }

// newListEnvelope folds a list.Result into the wire envelope. Items is
// never nil — v1 storages always allocated, serializing as [].
func newListEnvelope[T any](kind string, res *list.Result[T]) *listEnvelope[T] {
	env := &listEnvelope[T]{TypeMeta: runtime.TypeMeta{Kind: kind}, Items: []T{}}
	if res != nil {
		if res.Items != nil {
			env.Items = res.Items
		}
		env.TotalCount = res.TotalCount
	}
	return env
}

// defaultListKind derives "{TypeName}List" from T.
func defaultListKind[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().Name() + "List"
}

// The wrappers below are where v1's per-storage boilerplate went to die:
// ID parsing, dryRun handling, body limits, domain-error mapping, the
// apiVersion stamp and status-code selection all live here exactly once.
// Every behavioral quirk of the v1 installer handlers is reproduced
// deliberately — the wire harness diffs byte-for-byte against v1.

// wrapList serves GET on the collection path.
func wrapList[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	kind := def.ListKind
	if kind == "" {
		kind = defaultListKind[T]()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		q := parseListQuery(r, def.MaxPageSize)
		res, err := def.Ops.List(ctx, q)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, newListEnvelope(kind, res))
	})
}

// wrapListAny serves GET on the collection path for polymorphic lists:
// the op picks its own envelope, serialized as-is.
func wrapListAny[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.Ops.ListAny(ctx, parseListQuery(r, def.MaxPageSize))
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeActionResult(w, r, ctx, m, http.StatusOK, res)
	})
}

// wrapCreateAny serves POST on the collection path for asymmetric
// creates: the op decodes its own request and picks its response shape.
// Status 201, dryRun 200 — same contract as wrapCreate.
func wrapCreateAny[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		body, err := readBodyLimited(w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		// Bind the ResponseHeaders conduit exactly like wrapCreate: an
		// asymmetric create may publish response headers (ai chat sets
		// X-Vraxel-Trace-Id / X-Vraxel-Provider / ...). writeActionResult already
		// flushes it; without the bind those headers are silently dropped.
		bindResponseHeaders(&ctx, r)
		res, err := def.Ops.CreateAny(ctx, body)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		status := http.StatusCreated
		if ctx.DryRun {
			status = http.StatusOK
		}
		s.writeActionResult(w, r, ctx, m, status, res)
	})
}

// wrapSingleton serves GET on the collection path returning one object
// (no list envelope) — the SingletonGet contract.
func wrapSingleton[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.SingletonGet(ctx)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, asObject(res))
	})
}

// wrapGet serves GET on the item path.
func wrapGet[T any](s *Server, def ResourceDef[T], idParam string) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.Ops.Get(ctx, ctx.ID)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, asObject(res))
	})
}

// verbRoute adapts a verb handler to its own GET route under the item
// path; the id segment passes through raw (string-keyed legacy Listers).
func verbRoute(s *Server, vh verbHandler, idParam string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		vh(s, m, w, r, pathValue(r, idParam))
	})
}

// wrapCreate serves POST on the collection path. Status 201, dryRun 200,
// ResponseHeaders conduit bound — v1 createHandler parity.
func wrapCreate[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		in, err := decodeBody[T](s, w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		bindResponseHeaders(&ctx, r)

		res, err := def.Ops.Create(ctx, in)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		status := http.StatusCreated
		if ctx.DryRun {
			status = http.StatusOK
		}
		rest.ApplyResponseHeaders(ctx, w)
		s.writeObject(w, r, m, status, asObject(res))
	})
}

// wrapUpdate serves PUT on the item path.
func wrapUpdate[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		in, err := decodeBody[T](s, w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.Ops.Update(ctx, ctx.ID, in)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, asObject(res))
	})
}

// wrapPatch serves PATCH on the item path, handing the raw body to the
// resource so absent-vs-zero stays distinguishable in its own patch type.
func wrapPatch[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		body, err := readBodyLimited(w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.Ops.Patch(ctx, ctx.ID, body)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, asObject(res))
	})
}

// wrapDelete serves DELETE on the item path. 204 on success.
func wrapDelete[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if err := def.Ops.Delete(ctx, ctx.ID); err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// wrapBatchDelete serves DELETE on the collection path with the v1
// {"ids": [...]} body contract. Quirk-compat: like v1, a malformed body
// or an empty id list surfaces as a 500 (plain error through the
// negotiated error writer), because that is what the wire harness will
// record from v1.
func wrapBatchDelete[T any](s *Server, def ResourceDef[T]) http.Handler {
	label := singularLabel(def.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		body, err := readBodyLimited(w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		var req BatchDeleteRequest
		if err := json.Unmarshal(body, &req); err != nil {
			s.writeError(w, r, err)
			return
		}
		if len(req.IDs) == 0 {
			s.writeError(w, r, fmt.Errorf("no ids provided"))
			return
		}
		ids, err := parseIDs(req.IDs)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := def.Ops.BatchDelete(ctx, ids)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, res)
	})
}

// ─── shared plumbing ───

// parseListQuery converts URL query values into a list.Query with the
// exact semantics of v1 ParseListOptions + the per-module
// restOptionsToListQuery copies it replaces (defaults page=1/pageSize=20,
// reserved keys excluded from filters, filters kept as strings).
func parseListQuery(r *http.Request, maxPageSize int32) list.Query {
	o := rest.ParseListOptions(r.URL.Query())
	q := list.Query{
		Filters: make(map[string]any, len(o.Filters)),
		Pagination: list.Pagination{
			Page:      o.Pagination.Page,
			PageSize:  o.Pagination.PageSize,
			SortBy:    o.SortBy,
			SortOrder: string(o.SortOrder),
		},
		MaxPageSize: maxPageSize,
	}
	for k, v := range o.Filters {
		q.Filters[k] = v
	}
	return q
}

// parseIDs converts the string id list of a batch delete, mirroring the
// per-module loop it replaces (any invalid id → 400).
func parseIDs(ids []string) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ID: %s", id), nil)
		}
		out = append(out, parsed)
	}
	return out, nil
}

// decodeBody reads and decodes a create/update body into *T through the
// v1 negotiated decoder (Content-Type json/yaml), preserving decode
// error shapes.
func decodeBody[T any](s *Server, w http.ResponseWriter, r *http.Request, limit int64) (*T, error) {
	body, err := readBodyLimited(w, r, limit)
	if err != nil {
		return nil, err
	}
	into := new(T)
	obj, ok := any(into).(runtime.Object)
	if !ok { // unreachable: Register panics on non-Object T
		return nil, apierrors.NewInternalError(fmt.Errorf("resource type %T does not implement runtime.Object", into))
	}
	decoded, err := rest.DecodeBody(s.serializer, r, body, obj)
	if err != nil {
		return nil, err
	}
	out, ok := any(decoded).(*T)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("decoded %T, expected %T", decoded, into))
	}
	return out, nil
}

// readBodyLimited enforces the per-resource body cap with v1
// readBodyWithLimit semantics (413 with the same message).
func readBodyLimited(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = rest.DefaultMaxRequestBodySize
	}
	defer func() { _ = r.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, apierrors.NewRequestEntityTooLarge(fmt.Sprintf("request body exceeds %d bytes", limit))
	}
	return data, nil
}

// bindResponseHeaders installs the ResponseHeaders conduit so handlers
// can publish headers (X-Vraxel-Trace-Id) without seeing the writer.
func bindResponseHeaders(ctx *Ctx, r *http.Request) {
	hdrs := &rest.ResponseHeaders{}
	ctx.Context = rest.WithResponseHeaders(ctx.Context, hdrs)
}

// mapDomain converts store-layer sentinel errors into API errors using
// the resource label — the framework-level home of the 5-line
// apierrors.FromDomain block every v1 storage method repeated.
func mapDomain(err error, label string) error {
	if err == nil {
		return nil
	}
	if se := apierrors.FromDomain(err, label); se != nil {
		return se
	}
	return err
}

// asObject converts a typed result into the serializer interface.
// Register guarantees *T implements runtime.Object at startup, so the
// assertion cannot fail at request time; Go just needs the explicit
// any() hop because a pointer-to-type-parameter never satisfies an
// interface directly. A nil *T maps to an untyped nil interface —
// exactly what v1 storages returned — so downstream nil checks and the
// serializer see the same value they always did.
func asObject[T any](v *T) runtime.Object {
	if v == nil {
		return nil
	}
	return any(v).(runtime.Object)
}

// invalidIDError formats the v1-style bad-id message from a path param
// name: "channelId"/"abc" → `invalid channel ID: abc`.
func invalidIDError(param, raw string) error {
	label := strings.TrimSuffix(param, "Id")
	return apierrors.NewBadRequest(fmt.Sprintf("invalid %s ID: %s", label, raw), nil)
}

// invalidJSONBody keeps typed action decode errors on the v1
// InvalidJSONBody wire shape.
func invalidJSONBody(err error) error {
	return apierrors.NewInvalidJSONBody(err)
}

// notFoundRoute is the shim's 404 for unknown verbs / absent handlers.
func notFoundRoute(r *http.Request) error {
	return apierrors.NewNotFound("route", r.URL.Path)
}
