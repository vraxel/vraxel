import { useMemo, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { Plus, Search, UserMinus } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { Checkbox } from "@/shared/ui/checkbox"
import { EmptyState } from "@/shared/components/empty-state"
import { Skeleton } from "@/shared/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import { NameCell } from "@/frameworks/list/name-cell"
import { Pagination } from "@/shared/components/pagination"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateText } from "@/shared/components/truncate-text"
import { RowActionsCell, RowActionsHead } from "@/shared/components/row-actions"
import { useApiQuery } from "@/core/query/hooks"
import { useListState } from "@/frameworks/list/use-list-state"
import { useQueryClient } from "@tanstack/react-query"
import { showApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import type { ListParams, BatchResult } from "@/core/api/types"
import type { RoleBinding, RoleBindingList, UserList } from "@/modules/iam/api/types"

/**
 * Scope-parameterized wiring for the role-centric bindings view. Each
 * scope (platform / workspace / namespace) injects closures that already
 * carry its URL prefix, so this component only adds the role filter.
 */
export interface RoleUsersConfig {
  roleId: string
  /** URL prefix for the user detail link (e.g. "/iam" or "/iam/workspaces/7"). */
  detailPrefix: string
  /** Lists bindings; the section injects role_id via extraParams. */
  listBindings: (params: ListParams) => Promise<RoleBindingList>
  /** Candidate users for the add dialog (all users / scope members). */
  listCandidates: (params?: ListParams) => Promise<UserList>
  /** Binds this role to the given users; returns how many were newly bound. */
  assign: (userIds: string[]) => Promise<BatchResult>
  /** Removes one binding. */
  revoke: (bindingId: string) => Promise<void>
  canAssign: boolean
  canRevoke: boolean
  /** Distinguishes the cache keys / dialog copy per scope + role. */
  cacheKey: (string | undefined)[]
}

const PAGE_SIZE = 10

/**
 * "Users with this role" -- the role-centric end of the (user, role)
 * relation, shown on the role detail page. Assigning a role to users
 * (batch) and revoking it live here, where the role is already the
 * context, instead of on a standalone role-binding list.
 */
export function RoleUsersSection({ config }: { config: RoleUsersConfig }) {
  const { t } = useTranslation()
  const qc = useQueryClient()

  // Shared list state: debounced search (300ms) + page reset on search,
  // the same behavior every other list in the app has.
  const { page, setPage, pageSize, setPageSize, searchInput, setSearchInput, search } =
    useListState({
      defaultPageSize: PAGE_SIZE,
    })
  const [addOpen, setAddOpen] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<RoleBinding | null>(null)

  const bindingsQuery = useApiQuery({
    queryKey: ["iam", "role-users", ...config.cacheKey, page, pageSize, search],
    queryFn: () => {
      const params: ListParams = {
        page,
        pageSize,
        role_id: config.roleId,
        sortBy: "created_at",
        sortOrder: "asc",
      }
      if (search) params.search = search
      return config.listBindings(params)
    },
  })
  const bindings = useMemo(() => bindingsQuery.data?.items ?? [], [bindingsQuery.data])
  const total = bindingsQuery.data?.totalCount ?? 0
  const loading = bindingsQuery.isPending

  // Assign/revoke change both the bindings list and who still qualifies as
  // a candidate (the holders query lives under the "role-users" prefix, so
  // the first call covers it too).
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["iam", "role-users", ...config.cacheKey] })
    qc.invalidateQueries({ queryKey: ["iam", "role-assign-candidates", ...config.cacheKey] })
  }

  const handleRevoke = async () => {
    if (!revokeTarget) return
    try {
      await config.revoke(revokeTarget.metadata.id)
      toast.success(t("action.deleteSuccess"))
      setRevokeTarget(null)
      refresh()
    } catch (err) {
      showApiError(err, t, "rolebinding.title")
    }
  }

  return (
    <div>
      <div className="mb-3 flex items-center justify-between gap-4">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="role-user-search"
            placeholder={t("user.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {config.canAssign && (
          <Button size="sm" onClick={() => setAddOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("role.assignUsers")}
          </Button>
        )}
      </div>

      <div className="border-border-subtle overflow-hidden rounded-xl border shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("user.username")}</TableHead>
              <TableHead className="whitespace-nowrap">{t("common.created")}</TableHead>
              {config.canRevoke && <RowActionsHead>{t("common.actions")}</RowActionsHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={3}>
                    <Skeleton className="h-5 w-full" />
                  </TableCell>
                </TableRow>
              ))
            ) : bindings.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={3} className="p-0 whitespace-normal">
                  <EmptyState title={search ? t("common.noSearchResults") : t("role.noUsers")} />
                </TableCell>
              </TableRow>
            ) : (
              bindings.map((b) => (
                <TableRow key={b.metadata.id}>
                  <TableCell>
                    <NameCell
                      to={`${config.detailPrefix}/users/${b.spec.userId}`}
                      displayName={b.spec.userDisplayName}
                      name={b.spec.username ?? ""}
                    />
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                    {formatDateTime(b.metadata.createdAt)}
                  </TableCell>
                  {config.canRevoke && (
                    <RowActionsCell>
                      {/* The owner's binding is structural and cannot be revoked here. */}
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        disabled={!!b.spec.isOwner}
                        title={
                          b.spec.isOwner ? t("rolebinding.ownerLocked") : t("rolebinding.revoke")
                        }
                        onClick={() => setRevokeTarget(b)}
                      >
                        <UserMinus className="h-3.5 w-3.5" />
                      </Button>
                    </RowActionsCell>
                  )}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Pagination
        page={page}
        pageSize={pageSize}
        totalCount={total}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
      />

      {config.canAssign && (
        <AssignUsersDialog
          open={addOpen}
          onOpenChange={setAddOpen}
          config={config}
          onDone={refresh}
        />
      )}

      <ConfirmDialog
        open={!!revokeTarget}
        onOpenChange={(v) => !v && setRevokeTarget(null)}
        title={t("rolebinding.revoke")}
        description={t("rolebinding.revokeConfirm", { name: revokeTarget?.spec.username ?? "" })}
        onConfirm={handleRevoke}
        confirmText={t("rolebinding.revoke")}
      />
    </div>
  )
}

function AssignUsersDialog({
  open,
  onOpenChange,
  config,
  onDone,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  config: RoleUsersConfig
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const candidatesQuery = useApiQuery({
    queryKey: ["iam", "role-assign-candidates", ...config.cacheKey],
    queryFn: () => config.listCandidates({ pageSize: 100 }),
    enabled: open,
  })
  // Users already holding THIS role are not candidates. Keyed under the
  // "role-users" prefix so the section's refresh() invalidates it too.
  // Exclusion is best-effort within the first page; server-side assign is
  // idempotent, so any holder that slips through just counts as skipped.
  const holdersQuery = useApiQuery({
    queryKey: ["iam", "role-users", ...config.cacheKey, "assign-holders"],
    queryFn: () => config.listBindings({ role_id: config.roleId, pageSize: 100 }),
    enabled: open,
  })
  const users = useMemo(() => {
    const held = new Set((holdersQuery.data?.items ?? []).map((b) => b.spec.userId))
    return (candidatesQuery.data?.items ?? []).filter((u) => !held.has(u.metadata.id))
  }, [candidatesQuery.data, holdersQuery.data])

  // Reset when the dialog opens (adjust-during-render, not an effect).
  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) {
      setSelected(new Set())
      setSearch("")
    }
  }

  const filtered = search
    ? users.filter((u) => {
        const q = search.toLowerCase()
        return (
          u.spec.username.toLowerCase().includes(q) ||
          u.spec.email?.toLowerCase().includes(q) ||
          u.spec.displayName?.toLowerCase().includes(q)
        )
      })
    : users

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const handleSubmit = async () => {
    if (selected.size === 0) return
    setSubmitting(true)
    try {
      const result = await config.assign(Array.from(selected))
      // Idempotent server-side: failedCount means "already had this role",
      // not an error -- report it so the number reconciles.
      if (result.failedCount > 0) {
        toast.success(
          t("rolebinding.createPartial", {
            created: result.successCount,
            skipped: result.failedCount,
          }),
        )
      } else {
        toast.success(t("action.createSuccess"))
      }
      onOpenChange(false)
      onDone()
    } catch (err) {
      showApiError(err, t, "rolebinding.title")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-lg"
        onOpenAutoFocus={(e) => e.preventDefault()}
        aria-describedby={undefined}
      >
        <DialogHeader>
          <DialogTitle>{t("role.assignUsers")}</DialogTitle>
          <DialogDescription>{t("role.assignUsersHint")}</DialogDescription>
        </DialogHeader>

        <div className="relative">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="assign-user-search"
            placeholder={t("user.searchPlaceholder")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto rounded-lg border">
          {candidatesQuery.isPending || holdersQuery.isPending ? (
            <div className="space-y-2 p-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <p className="text-muted-foreground p-4 text-center text-sm">
              {search ? t("common.noSearchResults") : t("rolebinding.noUsers")}
            </p>
          ) : (
            filtered.map((user) => (
              <label
                key={user.metadata.id}
                className={`hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-2 ${selected.has(user.metadata.id) ? "bg-muted" : ""}`}
              >
                <Checkbox
                  name={`assign-user-${user.metadata.id}`}
                  checked={selected.has(user.metadata.id)}
                  onCheckedChange={() => toggle(user.metadata.id)}
                />
                <div className="min-w-0 flex-1">
                  <TruncateText className="text-sm font-medium">{user.spec.username}</TruncateText>
                  <TruncateText className="text-muted-foreground text-xs">
                    {user.spec.displayName || user.spec.email}
                  </TruncateText>
                </div>
              </label>
            ))
          )}
        </div>

        <DialogFooter className="mt-4 border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={selected.size === 0 || submitting}>
            {submitting
              ? "..."
              : selected.size > 1
                ? t("rolebinding.createN", { count: selected.size })
                : t("rolebinding.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
