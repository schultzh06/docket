package canvas

import (
	"regexp"
	"strings"
)

// Canvas appends the course name in brackets
var courseSuffix = regexp.MustCompile(`\s*\[([^\]]+)\]\s*$`)

// SplitSummary: Separates title from course name
func SplitSummary(s string) (title, course string) {
	m := courseSuffix.FindStringSubmatchIndex(s)
	if m == nil {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(s[:m[0]]), s[m[2]:m[3]]
}

// IsAssignment: Reports whether a canvas UID is an assignment or a calendar event
func IsAssignment(uid string) bool {
	return strings.HasPrefix(uid, "event-assignment-")
}
