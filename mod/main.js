// @ts-check
// herdr-top for the Herdr desktop app: a popup (like the picker) listing each
// agent in this window with its whole process tree's CPU%, physical footprint,
// RSS and age, collected by the Go `herdr-top` binary (`herdr-top --json --hc`)
// and joined to the live agents list. Type to filter, click a column to sort,
// ↑/↓ to move, Enter or a click goes to the agent's terminal, Esc closes.
import { View, div, svg, image } from "gpui-kit";
import { v_flex, h_flex, InputState, Input } from "gpui-base";
import { agents, exec, run, theme, props, closeOverlay, settings } from "herdr";

const INTERVAL_MS = 2000;
const K = 1024;

/** @typedef {{ value?: number, valid: boolean, error?: string }} Metric */
/** @typedef {{ pid: number, kind: string, age_ns: number, started_at: string,
 *   tree_cpu_percent: Metric, tree_rss_bytes: Metric, tree_physical_footprint_bytes: Metric,
 *   location?: { session: string }, diagnostics?: string[] }} Row */
/** @typedef {{ records: Row[], diagnostics?: string[] }} Snapshot */

const bytes = (/** @type {Metric} */ v) =>
  !v.valid ? "—" : v.value >= K * K * K ? (v.value / (K * K * K)).toFixed(1) + "G" : v.value >= K * K ? Math.round(v.value / (K * K)) + "M" : v.value >= K ? Math.round(v.value / K) + "K" : v.value + "B";
const cpu = (/** @type {Metric} */ v) => (v.valid ? (v.value ?? 0).toFixed(1) + "%" : "—");
const age = (/** @type {number} */ ns) => {
  const m = Math.floor(ns / 6e10);
  if (ns < 0) return "—";
  if (m >= 1440) return `${Math.floor(m / 1440)}d ${Math.floor(m / 60) % 24}h`;
  if (m >= 60) return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
  return m ? `${m}m` : "<1m";
};

// Descending by a metric, unavailable last; age: oldest first.
function metricLess(/** @type {Metric} */ a, /** @type {Metric} */ b) {
  if (a.valid !== b.valid) return a.valid ? -1 : 1;
  return (b.value ?? 0) - (a.value ?? 0);
}
const SORTS = {
  memory: (/** @type {Row} */ a, /** @type {Row} */ b) => metricLess(a.tree_physical_footprint_bytes, b.tree_physical_footprint_bytes),
  cpu: (/** @type {Row} */ a, /** @type {Row} */ b) => metricLess(a.tree_cpu_percent, b.tree_cpu_percent),
  rss: (/** @type {Row} */ a, /** @type {Row} */ b) => metricLess(a.tree_rss_bytes, b.tree_rss_bytes),
  age: (/** @type {Row} */ a, /** @type {Row} */ b) => b.age_ns - a.age_ns,
};
const COLS = [
  { label: "CPU", w: 70, sort: "cpu" },
  { label: "Footprint", w: 84, sort: "memory" },
  { label: "RSS", w: 70, sort: "rss" },
  { label: "Age", w: 70, sort: "age" },
];
const WORDS = { working: "working", blocked: "needs you", done: "finished", idle: "idle" };

function icon(p, size, color) {
  if (!p?.icon) return null;
  return p.tint ? svg(p.icon).size(size).flex_none().text_color(color) : image(p.icon).w(size).h(size).flex_none();
}

export default class HerdrTop extends View {
  init(_props, cx) {
    /** @type {Snapshot | null} */
    this.snapshot = null;
    this.sort = "memory";
    this.at = 0;
    this.busy = false;
    this.error = null;
    this.filter = "";
    this.input = InputState.new({ placeholder: "Filter agents by space, tab, program or terminal…" });
    this.input.on("change", (_e, cx) => { this.filter = this.input.value(); this.at = 0; cx.notify(); });
    this.input.on("submit", () => this.go(this.rows()[this.at]));
    this.input.focus();
    cx.timer.after(600, () => this.input.focus());
    this.handle = cx.focus_handle();
    cx.spawn(async (cx) => {
      for (;;) {
        await this.collect(cx);
        await cx.sleep(INTERVAL_MS);
      }
    });
  }

  async collect(cx) {
    if (this.busy) return;
    this.busy = true;
    try {
      const r = await exec("herdr-top", ["--json", "--hc", "--sample", "500ms"]);
      if (r.code !== 0) throw new Error(r.stderr.trim() || `herdr-top exited ${r.code}`);
      this.snapshot = JSON.parse(r.stdout);
      this.error = null;
    } catch (e) {
      this.error = /not found|No such file|cannot start/i.test(String(e)) ? "herdr-top is not on PATH: run `make install GOBIN=~/.local/bin`" : e instanceof Error ? e.message : String(e);
    }
    this.busy = false;
    cx.notify();
  }

  /** Agents in this window (joined to the live list), filtered and sorted. */
  rows() {
    const f = this.filter.toLowerCase();
    const live = Object.fromEntries(agents().map((a) => [a.address, a]));
    return (this.snapshot?.records ?? [])
      .map((r) => ({ r, a: r.location && live[r.location.session] }))
      .filter(({ a, r }) => a && (!f || [a.space_name, a.tab_name, a.program?.name, r.kind, a.address, a.title].join(" ").toLowerCase().includes(f)))
      .sort((x, y) => SORTS[this.sort](x.r, y.r));
  }

