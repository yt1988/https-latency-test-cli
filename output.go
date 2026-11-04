package main

import (
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"
)

var useColor bool

const (
	cReset   = "\x1b[0m"
	cBold    = "\x1b[1m"
	cDim     = "\x1b[2m"
	cRed     = "\x1b[31m"
	cGreen   = "\x1b[32m"
	cYellow  = "\x1b[33m"
	cCyan    = "\x1b[36m"
	cMagenta = "\x1b[35m"
)

func paint(code, s string) string {
	if !useColor || s == "" {
		return s
	}
	return code + s + cReset
}

// width is how many columns s takes in a terminal: CJK and fullwidth characters take two.
func width(s string) int {
	w, esc := 0, false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc:
			esc = r != 'm'
		case r >= 0x1100 && r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf, r >= 0xac00 && r <= 0xd7a3,
			r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe4f, r >= 0xff00 && r <= 0xff60, r >= 0xffe0 && r <= 0xffe6:
			w += 2
		default:
			w++
		}
	}
	return w
}

func padR(s string, n int) string { return s + strings.Repeat(" ", max(0, n-width(s))) }
func padL(s string, n int) string { return strings.Repeat(" ", max(0, n-width(s))) + s }

// fmtMS: one decimal below 10 ms, whole milliseconds above, – for nothing.
func fmtMS(v float64) string {
	if v < 0 {
		return "–"
	}
	if v < 10 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.0f", math.Round(v))
}

// each grade's color, as on the page's field: green when fast, through yellow, to red when napping
var tierColor = map[string]string{"legend": cMagenta, "zoom": cGreen, "sprint": cGreen, "trot": cYellow, "stroll": cYellow, "nap": cRed, "lost": cRed}

// The field: a lane per service on the page's log scale (5 ms to 1 s), the ball where its median landed.
const laneW = 20

func lane(ms float64) string {
	if ms < 0 {
		return paint(cDim, strings.Repeat("·", laneW))
	}
	p := math.Log(ms/5) / math.Log(1000.0/5)
	x := int(math.Round(math.Max(0, math.Min(1, p)) * float64(laneW-1)))
	c := tierColor[tierOf(ms)]
	return paint(c, strings.Repeat("━", x)) + paint(c+cBold, "●") + paint(cDim, strings.Repeat("·", laneW-1-x))
}

func catName(c string, pack string) string {
	if c == "local" && pack != "" {
		return T("cat_local") + " · " + T("pk_"+pack)
	}
	return T("cat_" + c)
}

// summary is what the page shows above the field: the two segments, the verdict and the everyday uses.
type summary struct {
	Local, Global, All segment
	Key, Mood          string
	Uses               []useLine
	Lost               []string
}

type useLine struct {
	Use    string  `json:"use"`
	Status string  `json:"status"`
	MS     float64 `json:"ms"`
}

func summarize(l *list, rs []*result) summary {
	var local, global []*result
	byID := map[string]*result{}
	for _, r := range rs {
		byID[r.s.ID] = r
		if r.cat == "local" {
			local = append(local, r)
		} else {
			global = append(global, r)
		}
	}
	s := summary{Local: segmentOf(local), Global: segmentOf(global), All: segmentOf(rs)}
	s.Key, s.Mood = diagnose(s.Local, s.Global)
	for _, u := range l.Uses {
		var in []*result
		for _, id := range u.IDs {
			if r := byID[id]; r != nil {
				in = append(in, r)
			}
		}
		if len(in) == 0 {
			continue
		}
		sg := segmentOf(in)
		s.Uses = append(s.Uses, useLine{u.Use, useStatus(sg, u.Good, u.OK), sg.MD})
	}
	s.Lost = s.All.Lost
	return s
}

var statusColor = map[string]string{"great": cGreen, "ok": cYellow, "slow": cRed, "blocked": cRed}

