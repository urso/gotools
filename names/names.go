package names

import "strings"

type Initials struct {
	initials []map[string]bool
}

var CommonInitialisms = map[string]bool{
	"ACL":   true,
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"DNS":   true,
	"EOF":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"LHS":   true,
	"QPS":   true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"UUID":  true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"VM":    true,
	"XML":   true,
	"XMPP":  true,
	"XSRF":  true,
	"XSS":   true,
}

func Parse(in string) map[string]bool {
	if in == "" {
		return nil
	}

	is := map[string]bool{}
	for _, s := range strings.Split(in, ",") {
		if s != "" {
			is[strings.ToUpper(strings.TrimSpace(s))] = true
		}
	}
	return is
}

func NewInitials(in string) *Initials {
	if m := Parse(in); len(m) > 0 {
		return NewInitialsWith(m, CommonInitialisms)
	}
	return NewInitialsWith(CommonInitialisms)
}

func NewInitialsWith(m ...map[string]bool) *Initials {
	return &Initials{m}
}

func (i *Initials) Has(name string) bool {
	name = strings.ToUpper(name)
	for _, m := range i.initials {
		if v, found := m[name]; found {
			return v
		}
	}
	return false
}

func (i *Initials) StartsWith(name string) string {
	name = strings.ToUpper(name)
	for _, m := range i.initials {
		for key, v := range m {
			if strings.HasPrefix(name, key) {
				if !v {
					return ""
				}
				return key
			}
		}
	}
	return ""
}

func IsTestName(t string) bool {
	for _, prefix := range []string{"Example", "Test", "Benchmark"} {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}
