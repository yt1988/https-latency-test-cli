package main

import "strings"

// Counted texts have key_one, key_few… beside key_other, picked by the language's plural rules (CLDR, whole
// numbers); a form a language doesn't have falls back to key_other. Chinese, Japanese and Korean only have key_other.
func pluralCat(n int) string {
	switch lang {
	case "en", "de", "es":
		if n == 1 {
			return "one"
		}
	case "fr", "pt":
		if n == 0 || n == 1 {
			return "one"
		}
	case "ru":
		switch d, h := n%10, n%100; {
		case d == 1 && h != 11:
			return "one"
		case d >= 2 && d <= 4 && (h < 12 || h > 14):
			return "few"
		default:
			return "many"
		}
	}
	return "other"
}

// TN looks up the counted text key for n and fills it like T.
func TN(key string, n int, a ...any) string {
	if k := key + "_" + pluralCat(n); texts[lang][k] != "" {
		return T(k, a...)
	}
	return T(key+"_other", a...)
}

// isPlural: a plural form other than key_other, which a language has only when it needs it
func isPlural(k string) bool {
	for _, f := range []string{"_zero", "_one", "_two", "_few", "_many"} {
		if strings.HasSuffix(k, f) {
			return texts["en"][strings.TrimSuffix(k, f)+"_other"] != ""
		}
	}
	return false
}
