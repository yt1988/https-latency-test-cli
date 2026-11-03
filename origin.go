package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Where the test runs from, for the "from" line and to pick the local pack: city, country and network. 1ms.dog answers
// it from what Cloudflare knows of the connection (/_ms/me, the page asks the same); Cloudflare's speed test is the
// fallback. The address itself is never printed or saved.
type Origin struct {
	ASN     int    `json:"asn,omitempty"`
	Org     string `json:"org,omitempty"`
	City    string `json:"city,omitempty"`
	Country string `json:"country,omitempty"`
}

var meURL = "https://1ms.dog/_ms/me"

func origin(family string) *Origin {
	d := &net.Dialer{Timeout: 4 * time.Second}
	tr := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
		return d.DialContext(ctx, "tcp"+family, addr)
	}}
	cl := &http.Client{Transport: tr, Timeout: 6 * time.Second}
	defer tr.CloseIdleConnections()
	get := func(u string, v any) bool {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", userAgent())
		res, err := cl.Do(req)
		if err != nil {
			return false
		}
		defer res.Body.Close()
		return res.StatusCode == 200 && json.NewDecoder(res.Body).Decode(v) == nil
	}
	var me struct {
		Country string          `json:"country"`
		City    string          `json:"city"`
		ASN     json.RawMessage `json:"asn"`
		Org     string          `json:"org"`
	}
	if get(meURL, &me) && me.Country != "" {
		asn, _ := strconv.Atoi(strings.Trim(string(me.ASN), `"`))
		return &Origin{ASN: asn, Org: me.Org, City: me.City, Country: me.Country}
	}
	var m struct {
		ASN            json.RawMessage `json:"asn"`
		AsOrganization string          `json:"asOrganization"`
		City           string          `json:"city"`
		Country        string          `json:"country"`
	}
	if get("https://speed.cloudflare.com/meta", &m) && m.Country != "" {
		asn, _ := strconv.Atoi(strings.Trim(string(m.ASN), `"`))
		return &Origin{ASN: asn, Org: m.AsOrganization, City: m.City, Country: m.Country}
	}
	return nil
}

// tw: TW has a fixed name in every language, as on 1ms.dog; other places show their country code
var tw = map[string]string{"zh-CN": "中国台湾", "zh-TW": "中國台灣", "en": "Taiwan, China", "ja": "中国台湾", "ko": "중국 대만",
	"es": "Taiwán, China", "fr": "Taïwan, Chine", "de": "Taiwan, China", "pt": "Taiwan, China", "ru": "Тайвань, Китай"}

func regionName(cc string) string {
	if cc != "TW" {
		return cc
	}
	if n, ok := tw[lang]; ok {
		return n
	}
	return tw["en"]
}

// String: "Frankfurt am Main, DE · AS197540 netcup GmbH"
func (o *Origin) String() string {
	if o == nil {
		return T("unknownNet")
	}
	where := strings.Trim(o.City+", "+regionName(o.Country), ", ")
	net := o.Org
	if o.ASN > 0 {
		net = strings.TrimSpace("AS" + strconv.Itoa(o.ASN) + " " + o.Org)
	}
	if where == "" {
		return net
	}
	if net == "" {
		return where
	}
	return where + " · " + net
}