func printResult(w io.Writer, l *list, rs []*result, o runOpts, from *Origin, src, link string, verbose bool, took time.Duration) {
	fmt.Fprintf(w, "\n%s\n", paint(cBold, head(len(rs), o.rounds)))
	fmt.Fprintln(w, paint(cDim, T("from")+" "+from.String()+famNote(o.family)))
	nameW := 12
	for _, r := range rs {
		nameW = max(nameW, width(r.name))
	}
	nameW = min(nameW, 22)
	cat := ""
	for _, r := range rs {
		if r.cat != cat {
			cat = r.cat
			fmt.Fprintf(w, "\n  %s\n", paint(cCyan, catName(cat, o.pack)))
		}
		m := r.median()
		t := tierOf(m)
		ms := "✕   "
		if m >= 0 {
			ms = fmtMS(m) + " ms"
		}
		pop := ""
		if r.pop != "" {
			pop = paint(cDim, r.pop)
		}
		fmt.Fprintf(w, "    %s %s  %s  %s %s\n", padR(r.name, nameW), padL(paint(tierColor[t], ms), 8), lane(m), padR(paint(tierColor[t], T("tier_"+t)), 10), pop)
		if verbose {
			fmt.Fprintln(w, paint(cDim, "      "+detail(r)))
		}
	}

	s := summarize(l, rs)
	fmt.Fprintln(w)
	var segs []string
	for _, x := range []struct {
		k  string
		sg segment
	}{{"seg_local", s.Local}, {"seg_global", s.Global}, {"med", s.All}} {
		if x.sg.N == 0 {
			continue
		}
		t := tierOf(x.sg.MD)
		segs = append(segs, T(x.k)+" "+paint(tierColor[t]+cBold, fmtMS(x.sg.MD)+" ms"))
	}
	fmt.Fprintln(w, paint(cCyan, "›")+" "+strings.Join(segs, paint(cDim, "  ·  ")))
	if s.Local.N > 0 && s.Global.N > 0 {
		fmt.Fprintln(w, paint(cCyan, "›")+" "+paint(cBold, T("vh_"+s.Key))+T("dash")+T("v_"+s.Key))
	}
	if len(s.Uses) > 0 {
		var us []string
		for _, u := range s.Uses {
			us = append(us, T("use_"+u.Use)+" "+paint(statusColor[u.Status], T("st_"+u.Status)))
		}
		fmt.Fprintln(w, "  "+strings.Join(us, paint(cDim, "  ·  ")))
	}
	if len(s.Lost) > 0 {
		fmt.Fprintln(w, "  "+paint(cYellow, T("lost", strings.Join(s.Lost, T("join")))))
	}
	if link != "" {
		fmt.Fprintln(w, paint(cCyan, "›")+" "+T("link")+gap(T("link"))+paint(cCyan, link))
	}
	fmt.Fprintln(w, paint(cDim, T("took", took.Round(time.Second).String(), listName(src))))
}

func famNote(f string) string {
	if f == "" {
		return ""
	}
	return " · IPv" + f
}

// detail: the -v line under a service: address and protocol, the first request's handshakes, every sample, jitter
func detail(r *result) string {
	if r.cold < 0 {
		return T("noConn", r.err)
	}
	parts := []string{r.addr + " " + r.proto}
	if r.status > 0 {
		parts[0] += fmt.Sprintf(" %d", r.status)
	}
	parts = append(parts, fmt.Sprintf("%s %s · DNS %s · TCP %s · TLS %s", T("cold"), fmtMS(r.cold), fmtMS(r.dns), fmtMS(r.connect), fmtMS(r.tls)))
	var ss []string
	for _, v := range r.samples {
		if v < 0 {
			ss = append(ss, "✕")
		} else {
			ss = append(ss, fmtMS(v))
		}
	}
	parts = append(parts, T("samples")+" "+strings.Join(ss, " "))
	if j := jitter(r.samples); j >= 0 {
		parts = append(parts, T("jitter")+" ±"+fmtMS(j))
	}
	return strings.Join(parts, "  ·  ")
}

// listName says where the list came from: 1ms.dog, a file, or the copy built into the binary.
func listName(src string) string {
	if src == "built-in" || src == "offline" {
		return T("builtin")
	}
	return src
}

// gap is what goes between a label and its value: a space after "Saved:", nothing after a full-width colon ("已保存：").
func gap(label string) string {
	if strings.HasSuffix(label, "：") {
		return ""
	}
	return " "
}

// printMarkdown: the result as a Markdown table, to paste into a forum post or an issue.
func printMarkdown(w io.Writer, l *list, rs []*result, o runOpts, from *Origin, link string, took time.Duration) {
	fmt.Fprintf(w, "## %s\n\n", head(len(rs), o.rounds))
	fmt.Fprintf(w, "%s %s%s · %s\n\n", T("from"), from.String(), famNote(o.family), time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "| %s | %s | ms | %s | %s |\n|---|---|---:|---|---|\n", T("c_group"), T("c_service"), T("c_grade"), T("c_edge"))
	for _, r := range rs {
		m := r.median()
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", catName(r.cat, o.pack), r.name, fmtMS(m), T("tier_"+tierOf(m)), r.pop)
	}
	s := summarize(l, rs)
	fmt.Fprintf(w, "\n%s %s ms · %s %s ms · %s %s ms\n", T("seg_local"), fmtMS(s.Local.MD), T("seg_global"), fmtMS(s.Global.MD), T("med"), fmtMS(s.All.MD))
	if s.Local.N > 0 && s.Global.N > 0 {
		fmt.Fprintf(w, "\n**%s**%s%s\n", T("vh_"+s.Key), T("dash"), T("v_"+s.Key))
	}
	if link != "" {
		fmt.Fprintf(w, "\n%s%s%s\n", T("link"), gap(T("link")), link)
	}
	fmt.Fprintf(w, "\n%s\n", T("mdFoot", version))
}

