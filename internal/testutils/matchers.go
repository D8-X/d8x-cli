package testutils

import (
	"strings"

	"go.uber.org/mock/gomock"
)

var _ (gomock.Matcher) = (*MatchStringContains)(nil)

// MatchStringContains is gomock mathcer that matches if string contains
// substring
type MatchStringContains struct {
	Contains string
}

func (m MatchStringContains) Matches(x interface{}) bool {
	return strings.Contains(x.(string), m.Contains)
}
func (m MatchStringContains) String() string {
	return "Matches if string contains substring " + m.Contains
}
