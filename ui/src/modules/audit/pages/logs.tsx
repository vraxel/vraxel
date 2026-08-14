import { useState } from "react"
import { formatDateTimeSeconds } from "@/shared/lib/format"
import { Eye, Filter, X } from "lucide-react"
import type { DateRange } from "react-day-picker"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Input } from "@/shared/ui/input"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/shared/ui/dialog"
import { Separator } from "@/shared/ui/separator"
import { Popover, PopoverTrigger, PopoverContent } from "@/shared/ui/popover"
import { DateRangePicker } from "@/shared/ui/date-range-picker"
import { auditLogsApi, type AuditLogRow } from "@/modules/audit/api/logs"
import { auditLogsDef } from "@/modules/audit/defs"
import type { AuditLog } from "@/modules/audit/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import { useTranslation } from "@/i18n"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { SortIcon } from "@/shared/components/sort-icon"

function formatJsonDetail(detail: Record<string, unknown>): string {
  return JSON.stringify(detail, null, 2)
}

// AUDIT_MODULES is the full list of Vraxel modules whose write operations
// flow through the audit middleware. Must match the top-level directories
// under pkg/apis/. The audit backend stores whichever module the URL path
// resolves to, so this list only affects the UI filter dropdown.
const AUDIT_MODULES = [
  "app",
  "audit",
  "dashboard",
  "db",
  "iam",
  "infra",
  "mq",
  "mw",
  "network",
  "o11y",
  "ops",
  "pki",
  "system",
] as const

// Action filter options; each maps to an audit.action.<a> i18n key.
const AUDIT_ACTIONS = [
  "create",
  "update",
  "patch",
  "delete",
  "deleteCollection",
  "exec",
  "login",
  "login_failed",
  "token_refresh",
  "start",
  "stop",
  "restart",
  "switchover",
  "reboot",
  "power-on",
  "shutdown",
  "snapshot",
  "reconfigure",
  "change-password",
  "publish-config",
  "delete-config",
  "rollback-config",
  "create-namespace",
  "update-namespace",
  "delete-namespace",
  "config-validate",
  "config-apply",
  "reload",
] as const

