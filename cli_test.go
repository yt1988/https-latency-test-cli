package main

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

var verb = regexp.MustCompile(`%[sdv]`)

// Every language has every English key, with the same fmt verbs in the same order: a missing key would fall back to
// English, a wrong verb would print %!s(MISSING).
func TestTexts(t *testing.T) {
	for _, l := range languages {
		d, ok := texts[l]
		if !ok {
			t.Errorf("no texts for %s", l)
			continue
		}
		for k, en := range texts["en"] {
			s, ok := d[k]
			if !ok {
				t.Errorf("%s: missing %q", l, k)
				continue
			}
			if a, b := verb.FindAllString(en, -1), verb.FindAllString(s, -1); strings.Join(a, " ") != strings.Join(b, " ") {
				t.Errorf("%s: %q has verbs %v, English %v", l, k, b, a)
			}
		}
	}
	// every key the output asks for by name
	for _, k := range []string{"vh_allgood", "vh_far", "vh_near_slow", "vh_odd", "vh_down", "v_allgood", "v_down", "st_great", "st_blocked",
		"tier_legend", "tier_lost", "cat_local", "cat_cdn", "pk_cn", "pk_me", "use_game", "use_dev", "seg_local", "seg_global", "med"} {
		if texts["en"][k] == "" {
			t.Errorf("no %q", k)
		}
	}
}

