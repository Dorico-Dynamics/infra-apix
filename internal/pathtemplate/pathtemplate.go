// Package pathtemplate reads route patterns the way chi and gorilla/mux do.
package pathtemplate

import "strings"

// StripPatterns returns path with each {name:pattern} parameter written as
// {name}, as chi and gorilla/mux read it: the name ends at the first colon, and
// the pattern may hold braces and slashes of its own. Everything else, an
// unclosed brace included, is kept as is.
func StripPatterns(path string) string {
	var out strings.Builder
	for {
		open := strings.IndexByte(path, '{')
		if open < 0 {
			break
		}
		end := closingBrace(path, open)
		if end < 0 {
			break
		}
		name, _, _ := strings.Cut(path[open+1:end], ":")
		out.WriteString(path[:open])
		out.WriteString("{" + name + "}")
		path = path[end+1:]
	}
	out.WriteString(path)
	return out.String()
}

// closingBrace returns the index of the brace that closes the one at open,
// counting nested braces, or -1 when it is not closed.
func closingBrace(path string, open int) int {
	depth := 0
	for i := open; i < len(path); i++ {
		switch path[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
