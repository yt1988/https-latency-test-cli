package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSummarizeRuns(t *testing.T) {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	s := &service{ID: "x", Name: "X", Cat: "web"}
	rs := []*result{{s: s, name: "X", cat: "web", samples: []float64{10, 12, 11}, cold: 30, dns: 1, connect: 2, tls: 3}}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	var runs []dayRun
	// 6 runs in the last hour: answered, two failures in a row, answered, answered, answered
	vals := []any{[]float64{10, 1, 2, 3, 30}, "timeout", "timeout", []float64{20, 1, 2, 3, 30}, []float64{30, 1, 2, 3, 30}, []float64{40, 1, 2, 3, 30}}
	for i, v := range vals {
		runs = append(runs, dayRun{T: at.Add(time.Duration(i-5) * 10 * time.Minute).Unix(), S: map[string]json.RawMessage{"x": raw(v)}})
	}
	// one run 3 days ago, out of the 24 hours but in the 7 days
	runs = append(runs, dayRun{T: at.Add(-72 * time.Hour).Unix(), S: map[string]json.RawMessage{"x": raw([]float64{100, 1, 2, 3, 30})}})
	sf := summarizeRuns("Tokyo, JP", rs, runs, at)
	x := sf.Services["x"]
	if sf.Runs24 != 6 || *x.Up24 != 66.7 || *x.Up7 != 71.4 {
		t.Fatalf("runs24 %d up24 %v up7 %v", sf.Runs24, *x.Up24, *x.Up7)
	}
	if *x.Med24 != 25 || *x.Med7 != 30 {
		t.Fatalf("med24 %v med7 %v", *x.Med24, *x.Med7)
	}
	if len(x.Out) != 1 || x.Out[0].Runs != 2 || x.Out[0].Err != "timeout" {
		t.Fatalf("outages %+v", x.Out)
	}
	// the runs 11:10 … 11:50 are in hour 22, the one at 12:00 in hour 23
	if x.H24[23] == nil || *x.H24[23] != 40 || x.H24[22] == nil || *x.H24[22] != 20 || x.H24[0] != nil {
		t.Fatalf("h24 %v %v %v", x.H24[0], x.H24[22], x.H24[23])
	}
	if x.Last.Ms != 11 || x.Last.Err != "" {
		t.Fatalf("last %+v", x.Last)
	}
}

func TestWriteJSONAndPrune(t *testing.T) {
	dir := t.TempDir()
	if err := writeJSON(dir, "a.json", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(dir, "a.json", map[string]int{"n": 2}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.json"))
	if string(b) != `{"n":2}` {
		t.Fatalf("got %s", b)
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 1 {
		t.Fatalf("left behind: %v", es)
	}
	days := filepath.Join(dir, "days")
	os.Mkdir(days, 0o755)
	for _, d := range []string{"2026-09-01", "2026-10-10", "2026-11-04"} {
		os.WriteFile(filepath.Join(days, d+".json"), []byte("{}"), 0o644)
	}
	pruneDays(days, time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	es, _ = os.ReadDir(days)
	if len(es) != 2 || es[0].Name() != "2026-10-10.json" {
		t.Fatalf("after prune: %v", es)
	}
}

func TestStatusAndCompare(t *testing.T) {
	lang, useColor = "en", false
	defer func() { tokyo = nil }()
	at := time.Now().Add(-5 * time.Minute)
	s := &summaryFile{V: 1, At: at, Every: 600, Services: map[string]*svcSummary{
		"github": {Name: "GitHub", Cat: "dev", Last: &bsample{Ms: 40}, Med24: num(42), Up24: num(99.3), Up7: num(100),
			Out: []outage{{From: at.Add(-time.Hour).Unix(), To: at.Add(-50 * time.Minute).Unix(), Runs: 2, Err: "timeout"}}},
		"baidu": {Name: "Baidu", Cat: "pack:cn", Last: &bsample{Ms: -1, Err: "reset"}, Up24: num(80)},
	}}
	var b strings.Builder
	printStatus(&b, s, nil)
	out := b.String()
	for _, want := range []string{"GitHub", "42", "99.3%", "Baidu", "✕", "Outages in the last 7 days", "timeout"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hasn't reported") {
		t.Fatal("a fresh baseline shown as stale")
	}
	tokyo = s
	r := &result{s: &service{ID: "github"}, samples: []float64{150, 160, 155}}
	if c := tokyoCell(r); !strings.Contains(c, "42 ms") || !strings.Contains(c, "+113") {
		t.Fatalf("compare cell %q", c)
	}
	if c := tokyoCell(&result{s: &service{ID: "none"}}); !strings.Contains(c, "–") {
		t.Fatalf("no baseline cell %q", c)
	}
}