func TestKnownLang(t *testing.T) {
	for in, want := range map[string]string{"zh_CN.UTF-8": "zh-CN", "zh_TW": "zh-TW", "zh-Hant-HK": "zh-TW", "ja_JP.UTF-8": "ja",
		"pt_BR": "pt", "C.UTF-8": "", "en_US": "en", "": ""} {
		if got := knownLang(in); got != want {
			t.Errorf("knownLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuiltinList(t *testing.T) {
	l, err := parseList(builtinList)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Services) < 30 || len(l.Packs) < 8 || len(l.Uses) < 4 || l.telegramDC("cn") == nil {
		t.Errorf("thin list: %d services, %d packs, %d uses", len(l.Services), len(l.Packs), len(l.Uses))
	}
	for _, s := range append(l.Services, l.Packs["cn"]...) {
		if !strings.HasPrefix(s.URL, "https://") || s.ID == "" || s.Name == "" {
			t.Errorf("bad service %+v", s)
		}
		if s.Cat != "local" && !contains(cats, s.Cat) {
			t.Errorf("%s: unknown group %q", s.ID, s.Cat)
		}
		if texts["en"]["cat_"+s.Cat] == "" {
			t.Errorf("%s: no name for group %q", s.ID, s.Cat)
		}
	}
	for p := range l.Packs {
		if texts["en"]["pk_"+p] == "" {
			t.Errorf("no name for pack %q", p)
		}
	}
}

func TestTargets(t *testing.T) {
	l, _ := parseList(builtinList)
	rs := targets(l, runOpts{pack: "cn"})
	if rs[0].cat != "local" || rs[0].s.ID != "baidu" || len(rs) != len(l.Services)+len(l.Packs["cn"]) {
		t.Errorf("first %s/%s, %d targets", rs[0].cat, rs[0].s.ID, len(rs))
	}
	for _, r := range rs {
		if r.s.ID == "telegram" && (r.name != "Telegram DC5" || !strings.Contains(r.s.URL, "flora")) {
			t.Errorf("telegram for cn: %s %s", r.name, r.s.URL)
		}
	}
	if strings.Contains(l.find("telegram").URL, "flora") {
		t.Error("the data center was set on the list's own entry, not a copy")
	}
	if rs := targets(l, runOpts{pack: "jp", cats: []string{"ai", "local"}}); len(rs) != 6+4 {
		t.Errorf("ai+local: %d", len(rs))
	}
	if rs := targets(l, runOpts{pack: "", services: []string{"github", "baidu"}}); len(rs) != 2 || rs[0].s.ID != "github" {
		t.Errorf("by id: %d", len(rs))
	}
}

// The verdict reads like the page's: same grades, same thresholds.
func TestVerdict(t *testing.T) {
	seg := func(ok, n int, md float64) segment { return segment{N: n, OK: ok, MD: md} }
	for _, c := range []struct {
		local, global segment
		key           string
	}{
		{seg(4, 4, 20), seg(40, 40, 60), "allgood"},
		{seg(4, 4, 20), seg(40, 40, 250), "far"},
		{seg(4, 4, 20), seg(10, 40, 90), "far"},
		{seg(4, 4, 260), seg(40, 40, 60), "odd"},
		{seg(4, 4, 260), seg(40, 40, 300), "near_slow"},
		{seg(1, 4, 20), seg(5, 40, 60), "down"},
	} {
		if k, _ := diagnose(c.local, c.global); k != c.key {
			t.Errorf("%+v %+v: %s, want %s", c.local, c.global, k, c.key)
		}
	}
	if median([]float64{30, -1, 10, 20}) != 20 || median([]float64{-1}) != -1 || median([]float64{1, 2, 3, 4}) != 2.5 {
		t.Error("median")
	}
	if tierOf(1.5) != "legend" || tierOf(30) != "zoom" || tierOf(81) != "trot" || tierOf(301) != "nap" || tierOf(-1) != "lost" {
		t.Error("tiers")
	}
}

// The result link is the page's own format: the same code as 1ms.dog src/lib.mjs encodeShare gives, with only the country.
func TestShareCode(t *testing.T) {
	rs := []*result{{s: &service{ID: "baidu"}, samples: []float64{21.3}}, {s: &service{ID: "chatgpt"}, samples: []float64{188}}, {s: &service{ID: "claude"}, samples: []float64{-1}}}
	got := shareCode(time.Unix(1793433600, 0), "cn", "CN", rs)
	if want := "2.tnri80.cn.CN~~~~l.baidu:5x,chatgpt:1g8,claude:x"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	lang = "zh-CN"
	defer func() { lang = "en" }()
	if l := shareLink(time.Unix(1793433600, 0), "cn", "", rs[:1]); l != "https://1ms.dog/zh-cn/r/2.tnri80.cn.~~~~l.baidu:5x" {
		t.Error(l)
	}
}

func TestPop(t *testing.T) {
	h := http.Header{}
	h.Set("Cf-Ray", "8f1c2d3e4f5a6b7c-NRT")
	if p := popOf(h, nil); p != "NRT" {
		t.Error("cf-ray", p)
	}
	if p := popOf(http.Header{}, []byte("fl=123\nh=x\ncolo=FRA\nloc=DE\n")); p != "FRA" {
		t.Error("trace", p)
	}
	h = http.Header{}
	h.Set("X-Served-By", "cache-iad-kiad7000-IAD, cache-tyo11950-TYO")
	if p := popOf(h, nil); p != "TYO" {
		t.Error("fastly", p)
	}
	h = http.Header{}
	h.Set("Server-Timing", `cache;desc="hit-front", host;desc="cp5017"`)
	if p := popOf(h, nil); p != "SIN" {
		t.Error("wmf", p)
	}
}

// The menu turns keys into options: each case types its answers and expects the options it builds.
func TestWizard(t *testing.T) {
	l, _ := parseList(builtinList)
	for _, c := range []struct {
		v6   bool
		keys string
		want string // "" for the defaults, "exit" for none
	}{
		{false, "\n\n\n", ""},
		{false, "2\n2,3\n3\n2\n", "-c ai,web -n 10 -o md"},
		{false, "3\n1\n1\n3\n", "-P cn -n 3 -o json"},
		{true, "1\n3\n\n\n", "-6"},
		{true, "1\n2\n\n\n", "-4"},
		{true, "1\n0\n0\n", "exit"},
		{false, "2\n0\n3\n2\n\n\n", "-P tw"},
		{false, "9\n1\n\n\n", ""},
	} {
		hasRoute = func(bool) bool { return c.v6 }
		got := wizard(strings.NewReader(c.keys), &bytes.Buffer{}, l, false)
		if c.want == "exit" {
			if got != nil {
				t.Errorf("%q: want exit, got %v", c.keys, got)
			}
			continue
		}
		if got == nil || strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %v, want %q", c.keys, got, c.want)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.2.0", "0.1.9", true}, {"0.1.1", "0.1.1", false}, {"1.0.0", "dev", false}, {"0.10.0", "0.9.0", true}} {
		if newer(c.a, c.b) != c.want {
			t.Errorf("newer(%s, %s) != %v", c.a, c.b, c.want)
		}
	}
}
