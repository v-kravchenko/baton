package tips

import (
	"os"
	"runtime"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field weights: a query term scores the best weight among fields it hits.
const (
	wKeyword = 4.0
	wTitle   = 3.0
	wWhen    = 2.0
	wBody    = 1.0
)

// Options tune a search.
type Options struct {
	Error    bool // query is error output: drop noise, require several hits
	All      bool // include refuted and superseded tips
	NoBody   bool // ignore tip bodies (used for show's suggestions)
	MinScore float64
	Limit    int
}

// Result is a scored tip.
type Result struct {
	Tip     *Tip
	Score   float64
	Matched []string // query terms that hit
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an and are as at be but by can do does for from has have how i if in into is it its
		not of on or so that the then this to was what when where which while why will with you your
		та і й а але в у на з із до за що як це не по від для чи при же або щоб коли де
		error помилка`) {
		stopwords[w] = true
	}
	// "error" and "помилка" add nothing: almost every query and tip has them.
}

// Tokens splits text into lower-case words of letters and digits.
func Tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

var enSuffixes = []string{"ations", "ation", "ings", "ing", "ers", "ies", "ied", "es", "ed", "er", "ly", "s"}

var ukSuffixes = []string{
	"ування", "ювання", "ання", "ення", "иться", "ться", "ами", "ями", "ові", "еві", "ого", "ому", "ими", "іми",
	"ий", "ій", "их", "іх", "ою", "ею", "ам", "ям", "ах", "ях", "ом", "ем", "ів", "ти", "ть", "ся",
	"а", "я", "і", "и", "у", "ю", "о", "е", "ь", "ї", "є",
}

// Stem strips one common English or Ukrainian ending, keeping 3+ letters.
func Stem(w string) string {
	n := utf8.RuneCountInString(w)
	if n <= 3 {
		return w
	}
	suffixes := enSuffixes
	if r, _ := utf8.DecodeRuneInString(w); r > unicode.MaxASCII {
		suffixes = ukSuffixes
	}
	for _, s := range suffixes {
		if strings.HasSuffix(w, s) && n-utf8.RuneCountInString(s) >= 3 {
			base := strings.TrimSuffix(w, s)
			if s == "ies" || s == "ied" {
				base += "y"
			}
			return base
		}
	}
	return w
}

func termMatch(q, w string) bool {
	if q == w {
		return true
	}
	// Prefix forms: "lock" ~ "lockfile", "конфіг" ~ "конфігурац".
	short, long := q, w
	if len(short) > len(long) {
		short, long = long, short
	}
	return utf8.RuneCountInString(short) >= 4 && strings.HasPrefix(long, short)
}

// queryTerms turns a query into unique stems.
func queryTerms(q string, errMode bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range Tokens(q) {
		if stopwords[w] || utf8.RuneCountInString(w) < 2 {
			continue
		}
		if errMode && noise(w) {
			continue
		}
		s := Stem(w)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// noise drops numbers, hashes and other tokens that only occur in one log.
func noise(w string) bool {
	digits, hex := 0, true
	for _, r := range w {
		if unicode.IsDigit(r) {
			digits++
		}
		if !strings.ContainsRune("0123456789abcdef", r) {
			hex = false
		}
	}
	return digits == len(w) || (hex && len(w) >= 7 && digits > 0) || digits*2 > len(w)
}

func stems(text string) []string {
	ts := Tokens(text)
	for i, t := range ts {
		ts[i] = Stem(t)
	}
	return ts
}

func hits(term string, words []string) bool {
	for _, w := range words {
		if termMatch(term, w) {
			return true
		}
	}
	return false
}

// CurrentEnv lists environment tags of this machine for `env:` matching.
func CurrentEnv() []string {
	env := []string{runtime.GOOS}
	if strings.Contains(os.Getenv("PREFIX"), "com.termux") {
		env = append(env, "termux", "android")
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		env = append(env, "wsl")
	}
	return env
}

// Search scores tips against a free-text query.
func Search(ts []*Tip, query string, o Options) []Result {
	terms := queryTerms(query, o.Error)
	if len(terms) == 0 {
		return nil
	}
	env := CurrentEnv()
	var out []Result
	for _, t := range ts {
		if !o.All && !t.Live() {
			continue
		}
		var kw []string
		for _, k := range t.Keywords {
			kw = append(kw, stems(k)...)
		}
		title, when := stems(t.Title), stems(t.When)
		var body []string
		if !o.NoBody {
			body = stems(t.Body)
		}
		r := Result{Tip: t}
		for _, term := range terms {
			w := 0.0
			switch {
			case hits(term, kw):
				w = wKeyword
			case hits(term, title):
				w = wTitle
			case hits(term, when):
				w = wWhen
			case hits(term, body):
				w = wBody
			}
			if w > 0 {
				r.Score += w
				r.Matched = append(r.Matched, term)
			}
		}
		if r.Score == 0 {
			continue
		}
		// Whole multi-word keywords found verbatim in the query count extra.
		lq := " " + strings.Join(Tokens(query), " ") + " "
		for _, k := range t.Keywords {
			if strings.Contains(k, " ") && strings.Contains(lq, " "+strings.Join(Tokens(k), " ")+" ") {
				r.Score += 2
			}
		}
		if o.Error && len(terms) > 1 && len(r.Matched) < 2 {
			continue
		}
		if t.Status == Verified {
			r.Score += 0.5
		}
		if len(t.Env) > 0 && !overlaps(t.Env, env) {
			r.Score /= 2
		}
		if r.Score < o.MinScore {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Tip.Date().After(out[j].Tip.Date())
	})
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}
