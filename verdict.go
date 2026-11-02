package main

import (
	"math"
	"sort"
)

// The page's judgement (1ms.dog src/lib.mjs), so a run here and a run in a browser read the same way.

// median of the answered samples (lost ones are -1); -1 when none answered
func median(a []float64) float64 {
	var b []float64
	for _, v := range a {
		if v >= 0 {
			b = append(b, v)
		}
	}
	if len(b) == 0 {
		return -1
	}
	sort.Float64s(b)
	if len(b)%2 == 1 {
		return b[len(b)/2]
	}
	return (b[len(b)/2-1] + b[len(b)/2]) / 2
}

// dog speeds: up to this many ms, the grade (under 2 ms is the legendary 1ms dog)
var tiers = []struct {
	max float64
	key string
}{{2, "legend"}, {30, "zoom"}, {80, "sprint"}, {150, "trot"}, {300, "stroll"}, {math.Inf(1), "nap"}}

func tierOf(ms float64) string {
	if ms < 0 {
		return "lost"
	}
	for _, t := range tiers {
		if ms <= t.max {
			return t.key
		}
	}
	return "nap"
}

// a set of services: how many, how many answered, their median, and the names of those that didn't
type segment struct {
	N, OK int
	MD    float64
	Lost  []string
}

func segmentOf(rs []*result) segment {
	sg := segment{N: len(rs)}
	var meds []float64
	for _, r := range rs {
		if m := r.median(); m >= 0 {
			sg.OK++
			meds = append(meds, m)
		} else {
			sg.Lost = append(sg.Lost, r.name)
		}
	}
	sg.MD = median(meds)
	return sg
}

// good (zoomies, sprint), ok (trot) or slow (stroll, nap); blocked when most didn't answer
func grade(sg segment) string {
	switch {
	case float64(sg.OK) < float64(sg.N)/2:
		return "blocked"
	case sg.MD <= 80:
		return "good"
	case sg.MD <= 150:
		return "ok"
	}
	return "slow"
}

// diagnose says which side the slowness is on, local favorites or global services: the verdict's key and the grade
// that goes with it. The overall median never decides this alone.
func diagnose(local, global segment) (key, mood string) {
	lg, gg := grade(local), grade(global)
	fine := func(g string) bool { return g == "good" || g == "ok" }
	switch {
	case lg == "blocked" && gg == "blocked":
		return "down", "lost"
	case fine(lg) && fine(gg):
		return "allgood", tierOf(median([]float64{local.MD, global.MD}))
	case fine(lg):
		return "far", "far" // near is fast, far is slow or walled off
	case fine(gg):
		return "odd", "odd" // far faster than near: everything goes through one far-away exit
	}
	return "near_slow", tierOf(local.MD) // even nearby is slow: the local network
}

// an everyday use from its services: great up to good ms, ok up to ok ms
func useStatus(sg segment, good, ok float64) string {
	switch {
	case float64(sg.OK) < float64(sg.N)/2:
		return "blocked"
	case sg.MD <= good:
		return "great"
	case sg.MD <= ok:
		return "ok"
	}
	return "slow"
}

// jitter: the mean difference between one answered sample and the next
func jitter(a []float64) float64 {
	var ok []float64
	for _, v := range a {
		if v >= 0 {
			ok = append(ok, v)
		}
	}
	if len(ok) < 2 {
		return -1
	}
	d := 0.0
	for i := 1; i < len(ok); i++ {
		d += math.Abs(ok[i] - ok[i-1])
	}
	return d / float64(len(ok)-1)
}