export default function AuditLogListPage() {
  const { t } = useTranslation()
  // resourceType is a free-text header filter (Popover + Input), and the
  // date range produces startTime/endTime -- neither fits the framework's
  // dropdown filterKeys, so both live as local state fed into extraParams
  // (still part of the query key, so changes refetch). Page reset on
  // change is manual (query.setPage(1)) since only setFilter/search do it
  // automatically.
  const [resourceTypeFilter, setResourceTypeFilter] = useState<string>("")
  const [resourceTypeInput, setResourceTypeInput] = useState<string>("")
  const [resourceTypePopoverOpen, setResourceTypePopoverOpen] = useState(false)
  const [dateRange, setDateRange] = useState<DateRange | undefined>()

  // detail dialog
  const [selectedLog, setSelectedLog] = useState<AuditLog | null>(null)

  const scope: ScopeRef = {}
  const query = useListQuery<AuditLogRow>({
    def: auditLogsDef,
    api: auditLogsApi,
    scope,
    filterKeys: ["event_type", "action", "module", "success"],
    extraParams: {
      resource_type: resourceTypeFilter || undefined,
      start_time: dateRange?.from?.toISOString(),
      end_time: dateRange?.to?.toISOString(),
    },
  })

  const columns: ColumnDef<AuditLogRow>[] = [
    {
      key: "username",
      header: t("audit.username"),
      sortable: true,
      truncate: true,
      className: "font-medium",
      cell: (log) => log.spec.username || log.spec.clientIp || "-",
    },
    {
      key: "event_type",
      header: t("audit.eventType"),
      sortable: true,
      filter: [
        { value: "all", label: t("common.all") },
        { value: "api_operation", label: t("audit.eventType.api_operation") },
        { value: "authentication", label: t("audit.eventType.authentication") },
      ],
      cell: (log) => t(`audit.eventType.${log.spec.eventType}`),
    },
    {
      key: "resource_type",
      // Free-text filter + separate sort trigger; not expressible via the
      // ColumnDef filter (dropdown) so the header is hand-built like the
      // pre-migration page.
      header: (
        <div className="flex items-center gap-0.5">
          <Popover open={resourceTypePopoverOpen} onOpenChange={setResourceTypePopoverOpen}>
            <PopoverTrigger asChild>
              <button className="inline-flex items-center gap-1 select-none">
                {t("audit.resourceType")}
                <Filter
                  className={`h-3 w-3 ${resourceTypeFilter ? "text-primary" : "opacity-40"}`}
                />
              </button>
            </PopoverTrigger>
            <PopoverContent className="w-64 p-3" align="start">
              <div className="flex items-center gap-1.5">
                <Input
                  name="audit-resource-type-filter"
                  placeholder={t("audit.filter.resourceTypePlaceholder")}
                  value={resourceTypeInput}
                  onChange={(e) => setResourceTypeInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      setResourceTypeFilter(resourceTypeInput.trim())
                      setResourceTypePopoverOpen(false)
                      query.setPage(1)
                    }
                  }}
                  className="h-8 text-sm"
                />
                {resourceTypeFilter && (
                  <button
                    className="text-muted-foreground hover:text-foreground shrink-0"
                    onClick={() => {
                      setResourceTypeFilter("")
                      setResourceTypeInput("")
                      setResourceTypePopoverOpen(false)
                      query.setPage(1)
                    }}
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
              {resourceTypeFilter && (
                <p className="text-muted-foreground mt-2 text-xs">
                  {t("common.current")}:{" "}
                  <code className="text-foreground">{resourceTypeFilter}</code>
                </p>
              )}
            </PopoverContent>
          </Popover>
          <button
            className="hover:text-foreground cursor-pointer transition-colors select-none"
            onClick={() => query.handleSort("resource_type")}
          >
            <SortIcon field="resource_type" sortBy={query.sortBy} sortOrder={query.sortOrder} />
          </button>
        </div>
      ),
      truncate: true,
      cell: (log) =>
        log.spec.resourceType
          ? `${log.spec.resourceType}${log.spec.resourceId ? `/${log.spec.resourceId}` : ""}`
          : "-",
    },
    {
      key: "action",
      header: t("audit.action"),
      sortable: true,
      filter: [
        { value: "all", label: t("common.all") },
        ...AUDIT_ACTIONS.map((a) => ({ value: a, label: t(`audit.action.${a}`) })),
      ],
      cell: (log) => t(`audit.action.${log.spec.action}`),
    },
    {
      key: "module",
      header: t("audit.module"),
      sortable: true,
      truncate: true,
      filter: [
        { value: "all", label: t("common.all") },
        ...AUDIT_MODULES.map((m) => ({ value: m, label: m })),
      ],
      cell: (log) => log.spec.module || "-",
    },
    {
      key: "success",
      header: t("audit.success"),
      sortable: true,
      filter: [
        { value: "all", label: t("common.all") },
        { value: "true", label: t("audit.success.true") },
        { value: "false", label: t("audit.success.false") },
      ],
      cell: (log) => (
        <Badge variant={log.spec.success ? "default" : "destructive"}>
          {log.spec.success ? t("audit.success.true") : t("audit.success.false")}
        </Badge>
      ),
    },
    {
      key: "status_code",
      header: t("audit.statusCode"),
      sortable: true,
      cell: (log) => log.spec.statusCode ?? "-",
    },
    {
      key: "duration_ms",
      header: t("audit.duration"),
      sortable: true,
      cell: (log) => (log.spec.durationMs != null ? `${log.spec.durationMs}ms` : "-"),
    },
    {
      key: "created_at",
      header: t("audit.createdAt"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (log) => formatDateTimeSeconds(log.spec.createdAt),
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="audit.title"
      subtitle={t("audit.manage", { count: query.totalCount })}
      searchPlaceholderKey="audit.searchPlaceholder"
      selectable={false}
      emptyKey="audit.noData"
      toolbarExtra={
        <DateRangePicker
          value={dateRange}
          onChange={(v) => {
            setDateRange(v)
            query.setPage(1)
          }}
          placeholder={t("audit.filter.dateRange")}
          resetLabel={t("common.reset")}
          className="h-9 w-auto"
        />
      }
      rowActions={(log) => (
        <>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={() => setSelectedLog(log)}
            title={t("audit.viewDetail")}
          >
            <Eye className="h-3.5 w-3.5" />
          </Button>
        </>
      )}
    >
      {/* detail dialog */}
      <Dialog
        open={!!selectedLog}
        onOpenChange={(v) => {
          if (!v) setSelectedLog(null)
        }}
      >
        <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("audit.detail")}</DialogTitle>
            <DialogDescription>ID: {selectedLog?.spec.id}</DialogDescription>
          </DialogHeader>
          {selectedLog && (
            <div className="-mx-1 min-h-0 flex-1 space-y-5 overflow-y-auto px-1">
              {/* Two-column: Basic + Resource */}
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                {/* Basic */}
                <div className="space-y-3">
                  <h3 className="text-sm font-semibold">{t("audit.detail")}</h3>
                  <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                    <dt className="text-muted-foreground">{t("audit.username")}</dt>
                    <dd className="font-medium">{selectedLog.spec.username || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.userId")}</dt>
                    <dd>{selectedLog.spec.userId || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.eventType")}</dt>
                    <dd>{t(`audit.eventType.${selectedLog.spec.eventType}`)}</dd>
                    <dt className="text-muted-foreground">{t("audit.action")}</dt>
                    <dd>{t(`audit.action.${selectedLog.spec.action}`)}</dd>
                    <dt className="text-muted-foreground">{t("audit.success")}</dt>
                    <dd>
                      <Badge variant={selectedLog.spec.success ? "default" : "destructive"}>
                        {selectedLog.spec.success
                          ? t("audit.success.true")
                          : t("audit.success.false")}
                      </Badge>
                    </dd>
                    <dt className="text-muted-foreground">{t("audit.createdAt")}</dt>
                    <dd>{formatDateTimeSeconds(selectedLog.spec.createdAt)}</dd>
                  </dl>
                </div>

                {/* Resource */}
                <div className="space-y-3">
                  <h3 className="text-sm font-semibold">{t("audit.resourceType")}</h3>
                  <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                    <dt className="text-muted-foreground">{t("audit.resourceType")}</dt>
                    <dd>{selectedLog.spec.resourceType || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.resourceId")}</dt>
                    <dd>{selectedLog.spec.resourceId || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.module")}</dt>
                    <dd>{selectedLog.spec.module || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.scope")}</dt>
                    <dd>{selectedLog.spec.scope}</dd>
                    <dt className="text-muted-foreground">{t("audit.workspaceId")}</dt>
                    <dd>{selectedLog.spec.workspaceId || "-"}</dd>
                    <dt className="text-muted-foreground">{t("audit.namespaceId")}</dt>
                    <dd>{selectedLog.spec.namespaceId || "-"}</dd>
                  </dl>
                </div>
              </div>

              <Separator />

              {/* HTTP section - full width, two-column grid */}
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">HTTP</h3>
                <dl className="grid grid-cols-1 gap-x-8 gap-y-2 text-sm md:grid-cols-2">
                  <div className="grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.httpMethod")}</dt>
                    <dd className="font-mono">{selectedLog.spec.httpMethod || "-"}</dd>
                  </div>
                  <div className="grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.statusCode")}</dt>
                    <dd>
                      <span
                        className={
                          (selectedLog.spec.statusCode ?? 0) < 400
                            ? "text-success"
                            : "text-destructive"
                        }
                      >
                        {selectedLog.spec.statusCode ?? "-"}
                      </span>
                    </dd>
                  </div>
                  <div className="col-span-full grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.httpPath")}</dt>
                    <dd className="font-mono break-all">{selectedLog.spec.httpPath || "-"}</dd>
                  </div>
                  <div className="grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.duration")}</dt>
                    <dd>
                      {selectedLog.spec.durationMs != null
                        ? `${selectedLog.spec.durationMs}ms`
                        : "-"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.clientIp")}</dt>
                    <dd className="font-mono">{selectedLog.spec.clientIp || "-"}</dd>
                  </div>
                  <div className="col-span-full grid grid-cols-[auto_1fr] gap-x-4">
                    <dt className="text-muted-foreground">{t("audit.userAgent")}</dt>
                    <dd className="break-all">{selectedLog.spec.userAgent || "-"}</dd>
                  </div>
                </dl>
              </div>

              {/* Request Body */}
              {selectedLog.spec.detail && (
                <>
                  <Separator />
                  <div className="space-y-3">
                    <h3 className="text-sm font-semibold">{t("audit.detail.field")}</h3>
                    <pre className="bg-muted/50 max-h-80 overflow-auto rounded-md border p-4 font-mono text-xs leading-relaxed">
                      {formatJsonDetail(selectedLog.spec.detail)}
                    </pre>
                  </div>
                </>
              )}

              {/* Response Body */}
              {selectedLog.spec.responseDetail && (
                <>
                  <Separator />
                  <div className="space-y-3">
                    <h3 className="text-sm font-semibold">{t("audit.responseDetail.field")}</h3>
                    <pre className="bg-muted/50 max-h-80 overflow-auto rounded-md border p-4 font-mono text-xs leading-relaxed">
                      {formatJsonDetail(selectedLog.spec.responseDetail)}
                    </pre>
                  </div>
                </>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </ResourceListPage>
  )
}
