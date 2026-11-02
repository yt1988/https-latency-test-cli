package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"io"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/httptrace"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// How it measures, the way the page does in a browser (1ms.dog src/app.js): per service, one uncounted request opens
// DNS, TCP and TLS; then each timed request goes over that same connection, a short random pause apart, and is timed
// from sending it to having the response's headers (what a browser's fetch waits for). Any HTTP answer counts, even an
// error status: it still came back from the service's edge. A timed request that had to open a new connection (the
// server closed the old one) isn't counted and is sent again, so a handshake never ends up inside a sample. Two lost
// requests in a row, or no connection at all, and the service is "not back". Several services run at once.

type runOpts struct {
	family   string // "" (as the system picks, like a browser), "4" or "6"
	rounds   int
	timeout  time.Duration
	conc     int
	pack     string
	services []string // ids, empty for all
	cats     []string // empty for all
}

const warmTimeout = 5 * time.Second

type result struct {
	s       *service
	name    string // as shown: Telegram gets its data center
	cat     string
	cold    float64   // the first request, handshakes included, ms (-1: no connection)
	dns     float64   // its parts, ms (-1: not measured, e.g. a cached name)
	connect float64   //
	tls     float64   //
	proto   string    // h2 or http/1.1
	addr    string    // the service's address that answered
	samples []float64 // ms, -1 for lost
	pop     string    // the edge node, as an airport code, when the service names it
	status  int
	err     string
}

func (r *result) median() float64 { return median(r.samples) }

// targets: what a run tests, in the page's order (local pack first), with Telegram's data center for the pack.
func targets(l *list, o runOpts) []*result {
	want := func(s *service, cat string) bool {
		if len(o.services) > 0 {
			return contains(o.services, s.ID)
		}
		return len(o.cats) == 0 || contains(o.cats, cat)
	}
	var out []*result
	add := func(s *service, cat string) {
		if !want(s, cat) {
			return
		}
		r := &result{s: s, name: s.Name, cat: cat}
		if s.ID == "telegram" {
			if d := l.telegramDC(o.pack); d != nil {
				cp := *s
				cp.URL = d.URL
				r.s, r.name = &cp, s.Name+" DC"+strconv.Itoa(d.DC)
			}
		}
		out = append(out, r)
	}
	for _, s := range l.Packs[o.pack] {
		add(s, "local")
	}
	for _, c := range cats[1:] {
		for _, s := range l.Services {
			if s.Cat == c {
				add(s, c)
			}
		}
	}
	// a service picked by id from another region's pack
	if len(o.services) > 0 {
		for _, id := range o.services {
			if s := l.find(id); s != nil && !hasResult(out, id) {
				out = append(out, &result{s: s, name: s.Name, cat: "local"})
			}
		}
	}
	return out
}

func hasResult(rs []*result, id string) bool {
	for _, r := range rs {
		if r.s.ID == id {
			return true
		}
	}
	return false
}

type progressFn func(done, total int, r *result)

func run(ctx context.Context, rs []*result, o runOpts, prog progressFn) {
	q := make(chan *result)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i := 0; i < o.conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range q {
				measure(ctx, r, o)
				mu.Lock()
				done++
				if prog != nil {
					prog(done, len(rs), r)
				}
				mu.Unlock()
			}
		}()
	}
	for _, r := range rs {
		if ctx.Err() != nil {
			break
		}
		q <- r
	}
	close(q)
	wg.Wait()
}

var userAgentHeader = func() string { return userAgent() + " (+https://1ms.dog)" }

