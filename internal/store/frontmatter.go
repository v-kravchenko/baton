package store

import (
	"fmt"
	"strings"
)

// Field is one frontmatter entry: a scalar or a list. Raw holds the source
// lines of a multi-line field (block list, nested map, block scalar), which
// Render writes back unchanged, so fields added by other tools survive.
type Field struct {
	Key    string
	Value  string
	List   []string
	IsList bool
	Raw    string
}

// Frontmatter is an ordered set of top-level `key: value` fields.
type Frontmatter struct {
	Fields []Field
}

func (f *Frontmatter) index(key string) int {
	for i, fl := range f.Fields {
		if fl.Key == key {
			return i
		}
	}
	return -1
}

// Get returns a scalar value ("" if absent; lists are joined with ", ").
func (f *Frontmatter) Get(key string) string {
	if i := f.index(key); i >= 0 {
		if f.Fields[i].IsList {
			return strings.Join(f.Fields[i].List, ", ")
		}
		return f.Fields[i].Value
	}
	return ""
}

// GetList returns a list value; a scalar becomes a one-element list.
func (f *Frontmatter) GetList(key string) []string {
	i := f.index(key)
	if i < 0 {
		return nil
	}
	if f.Fields[i].IsList {
		return append([]string(nil), f.Fields[i].List...)
	}
	if f.Fields[i].Value == "" {
		return nil
	}
	return []string{f.Fields[i].Value}
}

// Set sets a scalar; an empty value deletes the key.
func (f *Frontmatter) Set(key, value string) {
	if value == "" {
		f.Del(key)
		return
	}
	if i := f.index(key); i >= 0 {
		f.Fields[i] = Field{Key: key, Value: value}
		return
	}
	f.Fields = append(f.Fields, Field{Key: key, Value: value})
}

// SetList sets a list; an empty list deletes the key.
func (f *Frontmatter) SetList(key string, list []string) {
	if len(list) == 0 {
		f.Del(key)
		return
	}
	fl := Field{Key: key, List: append([]string(nil), list...), IsList: true}
	if i := f.index(key); i >= 0 {
		f.Fields[i] = fl
		return
	}
	f.Fields = append(f.Fields, fl)
}

// Clone returns a deep copy.
func (f *Frontmatter) Clone() *Frontmatter {
	c := &Frontmatter{Fields: make([]Field, len(f.Fields))}
	for i, fl := range f.Fields {
		fl.List = append([]string(nil), fl.List...)
		c.Fields[i] = fl
	}
	return c
}

// Del removes key.
func (f *Frontmatter) Del(key string) {
	if i := f.index(key); i >= 0 {
		f.Fields = append(f.Fields[:i], f.Fields[i+1:]...)
	}
}

// Split separates frontmatter from body. CRLF is normalized to LF. A document
// without a leading `---` line has no frontmatter.
func Split(data []byte) (fm *Frontmatter, body string, err error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	fm = &Frontmatter{}
	if !strings.HasPrefix(text, "---\n") {
		return fm, text, nil
	}
	rest := text[4:]
	end := -1
	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		end = 0
	} else if i := strings.Index(rest, "\n---\n"); i >= 0 {
		end = i + 1
	} else if strings.HasSuffix(rest, "\n---") {
		end = len(rest) - 3
	}
	if end < 0 {
		return fm, text, fmt.Errorf("frontmatter: missing closing ---")
	}
	head := rest[:end]
	body = strings.TrimPrefix(rest[end:], "---")
	body = strings.TrimLeft(body, "\n")
	lines := strings.Split(strings.TrimSuffix(head, "\n"), "\n")
	for n := 0; n < len(lines); n++ {
		line := lines[n]
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if line != strings.TrimLeft(line, " \t") || strings.HasPrefix(t, "-") {
			return fm, body, fmt.Errorf("frontmatter line %d: expected key: value", n+2)
		}
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			return fm, body, fmt.Errorf("frontmatter line %d: expected key: value", n+2)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "#") {
			v = ""
		}
		// Continuation lines: indented, or `- item` right under the key.
		var cont []string
		for j := n + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				continue
			}
			if l == strings.TrimLeft(l, " \t") && !strings.HasPrefix(l, "-") {
				break
			}
			cont = append(cont, lines[n+1:j+1]...)
			n = j
		}
		var fl Field
		var err error
		if len(cont) == 0 {
			fl, err = parseValue(v)
		} else {
			fl, err = parseMultiline(v, cont)
			fl.Raw = line + "\n" + strings.Join(cont, "\n")
		}
		if err != nil {
			return fm, body, fmt.Errorf("frontmatter line %d: %v", n+2, err)
		}
		fl.Key = k
		if i := fm.index(k); i >= 0 {
			fm.Fields[i] = fl
		} else {
			fm.Fields = append(fm.Fields, fl)
		}
	}
	return fm, body, nil
}

