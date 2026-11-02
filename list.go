package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// The services are the ones 1ms.dog tests, from https://1ms.dog/services.json (the site builds it from its own list,
// so the page and this tool measure the same URLs). A copy is built into the binary for when the site can't be
// reached; a daily workflow keeps it current.

//go:embed services.json
var builtinList []byte

var listURL = "https://1ms.dog/services.json"

type service struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Cat    string `json:"cat"`
	URL    string `json:"url"`
	Method string `json:"m,omitempty"`      // GET (default), HEAD, or IMG (the page loads it as an image: a GET here)
	Raw    bool   `json:"raw,omitempty"`    // no cache-busting query (the CDN would go back to its origin)
	Origin string `json:"origin,omitempty"` // hosted in one country only
}

type use struct {
	Use  string   `json:"use"`
	IDs  []string `json:"ids"`
	Good float64  `json:"good"` // smooth up to this many ms
	OK   float64  `json:"ok"`   // fine up to this many
}

type tgDC struct {
	DC   int    `json:"dc"`
	City string `json:"city"`
	CC   string `json:"cc"`
	URL  string `json:"url"`
}

type list struct {
	Version  int                   `json:"version"`
	Services []*service            `json:"services"`
	Packs    map[string][]*service `json:"packs"`
	PackOf   map[string]string     `json:"packOf"`   // exit country → local pack
	LangPack map[string]string     `json:"langPack"` // the page's language → its pack before the country is known
	Uses     []use                 `json:"uses"`
	Telegram []tgDC                `json:"telegram"`   // Telegram's data centers
	TgOf     map[string]int        `json:"telegramOf"` // local pack → the data center most accounts there use
}

func parseList(b []byte) (*list, error) {
	var l list
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.Version != 1 || len(l.Services) < 10 {
		return nil, fmt.Errorf("not a service list (version %d, %d services)", l.Version, len(l.Services))
	}
	return &l, nil
}

// loadList reads the list from a file, from 1ms.dog, or (offline, or when the site doesn't answer) the built-in copy.
// src says which: the file name, "1ms.dog", "built-in" or "offline".
func loadList(file string, offline bool) (*list, string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, "", err
		}
		l, err := parseList(b)
		return l, file, err
	}
	if !offline {
		req, _ := http.NewRequest("GET", listURL, nil)
		req.Header.Set("User-Agent", userAgent())
		if res, err := (&http.Client{Timeout: 4 * time.Second}).Do(req); err == nil {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
			if res.StatusCode == 200 {
				if l, err := parseList(b); err == nil {
					return l, "1ms.dog", nil
				}
			}
		}
	}
	l, err := parseList(builtinList)
	if offline {
		return l, "offline", err
	}
	return l, "built-in", err
}

// cats in the page's order; "local" is the pack picked by the country this runs from
var cats = []string{"local", "ai", "web", "video", "social", "dev", "game", "cdn"}

// find looks a service up by id, in the global list and every pack.
func (l *list) find(id string) *service {
	for _, s := range l.Services {
		if s.ID == id {
			return s
		}
	}
	for _, p := range l.Packs {
		for _, s := range p {
			if s.ID == id {
				return s
			}
		}
	}
	return nil
}

// telegramDC is the data center the page measures for a pack (Amsterdam when the pack has none).
func (l *list) telegramDC(pack string) *tgDC {
	want := l.TgOf[pack]
	if want == 0 {
		want = 2
	}
	for i := range l.Telegram {
		if l.Telegram[i].DC == want {
			return &l.Telegram[i]
		}
	}
	return nil
}
