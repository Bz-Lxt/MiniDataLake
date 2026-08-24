import { useEffect, useMemo, useRef, useState } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Activity, Database, Play, Search, Trash2, Upload, X } from "lucide-react";
import { api, type CatalogTable, type ExplainNode, type QueryResult } from "./api";
import { useWS } from "./store";

export function App() {
  const [left, setLeft] = useState(300);
  const [top, setTop] = useState(42);
  const [collapsed, setCollapsed] = useState(false);
  const cat = useQuery({ queryKey: ["cat"], queryFn: api.catalog, refetchInterval: 2500 });
  const jobs = useQuery({ queryKey: ["jobs"], queryFn: api.jobs, refetchInterval: 1200 });
  const stats = useQuery({ queryKey: ["stats"], queryFn: api.stats, refetchInterval: 4000 });
  const toast = useWS((s) => s.toast);

  return (
    <div className="h-full grid-overlay flex flex-col">
      <header className="h-12 border-b border-line flex items-center px-4 gap-3">
        <div className="w-2 h-2 rounded-full bg-amber shadow-[0_0_10px_#f0a202]" />
        <div className="font-semibold tracking-wide">MINI DATA LAKE</div>
        <div className="text-mute text-xs font-mono">telemetry dock · GMT+8</div>
        <div className="flex-1" />
        <button className="lg:hidden text-xs border border-line px-2 py-1 rounded" onClick={() => setCollapsed((v) => !v)}>
          {collapsed ? "目录" : "收起"}
        </button>
      </header>
      <div className="flex-1 flex min-h-0">
        <aside className={`${collapsed ? "hidden" : "flex"} lg:flex flex-col border-r border-line bg-panel`} style={{ width: left }}>
          <CatalogPane tables={cat.data?.tables || []} jobs={jobs.data?.jobs || []} />
        </aside>
        <div className="w-1.5 cursor-col-resize bg-line/60 hover:bg-cyan" onMouseDown={(e) => dragX(e, setLeft)} />
        <main className="flex-1 flex flex-col min-w-0">
          <div style={{ height: `${top}%` }} className="min-h-[160px]">
            <SQLPane tables={cat.data?.tables || []} />
          </div>
          <div className="h-1.5 cursor-row-resize bg-line/60 hover:bg-cyan" onMouseDown={(e) => dragY(e, setTop)} />
          <div className="flex-1 min-h-0">
            <ResultPane />
          </div>
        </main>
      </div>
      <StatusBar stats={stats.data} />
      {toast && (
        <div className="fixed right-4 top-16 bg-panel2 border border-line px-3 py-2 text-sm flex gap-2 items-center">
          <span>{toast}</span>
          <button onClick={() => useWS.getState().setToast("")}><X size={14} /></button>
        </div>
      )}
    </div>
  );
}

function dragX(e: React.MouseEvent, set: (n: number) => void) {
  const start = e.clientX;
  const w = (e.currentTarget.previousElementSibling as HTMLElement).getBoundingClientRect().width;
  const move = (ev: MouseEvent) => set(Math.max(220, Math.min(480, w + ev.clientX - start)));
  const up = () => { window.removeEventListener("mousemove", move); window.removeEventListener("mouseup", up); };
  window.addEventListener("mousemove", move);
  window.addEventListener("mouseup", up);
}
function dragY(e: React.MouseEvent, set: (n: number) => void) {
  const start = e.clientY;
  const parent = e.currentTarget.parentElement!.getBoundingClientRect();
  const h = ((e.currentTarget.previousElementSibling as HTMLElement).getBoundingClientRect().height / parent.height) * 100;
  const move = (ev: MouseEvent) => set(Math.max(25, Math.min(70, h + ((ev.clientY - start) / parent.height) * 100)));
  const up = () => { window.removeEventListener("mousemove", move); window.removeEventListener("mouseup", up); };
  window.addEventListener("mousemove", move);
  window.addEventListener("mouseup", up);
}

