package canvas

import "testing"

func TestSplitSummary(t *testing.T) {
	tests := []struct {
		name, in, wantTitle, wantCourse string
	}{
		{"with course", "HW 3 [CS 3431]", "HW 3", "CS 3431"},
		{"no course", "Office hours", "Office hours", ""},
		{"brackets in title", "Lab [draft] due [ECE 2010]", "Lab [draft] due", "ECE 2010"},
		{"trailing space", "Quiz 2 [CS 3431]  ", "Quiz 2", "CS 3431"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, course := SplitSummary(tt.in)
			if title != tt.wantTitle || course != tt.wantCourse {
				t.Errorf("SplitSummary(%q) = (%q, %q), want (%q, %q)",
					tt.in, title, course, tt.wantTitle, tt.wantCourse)
			}
		})
	}
}
