package vibegrid

import (
	"errors"
	"strings"
)

var ErrBlockedTerm = errors.New("blocked term")

type wordBlocklist struct {
	terms []string
}

func newWordBlocklist(rawTerms []string) *wordBlocklist {
	terms := make([]string, 0, len(rawTerms))
	for _, term := range rawTerms {
		normalized := strings.ToLower(strings.TrimSpace(term))
		if normalized != "" {
			terms = append(terms, normalized)
		}
	}
	if len(terms) == 0 {
		return nil
	}
	return &wordBlocklist{terms: terms}
}

func (blocklist *wordBlocklist) review(input AdminPuzzleInput) error {
	if blocklist == nil {
		return nil
	}

	for _, group := range input.Groups {
		if blocklist.contains(group.Name) || blocklist.contains(group.Explanation) {
			return ErrBlockedTerm
		}
		for _, tile := range group.Tiles {
			if blocklist.contains(tile) {
				return ErrBlockedTerm
			}
		}
	}
	return nil
}

func (blocklist *wordBlocklist) contains(value string) bool {
	normalized := strings.ToLower(value)
	for _, term := range blocklist.terms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}