function CatalogPane({ tables, jobs }: { tables: CatalogTable[]; jobs: { id: string; status: string; phase: string; file_name: string; rows_done: number; bytes_done: number; bytes_total: number; table: string; error: string }[] }) {
  const [q, setQ] = useState("");
  const insert = useWS((s) => s.requestInsert);
  const toast = useWS((s) => s.setToast);
  const filtered = tables.filter((t) => {
    const s = q.toLowerCase();
    return !s || t.table.includes(s) || t.file_name.toLowerCase().includes(s) || t.columns.some((c) => c.name.includes(s));
  });
  return (
    <div className="flex flex-col h-full">
      <div className="p-3 border-b border-line">
        <UploadBox />
        <div className="mt-2 relative">
          <Search size={14} className="absolute left-2 top-2.5 text-mute" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索表 / 列" className="w-full bg-panel2 border border-line rounded pl-7 pr-2 py-1.5 text-xs" />
        </div>
      </div>
      <div className="flex-1 overflow-auto p-2 text-sm">
        {filtered.length === 0 && <div className="text-mute text-xs px-2 py-6">湖仓为空。拖入 CSV / JSON / Parquet。</div>}
        {filtered.map((t) => (
          <div key={t.table} className="mb-2">
            <div className="flex items-center gap-2 px-2 py-1 hover:bg-panel2 rounded">
              <Database size={14} className="text-cyan" />
              <button className="font-mono text-xs text-left flex-1" onClick={() => insert(t.table)}>{t.table}</button>
              <span className="text-[10px] text-mute">{t.rows} 行</span>
              <button className="text-mute hover:text-rose" title="卸载" onClick={async () => { await api.drop(t.table); toast(`已卸载 ${t.table}`); }}><Trash2 size={12} /></button>
            </div>
            <div className="pl-6 text-[11px] text-mute">{t.file_name} · {t.format} · {t.status}</div>
            {t.columns.map((c) => (
              <button key={c.name} onClick={() => insert(c.name)} className="w-full text-left pl-7 pr-2 py-0.5 hover:bg-panel2 font-mono text-[11px] flex justify-between">
                <span>{c.name}</span>
                <span className="text-cyan">{c.type}<span className="text-mute">/{c.encoding}</span></span>
              </button>
            ))}
          </div>
        ))}
        {jobs.filter((j) => j.status === "RUNNING" || j.status === "INTERRUPTED").map((j) => (
          <div key={j.id} className="mt-3 mx-2 text-xs border border-line rounded p-2 bg-panel2">
            <div className="flex justify-between"><span>{j.file_name}</span><span>{j.status}</span></div>
            <div className="h-1 bg-line mt-2"><div className="h-1 bg-amber" style={{ width: `${j.bytes_total ? (j.bytes_done / j.bytes_total) * 100 : 8}%` }} /></div>
            <div className="text-mute mt-1">{j.phase} · {j.rows_done} 行</div>
            {j.status === "INTERRUPTED" && <button className="mt-1 text-amber" onClick={() => api.retry(j.id)}>重跑</button>}
            {j.error && <div className="text-rose">{j.error}</div>}
          </div>
        ))}
      </div>
    </div>
  );
}

function UploadBox() {
  const toast = useWS((s) => s.setToast);
  const [drag, setDrag] = useState(false);
  const send = async (f: File) => {
    try {
      const job = await api.upload(f);
      toast(job.reused ? `复用已有表 ${job.table}` : `已开始摄取 ${f.name}`);
    } catch (e) {
      toast((e as Error).message);
    }
  };
  return (
    <label className={`block border border-dashed rounded p-3 text-center text-xs cursor-pointer ${drag ? "border-amber bg-panel2" : "border-line"}`}
      onDragOver={(e) => { e.preventDefault(); setDrag(true); }}
      onDragLeave={() => setDrag(false)}
      onDrop={(e) => { e.preventDefault(); setDrag(false); const f = e.dataTransfer.files[0]; if (f) void send(f); }}>
      <Upload size={14} className="inline mr-1 text-amber" />
      拖入或选择文件（CSV / JSON / Parquet，≤128MB）
      <input type="file" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) void send(f); }} />
    </label>
  );
}

