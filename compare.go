package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// The Tokyo baseline from the other side: what 1ms.dog's server in Tokyo measured (baseline.go writes it, the site
// serves it as https://1ms.dog/baseline.json). Two uses:
//   httpslatency --compare   your run, each service with the Tokyo server's 24-hour median beside it
//   httpslatency status      every service as seen from Tokyo: now, the 24-hour median, uptime, recent outages
// Only the summary is downloaded; nothing about the run is sent.

var baselineURL = "https://1ms.dog/baseline.json"

// tokyo is set when a run is compared (--compare)
var tokyo *summaryFile

func loadBaseline() (*summaryFile, error) {
	u := baselineURL
	if e := os.Getenv("HTTPSLATENCY_BASELINE_URL"); e != "" { // a local copy, for testing
		u = e
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", userAgent())
	res, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", shortErr(err))
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var s summaryFile
	if err := json.Unmarshal(b, &s); err != nil || s.V != 1 || len(s.Services) == 0 {
		return nil, fmt.Errorf("no data yet")
	}
	return &s, nil
}

// the Tokyo number for a service: its 24-hour median, or the latest run when there is no median yet; -1 when none
func tokyoMS(s *svcSummary) float64 {
	if s == nil {
		return -1
	}
	if s.Med24 != nil {
		return *s.Med24
	}
	if s.Last != nil && s.Last.Err == "" {
		return s.Last.Ms
	}
	return -1
}

func localTime(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// stale: no run for three intervals
func (s *summaryFile) stale() bool {
	every := time.Duration(max(s.Every, 60)) * time.Second
	return time.Since(s.At) > 3*every
}

// the --compare column: the Tokyo median and how far this run is from it
func tokyoCell(r *result) string {
	if tokyo == nil {
		return ""
	}
	tm := tokyoMS(tokyo.Services[r.s.ID])
	if tm < 0 {
		return paint(cDim, padL("–", 8))
	}
	cell := paint(cDim, padL(fmtMS(tm)+" ms", 8))
	if m := r.median(); m >= 0 {
		d := m - tm
		sign, c := "+", cDim
		if d < 0 {
			sign, d = "−", -d
		}
		if m > tm*2 && m-tm > 50 {
			c = cYellow // more than twice the Tokyo time, and 50 ms more: the route from here
		}
		cell += " " + paint(c, sign+fmtMS(d))
	}
	return cell
}

func statusMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	cat := fs.String("c", "", "")
	fs.StringVar(cat, "cat", "", "")
	lf := fs.String("lang", "", "")
	noColor := fs.Bool("no-color", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "Usage: httpslatency status [-c GROUPS] [--json] [--lang CODE] [--no-color]")
		return 2
	}
	if *lf != "" {
		if lang = knownLang(*lf); lang == "" {
			fmt.Fprintf(stderr, "httpslatency: unknown language %q (en, zh-CN, zh-TW, ja, ko, es, fr, de, pt, ru)\n", *lf)
			return 2
		}
	}
	useColor = !*noColor && os.Getenv("NO_COLOR") == "" && isTTY(stdout) && enableVT()
	s, err := loadBaseline()
	if err != nil {
		fmt.Fprintln(stderr, "httpslatency: "+T("stErr", err.Error()))
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(s)
		return 0
	}
	want := splitList(*cat)
	printStatus(stdout, s, want)
	return 0
}

// printStatus: the groups in the page's order, then each region's local favorites
func printStatus(w io.Writer, s *summaryFile, want []string) {
	fmt.Fprintf(w, "\n%s\n", paint(cBold, T("stHead", localTime(s.At))))
	if s.stale() {
		fmt.Fprintln(w, paint(cYellow, T("stStale", localTime(s.At))))
	}
	fmt.Fprintln(w, paint(cDim, T("stCols")))
	groups := map[string][]string{}
	for id, x := range s.Services {
		groups[x.Cat] = append(groups[x.Cat], id)
	}
	var order []string
	for _, c := range cats[1:] {
		order = append(order, c)
	}
	var packs []string
	for g := range groups {
		if strings.HasPrefix(g, "pack:") {
			packs = append(packs, g)
		}
	}
	sort.Strings(packs)
	order = append(order, packs...)
	nameW := 12
	for _, x := range s.Services {
		nameW = max(nameW, width(x.Name))
	}
	nameW = min(nameW, 22)
	pct := func(v *float64) string {
		if v == nil {
			return paint(cDim, padL("–", 6))
		}
		c := cGreen
		if *v < 99.5 {
			c = cYellow
		}
		if *v < 95 {
			c = cRed
		}
		return paint(c, padL(fmt.Sprintf("%.1f%%", *v), 6))
	}
	type out struct {
		name string
		o    outage
	}
	var outs []out
	for _, g := range order {
		ids := groups[g]
		if len(ids) == 0 {
			continue
		}
		if len(want) > 0 && !contains(want, g) && !(strings.HasPrefix(g, "pack:") && (contains(want, "local") || contains(want, g[5:]))) {
			continue
		}
		sort.Slice(ids, func(i, j int) bool {
			return strings.ToLower(s.Services[ids[i]].Name) < strings.ToLower(s.Services[ids[j]].Name)
		})
		label := T("cat_" + g)
		if strings.HasPrefix(g, "pack:") {
			label = catName("local", g[5:])
		}
		fmt.Fprintf(w, "\n  %s\n", paint(cCyan, label))
		for _, id := range ids {
			x := s.Services[id]
			now := -1.0
			if x.Last != nil && x.Last.Err == "" {
				now = x.Last.Ms
			}
			t := tierOf(now)
			ms := "✕   "
			if now >= 0 {
				ms = fmtMS(now) + " ms"
			}
			med := -1.0
			if x.Med24 != nil {
				med = *x.Med24
			}
			fmt.Fprintf(w, "    %s %s  %s  %s  %s %s\n", padR(x.Name, nameW), padL(paint(tierColor[t], ms), 8), lane(med),
				paint(cDim, padL(fmtMS(med), 5)), pct(x.Up24), pct(x.Up7))
			for _, o := range x.Out {
				outs = append(outs, out{x.Name, o})
			}
		}
	}
	fmt.Fprintln(w)
	if len(outs) == 0 {
		fmt.Fprintln(w, paint(cCyan, "›")+" "+T("stNone"))
		return
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].o.To > outs[j].o.To })
	fmt.Fprintln(w, paint(cCyan, "›")+" "+T("stOut"))
	for i, x := range outs {
		if i == 10 {
			break
		}
		from, to := time.Unix(x.o.From, 0), time.Unix(x.o.To, 0)
		when := localTime(from)
		if x.o.To != x.o.From {
			when += " – " + to.Local().Format("15:04")
		}
		fmt.Fprintf(w, "    %s %s  %s\n", padR(x.name, nameW), paint(cDim, when), paint(cYellow, x.o.Err))
	}
}
