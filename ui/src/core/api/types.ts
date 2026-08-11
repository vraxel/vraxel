// Cross-module shared types only.
// Per-module types live in api/{module}/types.ts.

// --- Kubernetes-style envelope ---

export interface TypeMeta {
  apiVersion: string
  kind: string
}

export interface ObjectMeta {
  id: string
  name: string
  createdAt: string
  updatedAt: string
}

// --- Common request/response shapes ---

export interface BatchRequest extends TypeMeta {
  ids: string[]
}

/** Envelope for any batch mutation (apiserver.BatchResult). */
export interface BatchResult {
  successCount: number
  failedCount: number
  failedIds?: string[]
}

export interface ServiceControlResult {
  jobId?: string
}

export interface ListParams {
  page?: number
  pageSize?: number
  sortBy?: string
  sortOrder?: "asc" | "desc"
  [key: string]: string | number | undefined
}

// --- Metric data point (used by host/db/mq/mw metrics tabs) ---

export interface MetricData {
  current: number
  series?: [number, string][]
}

// --- Metrics query (Prometheus/VictoriaMetrics wire format) ---

export interface MetricsQueryParams {
  query: string
  start?: string
  end?: string
  step?: string
  time?: string
  job?: string
}

export interface MetricsSeries {
  metric: Record<string, string>
  value?: [number, string] // instant query
  values?: [number, string][] // range query
}

export interface MetricsResult {
  status: string
  data: {
    resultType: "vector" | "matrix"
    result: MetricsSeries[]
  }
}

// --- Logs search (VictoriaLogs wire format) ---

export interface LogsSearchParams {
  query?: string
  start?: string
  end?: string
  unit?: string
  priority?: string
  source?: string
  instanceId?: string
  limit?: string
  node?: string
}

export interface LogEntry {
  timestamp: string
  message: string
  unit?: string
  priority?: string
  hostId?: string
  hostname?: string
}

export interface LogsHit {
  timestamp: string
  count: number
}

export interface LogsSearchResult {
  entries: LogEntry[]
  hits: LogsHit[]
}

// --- Standard error envelope ---

export interface StatusResponseDetail {
  field: string
  message: string
  // Optional source-position hints. Currently emitted by the ops
  // /scripts endpoints when shell syntax validation fails so the
  // editor can underline the offending line/column.
  line?: number
  column?: number
}

export interface StatusResponse extends TypeMeta {
  status: string | number
  reason: string
  message: string
  details?: StatusResponseDetail[]
}
