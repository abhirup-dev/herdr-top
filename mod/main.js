// @ts-check
// herdr-top for the Herdr desktop app: a dialog with one row per coding agent
// (CPU%, physical footprint, RSS, age of its whole process tree), collected by
// the Go `herdr-top` binary (`herdr-top --json --hc`) and joined to the live
// agents list. Keys: ↑/↓ or j/k move, m/c/o sort, r refresh, p pause,
// Enter (or double-click) goes to the agent's terminal and closes, q/Esc close.
import { View, div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { agents, exec, run, theme, props, closeOverlay, settings } from "herdr";

const INTERVAL_MS = 2000;
const ROW_H = 22;
const K = 1024;

/** @typedef {{ value?: number, valid: boolean, error?: string }} Metric */
/** @typedef {{ pid: number, kind: string, age_ns: number, started_at: string,
 *   tree_cpu_percent: Metric, tree_rss_bytes: Metric, tree_physical_footprint_bytes: Metric,
 *   cpu_percent: Metric, rss_bytes: Metric, physical_footprint_bytes: Metric,
 *   location?: { session: string, workspace?: string, pane_label?: string, pane_title?: string, state?: string },
 *   diagnostics?: string[] }} Row */
/** @typedef {{ generated_at: string, records: Row[], diagnostics?: string[] }} Snapshot */

const bytes = (/** @type {Metric} */ v) =>
  !v.valid ? "—" : v.value >= K * K * K ? (v.value / (K * K * K)).toFixed(1) + "G" : v.value >= K * K ? Math.round(v.value / (K * K)) + "M" : v.value >= K ? Math.round(v.value / K) + "K" : v.value + "B";
const cpu = (/** @type {Metric} */ v) => (v.valid ? (v.value ?? 0).toFixed(1) : "—");
const age = (/** @type {number} */ ns) => {
  const m = Math.floor(ns / 6e10);
  if (ns < 0) return "—";
  if (m >= 1440) return `${Math.floor(m / 1440)}d ${Math.floor(m / 60) % 24}h`;
  if (m >= 60) return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
  return `${m}m`;
};
const identity = (/** @type {Row} */ r) => `${r.pid}@${r.started_at}`;

// Descending by a metric; unavailable values last. Age: oldest first.
const SORTS = {
  memory: (/** @type {Row} */ a, /** @type {Row} */ b) => metricLess(a.tree_physical_footprint_bytes, b.tree_physical_footprint_bytes),
  cpu: (/** @type {Row} */ a, /** @type {Row} */ b) => metricLess(a.tree_cpu_percent, b.tree_cpu_percent),
  age: (/** @type {Row} */ a, /** @type {Row} */ b) => b.age_ns - a.age_ns,
};
function metricLess(/** @type {Metric} */ a, /** @type {Metric} */ b) {
  if (a.valid !== b.valid) return a.valid ? -1 : 1;
  return (b.value ?? 0) - (a.value ?? 0);
}

const COLS = [
  { label: "PID", w: 58 },
  { label: "KIND", w: 70 },
  { label: "CPU%", w: 56, sort: "cpu" },
  { label: "FOOT", w: 62, sort: "memory" },
  { label: "RSS", w: 62 },
  { label: "AGE", w: 64, sort: "age" },
  { label: "TERM", w: 52 },
  { label: "STATE", w: 70 },
  { label: "SPACE / TAB", w: 170 },
  { label: "NAME", w: 0 },
];

export default class HerdrTop extends View {
  init(_props, cx) {
    /** @type {Snapshot | null} */
    this.snapshot = null;
    this.sort = "memory";
    this.prevSort = "cpu";
    this.selected = null; // identity of the selected row
    this.top = 0;
    this.paused = false;
    this.busy = false;
    this.runs = 0;
    this.error = null;
    this.message = null;
    this.size = { width: 0, height: 0 };
    this.handle = cx.focus_handle();
    this.handle.focus();
    cx.timer.after(600, () => this.handle.focus());
    cx.spawn(async (cx) => {
      await this.collect(cx);
      for (;;) {
        await cx.sleep(INTERVAL_MS);
        if (!this.paused) await this.collect(cx);
      }
    });
  }

  async collect(cx) {
    if (this.busy) return;
    this.busy = true;
    cx.notify();
    try {
      const r = await exec("herdr-top", ["--json", "--hc", "--sample", "500ms"]);
      if (r.code !== 0) throw new Error(r.stderr.trim() || `herdr-top exited ${r.code}`);
      this.snapshot = JSON.parse(r.stdout);
      this.error = null;
      this.runs++;
    } catch (e) {
      this.error = /not found|No such file|cannot start/i.test(String(e)) ? "herdr-top is not on PATH: run `make install GOBIN=~/.local/bin`" : (e instanceof Error ? e.message : String(e));
    }
    this.busy = false;
    cx.notify();
  }

  /** Rows in the current sort. */
  rows() {
    return [...(this.snapshot?.records ?? [])].sort(SORTS[this.sort]);
  }

  go(/** @type {Row | undefined} */ r) {
    const addr = r?.location?.session;
    if (!addr) {
      this.message = { text: r ? "not in Herdr: nothing to focus" : "no row selected", failed: true };
      return;
    }
    if (!agents().some((a) => a.address === addr)) {
      this.message = { text: `${addr} is no longer an agent in Herdr`, failed: true };
      return;
    }
    run("agent.focus", { terminal: addr });
    this.close();
  }

  close() {
    const id = props()?.overlay;
    if (id) closeOverlay(id);
  }

  setSort(/** @type {string} */ s) {
    if (s === this.sort) return;
    this.prevSort = this.sort;
    this.sort = s;
  }

  key(e, cx, rows, at) {
    const k = e.key;
    const move = (d) => {
      if (rows.length) this.selected = identity(rows[Math.max(0, Math.min(rows.length - 1, at + d))]);
    };
    if (k === "escape" || k === "q") return this.close();
    else if (k === "down" || k === "j") move(1);
    else if (k === "up" || k === "k") move(-1);
    else if (k === "m") this.setSort("memory");
    else if (k === "c") this.setSort("cpu");
    else if (k === "o") this.setSort("age");
    else if (k === "r") { this.message = null; cx.spawn((cx) => this.collect(cx)); }
    else if (k === "p") this.paused = !this.paused;
    else if (k === "enter") this.go(rows[at]);
    else return;
    cx.stop_propagation();
    cx.notify();
  }

  render(cx) {
    // theme() is typed as plain strings; gpui-kit wants `#rrggbb` literals.
    const t = /** @type {Record<string, `#${string}`>} */ (theme());
    const live = Object.fromEntries(agents().map((a) => [a.address, a]));
    const rows = this.rows();
    let at = rows.findIndex((r) => identity(r) === this.selected);
    if (at < 0 && rows.length) { at = 0; this.selected = identity(rows[0]); }
    const visible = Math.max(1, Math.floor((this.size.height - 110) / ROW_H));
    if (at >= 0) this.top = Math.min(Math.max(this.top, at - visible + 1), at);
    this.top = Math.max(0, Math.min(this.top, Math.max(0, rows.length - visible)));
    const shown = rows.slice(this.top, this.top + visible);
    const mono = String(settings().font_family ?? "JetBrains Mono");

    const heat = (/** @type {Metric} */ v, warm, hot) => (!v.valid ? t.dim : v.value >= hot ? t.blocked : v.value >= warm ? t.working : t.idle);
    const cell = (text, w, color, extra = (d) => d) =>
      extra(div().flex_none().truncate().text_color(color).child(text)).when(w > 0, (d) => d.w(w)).when(w === 0, (d) => d.flex_1());

    const head = h_flex().h(ROW_H).px(10).gap(8).items_center().border_b(1).border_color(t.border).text_size(11).font_semibold().text_color(t.dim).children(
      COLS.map((c) => {
        const on = c.sort && c.sort === this.sort;
        const d = cell(on ? `${c.label} ▼` : c.label, c.w, on ? t.foreground : t.dim);
        return c.sort ? d.id(`sort-${c.sort}`).cursor_pointer().hover((s) => s.text_color(t.foreground)).on_click((_e, cx) => { this.setSort(c.sort); cx.notify(); }) : d;
      }),
    );

    const row = (/** @type {Row} */ r) => {
      const on = identity(r) === this.selected;
      const a = r.location ? live[r.location.session] : null;
      const state = a?.status ?? r.location?.state ?? "";
      const where = a ? `${a.space_name} / ${a.tab_name}` : r.location?.workspace ?? (r.location ? "" : "outside");
      const label = r.location?.pane_label ?? "";
      const name = label || a?.title || r.location?.pane_title || "";
      return h_flex()
        .id(`row-${r.pid}`)
        .h(ROW_H).px(10).gap(8).items_center().rounded(4)
        .when(on, (d) => d.bg(t.selected))
        .hover((s) => s.bg(on ? t.selected : t.hover))
        .cursor_pointer()
        .on_click((e, cx) => { this.selected = identity(r); if (e.click_count >= 2) this.go(r); cx.notify(); })
        .children([
          cell(String(r.pid), COLS[0].w, t.dim),
          cell(r.kind, COLS[1].w, t.accent),
          cell(cpu(r.tree_cpu_percent), COLS[2].w, heat(r.tree_cpu_percent, 25, 75), (d) => d.when(r.tree_cpu_percent.valid && r.tree_cpu_percent.value >= 75, (d) => d.font_semibold())),
          cell(bytes(r.tree_physical_footprint_bytes), COLS[3].w, heat(r.tree_physical_footprint_bytes, 256 * K * K, K * K * K)),
          cell(bytes(r.tree_rss_bytes), COLS[4].w, heat(r.tree_rss_bytes, 256 * K * K, K * K * K)),
          cell(age(r.age_ns), COLS[5].w, t.dim),
          cell(r.location?.session ?? "—", COLS[6].w, r.location ? t.foreground : t.dim),
          cell(state || "—", COLS[7].w, t[state] ?? t.dim),
          cell(where, COLS[8].w, r.location ? t.muted : t.dim),
          cell(name, COLS[9].w, label ? t.accent : t.muted, (d) => d.when(Boolean(label), (d) => d.font_semibold())),
        ]);
    };

    const sel = rows[at];
    const diag = sel?.diagnostics?.length ? `pid ${sel.pid}: ${sel.diagnostics.join("; ")}` : null;
    const warn = this.snapshot?.diagnostics?.length ? `${this.snapshot.diagnostics.length} warning(s): ${this.snapshot.diagnostics[0]}` : null;
    const footer = this.message ? { text: this.message.text, color: this.message.failed ? t.blocked : t.done }
      : this.error ? { text: this.error, color: t.blocked }
      : diag ? { text: diag, color: t.working }
      : warn ? { text: warn, color: t.dim }
      : { text: rows.length ? `row ${at + 1}/${rows.length}` : this.busy ? "collecting…" : "no agents found", color: t.dim };
    const watch = this.paused ? "paused" : this.busy ? "refreshing…" : `live ${INTERVAL_MS / 1000}s`;
    const inHerdr = rows.filter((r) => r.location).length;

    return v_flex()
      .id("root")
      .track_focus(this.handle)
      .on_key_down((e, cx) => this.key(e, cx, rows, at))
      .on_size_change((size, cx) => { this.size = size; cx.notify(); })
      .size_full().min_h(0).p(12).gap(6).bg(t.popover).text_size(12)
      .child(
        h_flex().items_center().gap(12)
          .child(div().text_size(15).font_semibold().text_color(t.foreground).child("herdr-top"))
          .child(div().text_color(t.muted).child(`${rows.length} agents (${inHerdr} in Herdr)  ·  sort: ${this.sort}  ·  ${watch}${this.runs ? `  ·  #${this.runs}` : ""}`))
          .child(div().flex_1())
          .child(div().text_size(11).text_color(t.dim).child("↑↓ move  m/c/o sort  r refresh  p pause  ⏎ go to  esc close")),
      )
      .child(v_flex().flex_1().min_h(0).font_family(mono).child(head).children(shown.map(row)))
      .child(h_flex().h(20).items_center().text_size(11).child(div().truncate().text_color(footer.color).child(footer.text)));
  }
}
