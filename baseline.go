package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// `httpslatency baseline --out DIR` is what 1ms.dog's server in Tokyo runs every hour: every service the page
// knows (the global groups and every local pack), measured the same way as a normal run, written as static JSON that
// the site reads. It isn't in the usage text: it is for that one machine, but anyone can run it.
//
// Files in DIR, each written whole and then renamed into place, so a reader never sees half a file:
//   latest.json          the last run: every service's median, DNS, TCP, TLS and first-request times
//   summary.json         per service: hourly medians for 24 hours, availability over 24 hours and 7 days, the outages
//                        of the last 7 days; what the site's pages and `httpslatency status` show
//   days/YYYY-MM-DD.json every run of that day (UTC), kept for 30 days
// Nothing about anyone else is in them: only what this machine measured.

const (
	baselineEvery = time.Hour
	keepDays      = 30
	summaryDays   = 7
)

// one service in one run: ok with times, or the reason it didn't answer
type bsample struct {
	Ms    float64 `json:"ms"`              // median of the timed requests
	DNS   float64 `json:"dns"`             // the first request's parts (-1: not measured)
	TCP   float64 `json:"tcp"`             //
	TLS   float64 `json:"tls"`             //
	Cold  float64 `json:"cold"`            // the first request, handshakes included
	Pop   string  `json:"pop,omitempty"`   // the edge node that answered, as an airport code
	Proto string  `json:"proto,omitempty"` // h2 or http/1.1
	Err   string  `json:"err,omitempty"`   // set when it didn't answer
}

type latestFile struct {
	V        int                 `json:"v"`
	Tool     string              `json:"tool"`
	At       time.Time           `json:"at"`
	From     string              `json:"from"`
	TookMs   int64               `json:"took_ms"`
	Version  string              `json:"version"`
	Services map[string]*bsample `json:"services"`
}

// a day file stores runs compactly: per service [ms, dns, tcp, tls, cold] when it answered, or its error as a string
type dayRun struct {
	T int64                      `json:"t"` // unix seconds
	S map[string]json.RawMessage `json:"s"`
}

type dayFile struct {
	V    int      `json:"v"`
	Tool string   `json:"tool"`
	Day  string   `json:"day"`
	Runs []dayRun `json:"runs"`
}

type outage struct {
	From int64  `json:"from"` // unix seconds of the first failed run
	To   int64  `json:"to"`   // of the last failed run (still going when it is the latest run)
	Runs int    `json:"runs"`
	Err  string `json:"err"`
}

type svcSummary struct {
	Name  string     `json:"name"`
	Cat   string     `json:"cat"`           // a global group, or "pack:<id>" for a local favorite
	Last  *bsample   `json:"last"`          // the latest run
	H24   []*float64 `json:"h24"`           // median per hour, oldest first, null where it never answered
	Up24  *float64   `json:"up24"`          // percent of runs that answered
	Up7   *float64   `json:"up7"`           //
	Med24 *float64   `json:"med24"`         // median of the medians
	Med7  *float64   `json:"med7"`          //
	Out   []outage   `json:"out,omitempty"` // outages in the last 7 days, newest first
}

type summaryFile struct {
	V        int                    `json:"v"`
	Tool     string                 `json:"tool"`
	At       time.Time              `json:"at"`    // the latest run
	Every    int                    `json:"every"` // seconds between runs
	From     string                 `json:"from"`
	H24Start int64                  `json:"h24_start"` // unix seconds of h24[0]'s hour
	Runs24   int                    `json:"runs24"`
	Services map[string]*svcSummary `json:"services"`
}

func baselineMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "folder to write the JSON files to")
	from := fs.String("from", "Tokyo, JP", "where this runs, as shown on the pages")
	file := fs.String("services", "", "a local service list instead of 1ms.dog's")
	rounds := fs.Int("rounds", 3, "timed requests per service")
	conc := fs.Int("concurrency", 8, "services measured at once")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(stderr, "httpslatency baseline: --out DIR is required")
		return 2
	}
	l, _, err := loadList(*file, false)
	if err != nil {
		fmt.Fprintf(stderr, "httpslatency baseline: %v\n", err)
		return 1
	}
	rs := allTargets(l)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	t0 := time.Now().UTC()
	run(ctx, rs, runOpts{rounds: *rounds, timeout: 4 * time.Second, conc: *conc}, nil)
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "httpslatency baseline: the run took too long")
		return 1
	}
	if err := writeBaseline(*out, *from, l, rs, t0, time.Since(t0)); err != nil {
		fmt.Fprintf(stderr, "httpslatency baseline: %v\n", err)
		return 1
	}
	ok := 0
	for _, r := range rs {
		if r.median() >= 0 {
			ok++
		}
	}
	fmt.Fprintf(stdout, "baseline: %d/%d answered in %s\n", ok, len(rs), time.Since(t0).Round(time.Second))
	return 0
}

// allTargets: every service once, global groups first, then each pack's own (Telegram as the page's default)
func allTargets(l *list) []*result {
	var out []*result
	seen := map[string]bool{}
	for _, s := range l.Services {
		seen[s.ID] = true
		out = append(out, &result{s: s, name: s.Name, cat: s.Cat})
	}
	for _, p := range packIDs(l) {
		for _, s := range l.Packs[p] {
			if !seen[s.ID] {
				seen[s.ID] = true
				out = append(out, &result{s: s, name: s.Name, cat: "pack:" + p})
			}
		}
	}
	return out
}

func sampleOf(r *result) *bsample {
	ms := func(v float64) float64 {
		if v < 0 {
			return -1
		}
		return round1(v)
	}
	b := &bsample{Ms: r.median(), DNS: ms(r.dns), TCP: ms(r.connect), TLS: ms(r.tls), Cold: ms(r.cold), Pop: r.pop, Proto: r.proto}
	if b.Ms < 0 {
		b.Err = r.err
		if b.Err == "" {
			b.Err = "no answer"
		}
	}
	return b
}