// parseMultiline reads a field whose value continues on the following lines:
// a block list, a block scalar (| or >), a multi-line plain or quoted scalar.
// Anything else (a nested map, a list of maps) has no usable value; it is
// kept only as Raw.
func parseMultiline(v string, cont []string) (Field, error) {
	var items []string
	for _, l := range cont {
		t := strings.TrimSpace(l)
		if t != "" {
			items = append(items, t)
		}
	}
	if v == "" {
		var out []string
		for _, it := range items {
			if strings.HasPrefix(it, "#") {
				continue
			}
			if it != "-" && !strings.HasPrefix(it, "- ") {
				return Field{}, nil
			}
			it = strings.TrimSpace(strings.TrimPrefix(it, "-"))
			if !strings.HasPrefix(it, `"`) && !strings.HasPrefix(it, "'") &&
				(strings.Contains(it, ": ") || strings.HasSuffix(it, ":")) {
				return Field{}, nil
			}
			s, _, err := parseScalar(it)
			if err != nil {
				return Field{}, err
			}
			if s != "" {
				out = append(out, s)
			}
		}
		return Field{List: out, IsList: true}, nil
	}
	if v[0] == '|' || v[0] == '>' {
		sep := "\n"
		if v[0] == '>' {
			sep = " "
		}
		return Field{Value: strings.Join(items, sep)}, nil
	}
	s, _, err := parseScalar(v + " " + strings.Join(items, " "))
	return Field{Value: s}, err
}

func parseValue(v string) (Field, error) {
	if strings.HasPrefix(v, "[") {
		end := strings.LastIndex(v, "]")
		if end < 0 {
			return Field{}, fmt.Errorf("unterminated list")
		}
		items, err := splitList(v[1:end])
		return Field{List: items, IsList: true}, err
	}
	s, _, err := parseScalar(v)
	return Field{Value: s}, err
}

// parseScalar parses a quoted or plain scalar and returns the rest of input.
func parseScalar(v string) (string, string, error) {
	if v == "" {
		return "", "", nil
	}
	switch v[0] {
	case '"':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			c := v[i]
			if c == '"' {
				return b.String(), v[i+1:], nil
			}
			if c == '\\' && i+1 < len(v) {
				i++
				switch v[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(v[i])
				}
				continue
			}
			b.WriteByte(c)
		}
		return "", "", fmt.Errorf("unterminated string")
	case '\'':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			if v[i] == '\'' {
				if i+1 < len(v) && v[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				return b.String(), v[i+1:], nil
			}
			b.WriteByte(v[i])
		}
		return "", "", fmt.Errorf("unterminated string")
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), "", nil
}

func splitList(s string) ([]string, error) {
	var out []string
	s = strings.TrimSpace(s)
	for s != "" {
		var item string
		if s[0] == '"' || s[0] == '\'' {
			var rest string
			var err error
			item, rest, err = parseScalar(s)
			if err != nil {
				return out, err
			}
			s = strings.TrimSpace(rest)
			s = strings.TrimPrefix(s, ",")
		} else {
			item, s, _ = strings.Cut(s, ",")
			item = strings.TrimSpace(item)
		}
		if item != "" {
			out = append(out, item)
		}
		s = strings.TrimSpace(s)
	}
	return out, nil
}

// alwaysQuote are keys whose values are always quoted for readability.
var alwaysQuote = map[string]bool{"title": true, "when": true}

// Render writes the frontmatter block including the `---` lines.
func (f *Frontmatter) Render() string {
	var b strings.Builder
	b.WriteString("---\n")
	for _, fl := range f.Fields {
		if fl.Raw != "" {
			b.WriteString(fl.Raw + "\n")
			continue
		}
		if !fl.IsList && fl.Value == "" {
			b.WriteString(fl.Key + ":\n")
			continue
		}
		b.WriteString(fl.Key + ": ")
		if fl.IsList {
			parts := make([]string, len(fl.List))
			for i, it := range fl.List {
				parts[i] = quoteIfNeeded(it, true)
			}
			b.WriteString("[" + strings.Join(parts, ", ") + "]")
		} else if alwaysQuote[fl.Key] {
			b.WriteString(quote(fl.Value))
		} else {
			b.WriteString(quoteIfNeeded(fl.Value, false))
		}
		b.WriteByte('\n')
	}
	b.WriteString("---\n")
	return b.String()
}

// Compose joins frontmatter and body into a document with LF line endings.
func Compose(fm *Frontmatter, body string) []byte {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimLeft(body, "\n")
	if !strings.HasSuffix(body, "\n") && body != "" {
		body += "\n"
	}
	head := fm.Render()
	if body == "" {
		return []byte(head)
	}
	return []byte(head + "\n" + body)
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func quoteIfNeeded(s string, inList bool) string {
	if s == "" || s != strings.TrimSpace(s) || strings.ContainsAny(s, "\"'\n\t#") ||
		strings.Contains(s, ": ") || strings.HasSuffix(s, ":") ||
		strings.ContainsRune("[]{}&*!|>%@`-?:,", rune(s[0])) ||
		(inList && strings.ContainsAny(s, ",[]")) {
		return quote(s)
	}
	return s
}
