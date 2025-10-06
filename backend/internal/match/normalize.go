package match

import (
	"regexp"
	"strings"
)

var (
	protocolStripper = regexp.MustCompile(`^https?://`)
	nonAlphaNum      = regexp.MustCompile(`[^a-z0-9]`)
)

// DomainProfile captures the normalization output for a domain string.
type DomainProfile struct {
	Original   string
	Host       string
	Core       string
	BrandToken string
	Tokens     []string
	AltSplits  []string
}

// NormalizeDomain normalizes and tokenizes the supplied domain name.
func NormalizeDomain(input string) DomainProfile {
	lower := strings.ToLower(strings.TrimSpace(input))
	lower = protocolStripper.ReplaceAllString(lower, "")

	// Trim query, path, fragment
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(lower, sep); idx >= 0 {
			lower = lower[:idx]
		}
	}

	// Drop credentials if present (user:pass@)
	if idx := strings.LastIndex(lower, "@"); idx >= 0 {
		lower = lower[idx+1:]
	}

	lower = strings.Trim(lower, ".")
	lower = strings.TrimPrefix(lower, "www.")

	host := lower
	if idx := strings.IndexRune(host, ':'); idx >= 0 {
		host = host[:idx]
	}

	segments := strings.Split(host, ".")
	segments = compactSegments(segments)
	if len(segments) > 3 {
		segments = segments[len(segments)-3:]
	}

	core := host
	if len(segments) == 1 {
		core = segments[0]
	} else if len(segments) >= 2 {
		tld := segments[len(segments)-1]
		second := segments[len(segments)-2]
		if len(tld) == 2 && len(segments) >= 3 {
			core = segments[len(segments)-3]
		} else {
			core = second
		}
	}

	brandToken := nonAlphaNum.ReplaceAllString(core, "")
	if brandToken == "" {
		brandToken = core
	}

	tokens := splitTokens(core)
	alt := compoundSplits(brandToken)

	return DomainProfile{
		Original:   input,
		Host:       host,
		Core:       core,
		BrandToken: brandToken,
		Tokens:     tokens,
		AltSplits:  alt,
	}
}

func compactSegments(in []string) []string {
	var out []string
	for _, seg := range in {
		if trimmed := strings.TrimSpace(seg); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitTokens(core string) []string {
	core = strings.ReplaceAll(core, "_", "-")
	parts := strings.FieldsFunc(core, func(r rune) bool {
		return r == '-' || r == '_' || r == '+' || (r >= '0' && r <= '9')
	})
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		out = append(out, core)
	}
	return out
}

var genericSuffixes = []string{"support", "help", "shop", "store", "online", "tech", "services", "blog", "app", "world", "global", "labs", "care", "pay", "group", "cloud", "ai", "hub", "zone", "plus"}

func compoundSplits(token string) []string {
	var splits []string
	for _, suffix := range genericSuffixes {
		if strings.HasSuffix(token, suffix) && len(token) > len(suffix)+2 {
			prefix := strings.TrimSuffix(token, suffix)
			splits = appendUnique(splits, prefix)
			splits = appendUnique(splits, suffix)
		}
		if strings.HasPrefix(token, suffix) && len(token) > len(suffix)+2 {
			rest := strings.TrimPrefix(token, suffix)
			splits = appendUnique(splits, suffix)
			splits = appendUnique(splits, rest)
		}
	}
	return splits
}

func appendUnique(s []string, v string) []string {
	if v == "" {
		return s
	}
	for _, existing := range s {
		if existing == v {
			return s
		}
	}
	return append(s, v)
}
