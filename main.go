// httpslatency times real HTTPS round trips from the machine it runs on to the services 1ms.dog tests: AI, video,
// social, developer, gaming and CDN services worldwide, plus a pack of local favorites for the country it runs from.
// Each one over a warm connection, as a browser would see it, then the same verdict as the page: is it fast nearby,
// worldwide, and for gaming, calls, streaming, AI and developer tools.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev" // set at build time

const usage = `httpslatency: real HTTPS latency from here to the services 1ms.dog tests.

Usage: httpslatency [options]     (with no options in a terminal, a menu asks what to test)

What to test
  -s, --service LIST    services by id, comma separated (chatgpt,youtube,github …; --list shows all)
  -c, --cat LIST        groups: local, ai, web, video, social, dev, game, cdn
  -P, --pack ID         local favorites of a region: cn, tw, jp, kr, ru, in, sea, eu, latam, na, me, or none
                        (default: the country this runs from)
  -4 / -6               IPv4 or IPv6 only (default: as the system picks, like a browser)

How to measure
  -n, --rounds N        timed requests per service, after one that opens the connection (default 5)
  -t, --timeout DUR     per request (default 4s)
  -C, --concurrency N   services measured at once (default 8)

Output
  -o, --output FORMAT   md or json: also saved to a file in the current folder (httpslatency-<date>-<time>.<ext>)
      --file NAME       the file to save to instead
      --json            the full result as JSON on stdout (nothing else)
  -v, --verbose         a line per service: address, protocol, status, handshakes, every sample, jitter
      --no-link         don't print the result link
      --long-link       print it in full instead of shortened (s.1ms.dog/…)
      --lang CODE       en, zh-CN, zh-TW, ja, ko, es, fr, de, pt, ru (the menu asks once and remembers it;
                        HTTPSLATENCY_LANG works too)
      --no-color        plain text (also with NO_COLOR set)
  -q, --quiet           no progress display

Service list and updates
      --offline         use the list built into this binary, and don't look for updates
      --services FILE   use a local list (same format as 1ms.dog/services.json)
      --list            show the services, then exit
      --update          update to the latest release now
  -V, --version

Nothing is uploaded. Besides the requests to the services themselves, a run fetches the current list from 1ms.dog and
asks it where it runs from (city, country and network for the "from" line and the local pack; the address is never
shown or saved). The result link holds the country and each service's time, never an address; it is shortened with
1ms.dog's link shortener, as on the page (--long-link prints it in full without asking). At most once an hour it also
asks 1ms.dog for the latest release and, in a terminal, offers to update (the new binary is checked against the
release checksums). --offline turns all of these off; HTTPSLATENCY_NO_UPDATE=1 only the update check.
`

type cliOpts struct {
	services, cats, pack, lang, file string
	output, saveTo                   string
	v6, v4, json, noLink, longLink   bool
	verbose, quiet, noColor          bool
	offline, list, version           bool
	rounds, conc                     int
	timeout                          time.Duration
}

func main() {
	args := os.Args[1:]

	// language: --lang (read in cli), then HTTPSLATENCY_LANG, then the saved choice, then the system's
	askLang := true
	if l := knownLang(os.Getenv("HTTPSLATENCY_LANG")); l != "" {
		lang, askLang = l, false
	} else if l := knownLang(loadConfig().Lang); l != "" {
		lang, askLang = l, false
	} else {
		lang = pickLang("")
	}
	if code, stop := checkUpdate(args); stop {
		os.Exit(code)
	}

	// no options in a terminal: the menu builds them, and shows them so the test can be run again without it
	if len(args) == 0 && isTTY(os.Stdin) && isTTY(os.Stdout) {
		useColor = os.Getenv("NO_COLOR") == "" && enableVT()
		l, _, err := loadList("", false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "httpslatency: %v\n", err)
			os.Exit(1)
		}
		chosen := wizard(os.Stdin, os.Stdout, l, askLang)
		if chosen == nil {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stdout, "\n%s\n  %s\n", paint(cDim, T("again")), paint(cCyan, commandLine(chosen)))
		args = chosen
	}
	os.Exit(cli(args, os.Stdout, os.Stderr))
}

