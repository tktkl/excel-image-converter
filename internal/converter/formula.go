package converter

import (
	"errors"
	"path"
	"strings"
)

var errNotLiteralImageFormula = errors.New("not a literal IMAGE(url) formula")

func extractLiteralImageURL(formula string) (string, error) {
	f := strings.TrimSpace(formula)
	if strings.HasPrefix(f, "=") {
		f = strings.TrimSpace(f[1:])
	}
	f = strings.TrimPrefix(f, "_xlfn.")
	f = strings.TrimSpace(f)

	if len(f) < len("IMAGE(") || !strings.EqualFold(f[:len("IMAGE(")], "IMAGE(") {
		return "", errNotLiteralImageFormula
	}

	arg, ok := firstFormulaArgument(f[len("IMAGE("):])
	if !ok {
		return "", errNotLiteralImageFormula
	}
	arg = strings.TrimSpace(arg)
	if len(arg) < 2 || arg[0] != '"' || arg[len(arg)-1] != '"' {
		return "", errNotLiteralImageFormula
	}
	return unquoteExcelString(arg)
}

func firstFormulaArgument(rest string) (string, bool) {
	var b strings.Builder
	inQuotes := false
	depth := 0

	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if ch == '"' {
			b.WriteByte(ch)
			if inQuotes && i+1 < len(rest) && rest[i+1] == '"' {
				b.WriteByte('"')
				i++
				continue
			}
			inQuotes = !inQuotes
			continue
		}

		if !inQuotes {
			switch ch {
			case '(':
				depth++
			case ')':
				if depth == 0 {
					return strings.TrimSpace(b.String()), true
				}
				depth--
			case ',':
				if depth == 0 {
					return strings.TrimSpace(b.String()), true
				}
			}
		}
		b.WriteByte(ch)
	}
	return "", false
}

func unquoteExcelString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", errNotLiteralImageFormula
	}
	return strings.ReplaceAll(value[1:len(value)-1], `""`, `"`), nil
}

func extractPlainImageURL(value string) (string, bool) {
	url := strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
		return "", false
	}

	clean := url
	if idx := strings.IndexAny(clean, "?#"); idx >= 0 {
		clean = clean[:idx]
	}
	switch strings.ToLower(path.Ext(clean)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		return url, true
	default:
		return "", false
	}
}
