import { create } from "zustand";

export type Hist = { sql: string; ms: number; rows: number; scanned: number; ok: boolean; at: string };

type S = {
  sql: string;
  setSql: (s: string) => void;
  insertAt: string | null;
  requestInsert: (s: string) => void;
  clearInsert: () => void;
  history: Hist[];
  pushHist: (h: Hist) => void;
  toast: string;
  setToast: (s: string) => void;
};

export const useWS = create<S>((set, get) => ({
  sql: "SELECT city, COUNT(*) AS c, AVG(age) FROM users WHERE age > 18 GROUP BY city ORDER BY c DESC LIMIT 100",
  setSql: (sql) => set({ sql }),
  insertAt: null,
  requestInsert: (s) => set({ insertAt: s }),
  clearInsert: () => set({ insertAt: null }),
  history: JSON.parse(localStorage.getItem("mdl.hist") || "[]"),
  pushHist: (h) => {
    const history = [h, ...get().history].slice(0, 40);
    localStorage.setItem("mdl.hist", JSON.stringify(history));
    set({ history });
  },
  toast: "",
  setToast: (toast) => {
    set({ toast });
    if (toast) setTimeout(() => set({ toast: "" }), 5000);
  },
}));