  go(row) {
    if (!row) return;
    this.close();
    run("agent.focus", { terminal: row.a.address });
  }

  close() {
    const id = props()?.overlay;
    if (id) closeOverlay(id);
  }

  glyph(t, st) {
    if (st === "working") return svg("mods/workspace-chrome/spinner.svg").size(12).flex_none().text_color(t.working).spin(900);
    return div().w(12).flex_none().text_color(t[st] ?? t.dim).child(st === "idle" ? "○" : st ? "●" : "·");
  }

  render() {
    // theme() is typed as plain strings; gpui-kit wants `#rrggbb` literals.
    const t = /** @type {Record<string, `#${string}`>} */ (theme());
    const rows = this.rows();
    this.at = Math.max(0, Math.min(this.at, rows.length - 1));
    const sel = rows[this.at];
    const mono = String(settings().font_family ?? "JetBrains Mono");
    const num = (text, w, color) => div().w(w).flex_none().text_right().font_family(mono).text_size(12).text_color(color).child(text);
    const heat = (/** @type {Metric} */ v, warm, hot) => (!v.valid ? t.dim : v.value >= hot ? t.blocked : v.value >= warm ? t.working : t.muted);

    const head = h_flex().mx(4).px(10).h(24).items_center().gap(10).text_size(11).text_color(t.dim)
      .child(div().w(12).flex_none())
      .child(div().flex_1().child("Agent"))
      .children(COLS.map((c) => {
        const on = c.sort === this.sort;
        return div().id(`sort-${c.sort}`).w(c.w).flex_none().text_right().cursor_pointer()
          .text_color(on ? t.foreground : t.dim).hover((s) => s.text_color(t.foreground))
          .on_click((_e, cx) => { this.sort = c.sort; cx.notify(); })
          .child(on ? `${c.label} ▾` : c.label);
      }));

    const row = ({ r, a }, i) => {
      const on = i === this.at;
      const pic = icon(a.program, 14, t.foreground);
      return h_flex()
        .id(`row-${a.address}`)
        .mx(4).px(10).h(32).rounded(7).gap(10).items_center().cursor_pointer()
        .when(on, (el) => el.bg(t.hover))
        .hover((s) => s.bg(t.hover))
        .on_click(() => this.go({ r, a }))
        .child(this.glyph(t, a.status))
        .when(Boolean(pic), (el) => el.child(pic))
        .child(div().truncate().text_color(t.foreground).when(a.focused, (d) => d.font_semibold()).child(a.space_name))
        .child(div().flex_1().truncate().text_color(t.dim).child(`${a.tab_name} · ${a.program?.name ?? r.kind} · ${a.address}`))
        .when(a.status === "blocked", (el) => el.child(div().text_size(11).text_color(t.blocked).child(WORDS.blocked)))
        .when(a.status === "done", (el) => el.child(div().text_size(11).text_color(t.done).child(WORDS.done)))
        .child(num(cpu(r.tree_cpu_percent), COLS[0].w, heat(r.tree_cpu_percent, 25, 75)))
        .child(num(bytes(r.tree_physical_footprint_bytes), COLS[1].w, heat(r.tree_physical_footprint_bytes, 256 * K * K, K * K * K)))
        .child(num(bytes(r.tree_rss_bytes), COLS[2].w, heat(r.tree_rss_bytes, 256 * K * K, K * K * K)))
        .child(num(age(r.age_ns), COLS[3].w, t.dim));
    };

    const note = this.error ? { text: this.error, color: t.blocked }
      : sel?.r.diagnostics?.length ? { text: sel.r.diagnostics.join("; "), color: t.working }
      : !this.snapshot ? { text: "collecting…", color: t.dim }
      : !rows.length ? { text: this.filter ? "no agent matches" : "no agents in this window", color: t.dim }
      : null;

    return v_flex()
      .id("herdr-top")
      .track_focus(this.handle)
      .w_full().min_h(0).p(18).gap(10).bg(t.popover).text_size(13)
      .on_key_down((e, cx) => {
        const k = e.key;
        if (k === "escape") return this.close();
        if (k === "down") this.at = (this.at + 1) % Math.max(1, rows.length);
        else if (k === "up") this.at = (this.at + rows.length - 1) % Math.max(1, rows.length);
        else return;
        cx.stop_propagation();
        cx.notify();
      })
      .child(h_flex().items_center()
        .child(div().flex_1().text_size(15).font_semibold().text_color(t.foreground).child("Agents"))
        .child(div().text_size(11).text_color(t.dim).child(`${rows.length} shown  ·  ${this.busy ? "refreshing…" : `every ${INTERVAL_MS / 1000}s`}`))
        .child(div().ml(12).text_size(11).text_color(t.dim).child("Esc")))
      .child(h_flex().h(36).flex_none().px(10).gap(8).items_center().rounded(9).border_1().border_color(t.accent).bg(t.background)
        .child(svg("mods/workspace-chrome/search.svg").size(15).text_color(t.dim))
        .child(Input.new(this.input).flex_1().h_full().text_size(12.8).text_color(t.foreground)))
      .child(head)
      .child(v_flex().h(Math.min(480, 6 + rows.length * 32)).min_h(0).flex_shrink(1).pb(6).overflow_y_scrollbar().children(rows.map(row)))
      .when(Boolean(note), (el) => el.child(div().px(14).text_size(11).truncate().text_color(note.color).child(note.text)));
  }
}