// toJSON: everything measured, for scripts
func toJSON(l *list, rs []*result, o runOpts, from *Origin, src, link string, took time.Duration) map[string]any {
	num := func(v float64) any {
		if v < 0 {
			return nil
		}
		return v
	}
	var out []map[string]any
	for _, r := range rs {
		var ss []any
		for _, v := range r.samples {
			ss = append(ss, num(v))
		}
		j := map[string]any{"id": r.s.ID, "name": r.name, "group": r.cat, "url": r.s.URL, "median": num(r.median()), "grade": tierOf(r.median()),
			"samples": ss, "cold": num(r.cold), "dns": num(r.dns), "connect": num(r.connect), "tls": num(r.tls), "jitter": num(jitter(r.samples))}
		if r.proto != "" {
			j["protocol"], j["address"], j["status"] = r.proto, r.addr, r.status
		}
		if r.pop != "" {
			j["edge"] = r.pop
		}
		if r.cold < 0 && r.err != "" {
			j["error"] = r.err
		}
		out = append(out, j)
	}
	s := summarize(l, rs)
	seg := func(sg segment) map[string]any {
		return map[string]any{"services": sg.N, "answered": sg.OK, "median": num(sg.MD)}
	}
	fam := "auto"
	if o.family != "" {
		fam = "ipv" + o.family
	}
	return map[string]any{
		"version": version, "list": src, "from": from, "family": fam, "rounds": o.rounds, "pack": o.pack,
		"seconds": math.Round(took.Seconds()*10) / 10, "link": link,
		"summary": map[string]any{"local": seg(s.Local), "global": seg(s.Global), "all": seg(s.All), "verdict": s.Key, "uses": s.Uses},
		"results": out,
	}
}

// printList: every service the tool knows, by group
func printList(w io.Writer, l *list) {
	for _, c := range cats[1:] {
		fmt.Fprintln(w, paint(cCyan, T("cat_"+c))+paint(cDim, "  ("+c+")"))
		for _, s := range l.Services {
			if s.Cat == c {
				fmt.Fprintf(w, "  %s %s\n", padR(s.ID, 14), paint(cDim, s.Name+"  "+s.URL))
			}
		}
	}
	fmt.Fprintln(w, paint(cCyan, T("cat_local"))+paint(cDim, "  (--pack)"))
	for _, p := range packIDs(l) {
		var names []string
		for _, s := range l.Packs[p] {
			names = append(names, s.ID)
		}
		fmt.Fprintf(w, "  %s %s\n", padR(p, 14), paint(cDim, T("pk_"+p)+": "+strings.Join(names, ", ")))
	}
}

// packIDs in a fixed order (the page's)
func packIDs(l *list) []string {
	order := []string{"cn", "tw", "jp", "kr", "ru", "in", "sea", "eu", "latam", "na", "me"}
	var out []string
	for _, p := range order {
		if l.Packs[p] != nil {
			out = append(out, p)
		}
	}
	for p := range l.Packs {
		if !contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

/* ---------- progress ---------- */

type progLine struct {
	mu sync.Mutex
	w  io.Writer
}

func newProgLine(w io.Writer) *progLine { return &progLine{w: w} }

func (p *progLine) update(done, total int, r *result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pct := done * 100 / max(1, total)
	n := 24 * done / max(1, total)
	// the rest of the track is dim; without colors it is a thinner line, so the bar still shows how far it got
	rest := "━"
	if !useColor {
		rest = "─"
	}
	bar := paint(cCyan, strings.Repeat("━", n)) + paint(cDim, strings.Repeat(rest, 24-n))
	fmt.Fprintf(p.w, "\r\x1b[K  %s %3d%%  %s", bar, pct, paint(cDim, r.name))
}

func (p *progLine) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprint(p.w, "\r\x1b[K")
}

// head: "HTTPS latency to 40 services · 5 timed requests each", each count in its plural form
func head(services, rounds int) string {
	return TN("headSvc", services, services) + " · " + TN("headReq", rounds, rounds)
}
