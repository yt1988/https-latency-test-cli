package main

import (
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The result link opens this run on 1ms.dog, the same way a result shared from the page does (1ms.dog src/lib.mjs:
// encodeShare): https://1ms.dog/<lang>/r/2.<unix time base36>.<pack>.<exits>.<results>. It holds no address: only
// the country this ran from, and per service its median. exits: cc~city~org~asn~p|l; results: id:ms×10 in base36,
// "x" for not back.

var siteURL = "https://1ms.dog"

// enc percent-encodes a value, dots and the characters chat apps end a link at included, so the code stays one plain
// path segment
func enc(v string) string {
	s := url.QueryEscape(v)
	s = strings.ReplaceAll(s, "+", "%20")
	r := strings.NewReplacer(".", "%2E", "~", "%7E", "!", "%21", "'", "%27", "(", "%28", ")", "%29", "*", "%2A")
	return r.Replace(s)
}

func shareCode(ts time.Time, pack, country string, rs []*result) string {
	ex := strings.Join([]string{enc(country), "", "", "", "l"}, "~")
	var res []string
	for _, r := range rs {
		m := r.median()
		v := "x"
		if m >= 0 {
			v = strconv.FormatInt(int64(math.Round(m*10)), 36)
		}
		res = append(res, r.s.ID+":"+v)
	}
	return strings.Join([]string{"2", strconv.FormatInt(ts.Unix(), 36), pack, ex, strings.Join(res, ",")}, ".")
}

// shareLink: the link in the page's language
func shareLink(ts time.Time, pack, country string, rs []*result) string {
	slug := ""
	if lang != "en" {
		slug = strings.ToLower(lang) + "/"
	}
	return siteURL + "/" + slug + "r/" + shareCode(ts, pack, country, rs)
}