func writeBaseline(dir, from string, l *list, rs []*result, at time.Time, took time.Duration) error {
	if err := os.MkdirAll(filepath.Join(dir, "days"), 0o755); err != nil {
		return err
	}
	latest := latestFile{V: 1, Tool: "index-baseline", At: at, From: from, TookMs: took.Milliseconds(), Version: version, Services: map[string]*bsample{}}
	run := dayRun{T: at.Unix(), S: map[string]json.RawMessage{}}
	for _, r := range rs {
		b := sampleOf(r)
		latest.Services[r.s.ID] = b
		var raw []byte
		if b.Err != "" {
			raw, _ = json.Marshal(b.Err)
		} else {
			raw, _ = json.Marshal([]float64{b.Ms, b.DNS, b.TCP, b.TLS, b.Cold})
		}
		run.S[r.s.ID] = raw
	}
	if err := writeJSON(dir, "latest.json", latest); err != nil {
		return err
	}

	// today's file, with this run added
	day := at.Format("2006-01-02")
	dp := filepath.Join("days", day+".json")
	df := dayFile{V: 1, Tool: "index-baseline", Day: day}
	if b, err := os.ReadFile(filepath.Join(dir, dp)); err == nil {
		json.Unmarshal(b, &df)
	}
	df.Runs = append(df.Runs, run)
	if err := writeJSON(dir, dp, df); err != nil {
		return err
	}
	pruneDays(filepath.Join(dir, "days"), at)

	// the summary, from the last 7 days of runs
	var runs []dayRun
	for i := summaryDays; i >= 0; i-- {
		d := at.AddDate(0, 0, -i).Format("2006-01-02")
		var f dayFile
		if b, err := os.ReadFile(filepath.Join(dir, "days", d+".json")); err == nil && json.Unmarshal(b, &f) == nil {
			runs = append(runs, f.Runs...)
		}
	}
	return writeJSON(dir, "summary.json", summarizeRuns(from, rs, runs, at))
}

func summarizeRuns(from string, rs []*result, runs []dayRun, at time.Time) summaryFile {
	sort.Slice(runs, func(i, j int) bool { return runs[i].T < runs[j].T })
	hour0 := at.Truncate(time.Hour).Add(-23 * time.Hour).Unix()
	since7 := at.Add(-summaryDays * 24 * time.Hour).Unix()
	sf := summaryFile{V: 1, Tool: "index-baseline", At: at, Every: int(baselineEvery.Seconds()), From: from, H24Start: hour0, Services: map[string]*svcSummary{}}
	for _, r := range runs {
		if r.T >= hour0 {
			sf.Runs24++
		}
	}
	for _, r := range rs {
		id := r.s.ID
		s := &svcSummary{Name: r.s.Name, Cat: r.cat, Last: sampleOf(r), H24: make([]*float64, 24)}
		hours := make([][]float64, 24)
		var m24, m7 []float64
		n24, ok24, n7, ok7 := 0, 0, 0, 0
		var cur *outage
		for _, run := range runs {
			raw, ok := run.S[id]
			if !ok || run.T < since7 {
				continue
			}
			var v []float64
			var e string
			answered := json.Unmarshal(raw, &v) == nil && len(v) > 0 && v[0] >= 0
			if !answered {
				json.Unmarshal(raw, &e)
			}
			n7++
			if answered {
				ok7++
				m7 = append(m7, v[0])
			}
			if run.T >= hour0 {
				n24++
				if answered {
					ok24++
					m24 = append(m24, v[0])
					if h := int((run.T - hour0) / 3600); h >= 0 && h < 24 {
						hours[h] = append(hours[h], v[0])
					}
				}
			}
			switch {
			case !answered && cur == nil:
				cur = &outage{From: run.T, To: run.T, Runs: 1, Err: e}
			case !answered:
				cur.To, cur.Runs = run.T, cur.Runs+1
			case cur != nil:
				s.Out = append(s.Out, *cur)
				cur = nil
			}
		}
		if cur != nil {
			s.Out = append(s.Out, *cur)
		}
		for i, j := 0, len(s.Out)-1; i < j; i, j = i+1, j-1 {
			s.Out[i], s.Out[j] = s.Out[j], s.Out[i]
		}
		if len(s.Out) > 20 {
			s.Out = s.Out[:20]
		}
		for h, v := range hours {
			if m := median(v); m >= 0 {
				s.H24[h] = num(round1(m))
			}
		}
		if n24 > 0 {
			s.Up24 = num(round1(float64(ok24) * 100 / float64(n24)))
		}
		if n7 > 0 {
			s.Up7 = num(round1(float64(ok7) * 100 / float64(n7)))
		}
		if m := median(m24); m >= 0 {
			s.Med24 = num(round1(m))
		}
		if m := median(m7); m >= 0 {
			s.Med7 = num(round1(m))
		}
		sf.Services[id] = s
	}
	return sf
}

func num(v float64) *float64 { return &v }

// pruneDays removes day files older than keepDays
func pruneDays(dir string, now time.Time) {
	cut := now.AddDate(0, 0, -keepDays).Format("2006-01-02")
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		n := e.Name()
		if strings.HasSuffix(n, ".json") && len(n) == len("2006-01-02.json") && n[:10] < cut {
			os.Remove(filepath.Join(dir, n))
		}
	}
}

// writeJSON writes a file whole: to a temporary file in the same folder, flushed to disk, then renamed over the old
// one. A rename within one file system is atomic, so a reader gets the old file or the new one, never part of one.
func writeJSON(dir, name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone after the rename; cleans up when something failed
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
