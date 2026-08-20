import { defineResourceApi } from "@/core/api/resource-api"
import type { HostAlertRule } from "@/generated/compute"
import type { ListResponse } from "@/core/api/resource-api"
import { hostAlertRulesDef } from "../defs"

// The metric keys mirror the server's whitelist (agentgw.AlertMetrics);
// labels resolve through i18n so a dropdown reads "CPU 使用率", not a
// snake_case key. Lives here rather than in the dialog component so
// react-refresh keeps component files component-only.
export const ALERT_METRICS = [
  "cpu_used_pct",
  "mem_used_pct",
  "disk_used_pct",
  "load1",
  "load5",
  "load15",
  "net_rx_bps",
  "net_tx_bps",
] as const

// Create/update send the writable half only: tenancy is fixed by the
// URL's scope at creation, and the read-only fields (firingCount,
// creator) are server-derived.
export interface AlertRuleBody {
  metadata: { name: string }
  spec: {
    description?: string
    metric: string
    op: string
    threshold: number
    forSeconds?: number
    severity: string
    enabled?: boolean
  }
}

export const alertRulesApi = defineResourceApi<
  HostAlertRule,
  ListResponse<HostAlertRule>,
  Record<string, unknown>,
  AlertRuleBody,
  AlertRuleBody
>(hostAlertRulesDef)
