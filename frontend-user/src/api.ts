export type Column = {
  name: string;
  type: string;
  encoding?: string;
  nulls?: number;
  raw_bytes?: number;
  enc_bytes?: number;
  compression?: number;
  reason?: string;
};

export type CatalogTable = {
  file_name: string;
  table: string;
  format: string;
  rows: number;
  status: string;
  columns: Column[];
};

export type Job = {
  id: string;
  status: string;
  phase: string;
  file_name: string;
  table: string;
  format: string;
  bytes_total: number;
  bytes_done: number;
  rows_done: number;
  error: string;
  reused?: boolean;
};

export type QueryResult = {
  result_set_id: string;
  schema: { name: string; type: string }[];
  total_rows: number;
  rows: (string | number | boolean | null)[][];
  elapsed_ms: number;
  scanned_rows: number;
  explain: ExplainNode[];
  sql: string;
  code?: string;
  message?: string;
};

export type ExplainNode = { op: string; detail?: string; est_rows?: number; children?: ExplainNode[] };

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, init);
  const data = await r.json().catch(() => ({}));
  if (!r.ok) {
    const err = new Error(data.message || r.statusText) as Error & { code?: string; details?: unknown };
    err.code = data.code;
    err.details = data.details;
    throw err;
  }
  return data as T;
}

export const api = {
  health: () => req<{ status: string }>("/api/v1/health"),
  catalog: () => req<{ tables: CatalogTable[] }>("/api/v1/catalog"),
  table: (name: string) => req<Record<string, unknown>>(`/api/v1/tables/${name}`),
  preview: (name: string) => req<{ columns: string[]; rows: unknown[][] }>(`/api/v1/tables/${name}/preview?n=20`),
  drop: (name: string) => req(`/api/v1/tables/${name}`, { method: "DELETE" }),
  jobs: () => req<{ jobs: Job[] }>("/api/v1/jobs"),
  job: (id: string) => req<Job>(`/api/v1/jobs/${id}`),
  retry: (id: string) => req<Job>(`/api/v1/jobs/${id}/retry`, { method: "POST" }),
  stats: () => req<Record<string, unknown>>("/api/v1/system/stats"),
  query: (sql: string) =>
    req<QueryResult>("/api/v1/query", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sql }) }),
  explain: (sql: string) =>
    req<{ explain: ExplainNode[] }>("/api/v1/query/explain", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sql }) }),
  page: (id: string, offset: number, limit: number) =>
    req<{ rows: QueryResult["rows"]; total_rows: number }>(`/api/v1/results/${id}?offset=${offset}&limit=${limit}`),
  upload: (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return req<Job>("/api/v1/files", { method: "POST", body: fd });
  },
  exportURL: (id: string, format: string) => `/api/v1/results/${id}/export?format=${format}`,
};