func measure(ctx context.Context, r *result, o runOpts) {
	r.cold, r.dns, r.connect, r.tls = -1, -1, -1, -1
	network := "tcp" + o.family
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: warmTimeout}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 1,
		MaxConnsPerHost:     1,
		IdleConnTimeout:     time.Minute,
		TLSHandshakeTimeout: warmTimeout,
		TLSClientConfig:     &tls.Config{},
	}
	if o.family != "" {
		d := &net.Dialer{Timeout: warmTimeout}
		tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) { return d.DialContext(ctx, network, addr) }
	}
	defer tr.CloseIdleConnections()
	cl := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// one request: ms until the headers, whether it went over a connection that was already open
	one := func(timeout time.Duration, first bool) (float64, bool, error) {
		c, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		u := r.s.URL
		if !r.s.Raw {
			u = bust(u)
		}
		method := "GET"
		if r.s.Method == "HEAD" {
			method = "HEAD"
		}
		reused := false
		var dnsAt, connAt, tlsAt time.Time
		trace := &httptrace.ClientTrace{
			GotConn: func(i httptrace.GotConnInfo) {
				reused = i.Reused
				if first && i.Conn != nil {
					if a, ok := i.Conn.RemoteAddr().(*net.TCPAddr); ok {
						r.addr = a.IP.String()
					}
				}
			},
		}
		if first {
			trace.DNSStart = func(httptrace.DNSStartInfo) { dnsAt = time.Now() }
			trace.DNSDone = func(httptrace.DNSDoneInfo) { r.dns = since(dnsAt) }
			trace.ConnectStart = func(string, string) { connAt = time.Now() }
			trace.ConnectDone = func(_, _ string, err error) {
				if err == nil {
					r.connect = since(connAt)
				}
			}
			trace.TLSHandshakeStart = func() { tlsAt = time.Now() }
			trace.TLSHandshakeDone = func(_ tls.ConnectionState, err error) {
				if err == nil {
					r.tls = since(tlsAt)
				}
			}
		}
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(c, trace), method, u, nil)
		if err != nil {
			return -1, false, err
		}
		req.Header.Set("User-Agent", userAgentHeader())
		req.Header.Set("Accept", "*/*")
		t0 := time.Now()
		res, err := cl.Do(req)
		if err != nil {
			return -1, reused, err
		}
		ms := since(t0)
		r.status = res.StatusCode
		if first {
			r.proto = "http/1.1"
			if res.ProtoMajor == 2 {
				r.proto = "h2"
			}
		}
		// read the whole (small) answer, so the connection can be used again
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if p := popOf(res.Header, body); p != "" {
			r.pop = p
		}
		return ms, reused, nil
	}

	cold, _, err := one(warmTimeout, true)
	if err != nil {
		r.err = shortErr(err)
		return
	}
	r.cold = cold
	fails, retries := 0, 0
	for len(r.samples) < o.rounds && fails < 2 && ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(120+mrand.IntN(80)) * time.Millisecond):
		}
		ms, reused, err := one(o.timeout, false)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.samples = append(r.samples, -1)
			fails++
			r.err = shortErr(err)
			continue
		}
		if !reused && retries < o.rounds {
			retries++ // a new connection: its handshake is in this time, ask again on it
			continue
		}
		r.samples = append(r.samples, round1(ms))
		fails = 0
	}
}

func since(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

// bust adds a random query string, as the page does, so no cache between here and the service answers instead
func bust(u string) string {
	b := make([]byte, 4)
	rand.Read(b)
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "_1ms=" + hex.EncodeToString(b)
}

// The edge node that answered, as each network writes it, down to its airport code. A browser only sees these where
// the service allows it; here every response header is readable.
var (
	reColo   = regexp.MustCompile(`\bcolo=([A-Z]{3})\b`)
	reRay    = regexp.MustCompile(`-([A-Z]{3})$`)
	reServed = regexp.MustCompile(`([A-Z]{3})\s*$`)
	reCFPop  = regexp.MustCompile(`^[A-Z]{3}`)
	reWMF    = regexp.MustCompile(`host;desc="?(cp\d)`)
	wmf      = map[string]string{"cp1": "IAD", "cp2": "DFW", "cp3": "AMS", "cp4": "SFO", "cp5": "SIN", "cp6": "MRS", "cp7": "GRU"}
)

func popOf(h http.Header, body []byte) string {
	if m := reColo.FindSubmatch(body); m != nil && strings.Contains(string(body), "fl=") {
		return string(m[1]) // a Cloudflare trace
	}
	if m := reRay.FindStringSubmatch(h.Get("Cf-Ray")); m != nil {
		return m[1]
	}
	if m := reCFPop.FindString(h.Get("X-Amz-Cf-Pop")); m != "" {
		return m
	}
	if m := reServed.FindStringSubmatch(h.Get("X-Served-By")); m != nil {
		return m[1]
	}
	if m := reWMF.FindStringSubmatch(h.Get("Server-Timing")); m != nil {
		return wmf[m[1]]
	}
	return ""
}

// shortErr: what went wrong, in a few words
func shortErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "deadline exceeded"), strings.Contains(s, "Timeout"), strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "no such host"):
		return "no such host"
	case strings.Contains(s, "connection refused"):
		return "refused"
	case strings.Contains(s, "connection reset"):
		return "reset"
	case strings.Contains(s, "certificate"):
		return "certificate"
	case strings.Contains(s, "no route"), strings.Contains(s, "network is unreachable"):
		return "no route"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}