func cli(args []string, stdout, stderr io.Writer) int {
	var o cliOpts
	fs := flag.NewFlagSet("httpslatency", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	str := func(p *string, def string, names ...string) {
		for _, n := range names {
			fs.StringVar(p, n, def, "")
		}
	}
	boolean := func(p *bool, names ...string) {
		for _, n := range names {
			fs.BoolVar(p, n, false, "")
		}
	}
	integer := func(p *int, def int, names ...string) {
		for _, n := range names {
			fs.IntVar(p, n, def, "")
		}
	}
	str(&o.services, "", "s", "service")
	str(&o.cats, "", "c", "cat")
	str(&o.pack, "", "P", "pack")
	str(&o.lang, "", "lang")
	str(&o.file, "", "services")
	str(&o.output, "", "o", "output")
	str(&o.saveTo, "", "file")
	boolean(&o.v4, "4")
	boolean(&o.v6, "6")
	boolean(&o.json, "json")
	boolean(&o.noLink, "no-link")
	boolean(&o.longLink, "long-link")
	boolean(&o.verbose, "v", "verbose")
	boolean(&o.quiet, "q", "quiet")
	boolean(&o.noColor, "no-color")
	boolean(&o.offline, "offline")
	boolean(&o.list, "list")
	boolean(&o.version, "V", "version")
	update := false
	boolean(&update, "update") // handled before cli; accepted here so it isn't an unknown flag
	integer(&o.rounds, 5, "n", "rounds")
	integer(&o.conc, 8, "C", "concurrency")
	fs.DurationVar(&o.timeout, "t", 4*time.Second, "")
	fs.DurationVar(&o.timeout, "timeout", 4*time.Second, "")
	help := false
	boolean(&help, "h", "help")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "httpslatency: %v\n\n%s", err, usage)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "httpslatency: unexpected %q\n\n%s", fs.Arg(0), usage)
		return 2
	}
	if help {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if o.version {
		fmt.Fprintln(stdout, "httpslatency "+version)
		return 0
	}
	if o.lang != "" {
		l := knownLang(o.lang)
		if l == "" {
			fmt.Fprintf(stderr, "httpslatency: unknown language %q (en, zh-CN, zh-TW, ja, ko, es, fr, de, pt, ru)\n", o.lang)
			return 2
		}
		lang = l
	}
	useColor = !o.noColor && os.Getenv("NO_COLOR") == "" && isTTY(stdout) && enableVT()
	if o.output != "" && o.output != "md" && o.output != "json" {
		fmt.Fprintf(stderr, "httpslatency: --output is md or json, not %q\n", o.output)
		return 2
	}
	if o.rounds < 1 || o.rounds > 100 || o.conc < 1 || o.timeout < 100*time.Millisecond {
		fmt.Fprintln(stderr, "httpslatency: --rounds 1–100, --concurrency ≥ 1, --timeout ≥ 100ms")
		return 2
	}

	l, src, err := loadList(o.file, o.offline)
	if err != nil {
		fmt.Fprintf(stderr, "httpslatency: %v\n", err)
		return 1
	}
	if o.list {
		printList(stdout, l)
		return 0
	}

	ro := runOpts{rounds: o.rounds, timeout: o.timeout, conc: o.conc}
	if o.v4 {
		ro.family = "4"
	} else if o.v6 {
		ro.family = "6"
	}
	for _, id := range splitList(o.services) {
		if l.find(id) == nil {
			fmt.Fprintf(stderr, "httpslatency: no service %q (httpslatency --list shows them)\n", id)
			return 2
		}
		ro.services = append(ro.services, id)
	}
	for _, c := range splitList(o.cats) {
		if !contains(cats, c) {
			fmt.Fprintf(stderr, "httpslatency: unknown group %q (%s)\n", c, strings.Join(cats, ", "))
			return 2
		}
		ro.cats = append(ro.cats, c)
	}
	pack := strings.ToLower(strings.TrimSpace(o.pack))
	if pack != "" && pack != "none" && l.Packs[pack] == nil {
		fmt.Fprintf(stderr, "httpslatency: unknown pack %q (%s, none)\n", pack, strings.Join(packIDs(l), ", "))
		return 2
	}

	// where this runs from: for the "from" line, and the local favorites of that country
	var from *Origin
	if !o.offline {
		from = origin(ro.family)
	}
	switch {
	case pack == "none":
		ro.pack = ""
	case pack != "":
		ro.pack = pack
	case from != nil && l.PackOf[from.Country] != "":
		ro.pack = l.PackOf[from.Country]
	default:
		ro.pack = l.LangPack[lang]
	}
	rs := targets(l, ro)
	if len(rs) == 0 {
		fmt.Fprintln(stderr, "httpslatency: nothing to test")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// progress on stderr: the running dog in a color terminal wide enough for it, a plain line in any other terminal
	var prog progressFn
	var finish func()
	if !o.quiet && !o.json && isTTY(stderr) {
		cols := termCols(os.Stderr)
		if r := startRunner(stderr, cols); r != nil && !o.noColor && os.Getenv("NO_COLOR") == "" && enableVT() {
			prog = func(done, total int, x *result) {
				r.update(done, total, fmt.Sprintf("%3d%%  %s", done*100/max(1, total), paint(cDim, x.name)))
			}
			finish = r.finish
		} else {
			if r != nil {
				r.finish()
			}
			pl := newProgLine(stderr)
			prog, finish = pl.update, pl.clear
		}
	}
	t0 := time.Now()
	run(ctx, rs, ro, prog)
	took := time.Since(t0)
	if finish != nil {
		finish()
	}
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "httpslatency: interrupted")
		return 130
	}

	link := ""
	if !o.noLink && ro.pack != "" {
		cc := ""
		if from != nil {
			cc = from.Country
		}
		link = shareLink(t0, ro.pack, cc, rs)
		if !o.longLink && !o.offline {
			link = shorten(link)
		}
	}
	if o.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(toJSON(l, rs, ro, from, src, link, took))
		return 0
	}
	printResult(stdout, l, rs, ro, from, src, link, o.verbose, took)

	// -o: the same result saved as a file next to where it was run
	if o.output != "" {
		name := o.saveTo
		if name == "" {
			name = "httpslatency-" + time.Now().Format("20060102-150405") + "." + o.output
		}
		f, err := os.Create(name)
		if err != nil {
			fmt.Fprintf(stderr, "httpslatency: %v\n", err)
			return 1
		}
		if o.output == "json" {
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			enc.Encode(toJSON(l, rs, ro, from, src, link, took))
		} else {
			printMarkdown(f, l, rs, ro, from, link, took)
		}
		f.Close()
		if abs, err := filepath.Abs(name); err == nil {
			name = abs
		}
		fmt.Fprintln(stdout, paint(cGreen, T("saved"))+gap(T("saved"))+name)
	}
	return 0
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}