function SQLPane({ tables }: { tables: CatalogTable[] }) {
  const sql = useWS((s) => s.sql);
  const setSql = useWS((s) => s.setSql);
  const insertAt = useWS((s) => s.insertAt);
  const clearInsert = useWS((s) => s.clearInsert);
  const push = useWS((s) => s.pushHist);
  const toast = useWS((s) => s.setToast);
  const hist = useWS((s) => s.history);
  const ref = useRef<Parameters<OnMount>[0] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [res, setRes] = useState<QueryResult | null>(null);
  const setResGlobal = useResult.setState;

  useEffect(() => {
    if (insertAt && ref.current) {
      const ed = ref.current;
      const pos = ed.getPosition();
      ed.executeEdits("ins", [{ range: { startLineNumber: pos?.lineNumber || 1, startColumn: pos?.column || 1, endLineNumber: pos?.lineNumber || 1, endColumn: pos?.column || 1 }, text: insertAt }]);
      clearInsert();
    }
  }, [insertAt, clearInsert]);

  const run = async (explain = false) => {
    setBusy(true); setErr("");
    const t0 = performance.now();
    try {
      const out = explain ? await api.explain(sql) as unknown as QueryResult : await api.query(sql);
      if (!explain) {
        setRes(out);
        useResult.getState().set(out);
        push({ sql, ms: out.elapsed_ms || Math.round(performance.now() - t0), rows: out.total_rows, scanned: out.scanned_rows, ok: true, at: new Date().toLocaleString("zh-CN", { hour12: false }) });
      } else {
        useResult.getState().setExplain(out.explain);
      }
    } catch (e) {
      const msg = (e as Error).message;
      setErr(msg);
      push({ sql, ms: 0, rows: 0, scanned: 0, ok: false, at: new Date().toLocaleString("zh-CN", { hour12: false }) });
      toast(msg);
    } finally { setBusy(false); }
  };

  const onMount: OnMount = (ed, monaco) => {
    ref.current = ed;
    ed.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => { void run(false); });
    monaco.languages.registerCompletionItemProvider("sql", {
      provideCompletionItems: (model, position) => {
        const word = model.getWordUntilPosition(position);
        const range = { startLineNumber: position.lineNumber, endLineNumber: position.lineNumber, startColumn: word.startColumn, endColumn: word.endColumn };
        const kw = ["SELECT", "FROM", "WHERE", "GROUP BY", "HAVING", "ORDER BY", "LIMIT", "COUNT", "AVG", "SUM", "MIN", "MAX", "DISTINCT", "EXPLAIN"];
        const tablesItems = tables.flatMap((t) => [{ label: t.table, kind: monaco.languages.CompletionItemKind.Class, insertText: t.table, range }, ...t.columns.map((c) => ({ label: `${t.table}.${c.name}`, kind: monaco.languages.CompletionItemKind.Field, insertText: c.name, range }))]);
        return { suggestions: [...kw.map((k) => ({ label: k, kind: monaco.languages.CompletionItemKind.Keyword, insertText: k, range })), ...tablesItems] };
      },
    });
  };

  return (
    <div className="h-full flex flex-col bg-panel">
      <div className="h-9 flex items-center px-3 gap-2 border-b border-line text-xs">
        <button disabled={busy} onClick={() => run(false)} className="bg-amber text-bg font-semibold px-3 py-1 rounded disabled:opacity-40 flex items-center gap-1">
          <Play size={12} /> 执行 ⌘↵
        </button>
        <button onClick={() => run(true)} className="border border-line px-2 py-1 rounded hover:border-cyan">EXPLAIN</button>
        {busy && <span className="text-amber animate-pulse">running…</span>}
        {res && <span className="text-mute font-mono">{res.elapsed_ms}ms · scan {res.scanned_rows} · out {res.total_rows}</span>}
        <div className="flex-1" />
        <select className="bg-panel2 border border-line text-[11px] py-1" onChange={(e) => { if (e.target.value) setSql(e.target.value); }}>
          <option value="">历史查询</option>
          {hist.map((h, i) => <option key={i} value={h.sql}>{h.ok ? "✓" : "✗"} {h.sql.slice(0, 48)}</option>)}
        </select>
      </div>
      <div className="flex-1 min-h-0">
        <Editor language="sql" theme="vs-dark" value={sql} onChange={(v) => setSql(v || "")} onMount={onMount}
          options={{ fontFamily: "IBM Plex Mono", fontSize: 13, minimap: { enabled: false }, automaticLayout: true }} />
      </div>
      {err && <div className="text-rose text-xs px-3 py-2 border-t border-line font-mono">{err}</div>}
    </div>
  );
}

type RState = { data: QueryResult | null; explain: ExplainNode[]; set: (d: QueryResult) => void; setExplain: (e: ExplainNode[]) => void };
const useResult = ((() => {
  let s: RState;
  const listeners = new Set<() => void>();
  const apiStore = {
    getState: () => s,
    setState: (p: Partial<RState>) => { s = { ...s, ...p }; listeners.forEach((l) => l()); },
    subscribe: (l: () => void) => { listeners.add(l); return () => listeners.delete(l); },
  };
  s = {
    data: null, explain: [],
    set: (data) => apiStore.setState({ data, explain: data.explain || [] }),
    setExplain: (explain) => apiStore.setState({ explain }),
  };
  return {
    getState: apiStore.getState,
    setState: apiStore.setState,
    use: <T,>(sel: (st: RState) => T) => {
      const [v, set] = useState(sel(s));
      useEffect(() => apiStore.subscribe(() => set(sel(s))), [sel]);
      return v;
    },
  };
})());

function ResultPane() {
  const data = useResult.use((s) => s.data);
  const explain = useResult.use((s) => s.explain);
  if (!data) {
    return (
      <div className="h-full flex items-center justify-center text-mute text-sm bg-panel">
        <div className="text-center">
          <Activity className="mx-auto mb-2 text-cyan" />
          尚未执行查询。在上方写下 SELECT 后按 ⌘/Ctrl+Enter。
        </div>
      </div>
    );
  }
  return (
    <div className="h-full flex flex-col bg-panel">
      <div className="h-8 px-3 flex items-center gap-3 text-xs border-b border-line">
        <span className="font-mono text-mute">{data.total_rows} rows</span>
        <a className="text-cyan hover:underline" href={api.exportURL(data.result_set_id, "csv")}>导出 CSV</a>
        <a className="text-cyan hover:underline" href={api.exportURL(data.result_set_id, "json")}>导出 JSON</a>
        {explain[0] && <span className="text-mute">plan: {flattenPlan(explain[0])}</span>}
      </div>
      <VirtualGrid data={data} />
    </div>
  );
}

function flattenPlan(n: ExplainNode): string {
  return n.op + (n.children?.[0] ? " ← " + flattenPlan(n.children[0]) : "");
}

function VirtualGrid({ data }: { data: QueryResult }) {
  const parent = useRef<HTMLDivElement>(null);
  const [pages, setPages] = useState<Record<number, QueryResult["rows"]>>({ 0: data.rows });
  const pageSize = data.rows.length || 1000;
  const rowH = 28;
  const virt = useVirtualizer({ count: data.total_rows, getScrollElement: () => parent.current, estimateSize: () => rowH, overscan: 16 });
  useEffect(() => { setPages({ 0: data.rows }); }, [data.result_set_id, data.rows]);

  const visible = virt.getVirtualItems();
  useEffect(() => {
    const need = new Set<number>();
    for (const v of visible) need.add(Math.floor(v.index / pageSize));
    need.forEach((p) => {
      if (pages[p]) return;
      void api.page(data.result_set_id, p * pageSize, pageSize).then((r) => setPages((old) => ({ ...old, [p]: r.rows }))).catch(() => undefined);
    });
  }, [visible, data.result_set_id, pageSize, pages]);

  const cell = (i: number) => {
    const p = Math.floor(i / pageSize);
    const rows = pages[p];
    return rows?.[i - p * pageSize];
  };

  if (data.total_rows === 0) {
    return <div className="flex-1 flex items-center justify-center text-mute text-sm">查询成功，但结果为 0 行。</div>;
  }

  return (
    <div ref={parent} className="flex-1 overflow-auto font-mono text-[12px]">
      <div className="sticky top-0 z-10 flex bg-panel2 border-b border-line">
        {data.schema.map((c) => (
          <div key={c.name} className="min-w-[140px] px-2 py-1 text-cyan border-r border-line">
            {c.name}<span className="text-mute ml-1">{c.type}</span>
          </div>
        ))}
      </div>
      <div style={{ height: virt.getTotalSize(), position: "relative" }}>
        {virt.getVirtualItems().map((v) => {
          const row = cell(v.index);
          return (
            <div key={v.key} className="absolute left-0 flex border-b border-line/60" style={{ top: v.start, height: v.size, width: "100%" }}>
              {!row && <div className="px-3 text-mute animate-pulse">loading…</div>}
              {row && row.map((val, i) => (
                <div key={i} className="min-w-[140px] px-2 py-1 truncate border-r border-line/40" title={val === null ? "NULL" : String(val)}>
                  {val === null ? <span className="text-mute/50">∅</span> : String(val)}
                </div>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function StatusBar({ stats }: { stats?: Record<string, unknown> }) {
  const mem = Number(stats?.tables_mem_bytes || 0);
  const lim = Number(stats?.budget_limit || 1);
  return (
    <footer className="h-7 border-t border-line px-3 flex items-center gap-4 text-[11px] font-mono text-mute">
      <span>heap {fmt(Number(stats?.go_heap_alloc || 0))}</span>
      <span>tables {String(stats?.table_count || 0)}</span>
      <span>rows {String(stats?.total_rows || 0)}</span>
      <span>compress {Number(stats?.global_compression || 1).toFixed(2)}</span>
      <span>budget {(mem / lim * 100).toFixed(1)}%</span>
      <span className="ml-auto">{String(stats?.time || "")}</span>
    </footer>
  );
}

function fmt(n: number) {
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + "MB";
  if (n > 1024) return (n / 1024).toFixed(1) + "KB";
  return n + "B";
}

void useMemo;
