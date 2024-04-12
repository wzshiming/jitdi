package handler

import (
	"fmt"
	"strings"
)

type segment struct {
	s string // literal or parameter name

	wildcard bool
}

type pattern struct {
	segments []segment
}

func parsePattern(s string) (p *pattern, err error) {
	if strings.Index(s, ":") == -1 {
		s += ":latest"
	}

	segs, err := parseSegments(s)
	if err != nil {
		return nil, err
	}

	return &pattern{segs}, nil
}

func (p *pattern) Match(s string) (map[string]string, bool) {
	return matchSegments(p.segments, s)
}

func parseSegments(s string) ([]segment, error) {
	var segs []segment
	off := 0
	for off < len(s) {
		// Find the next '{'.
		start := off
		for off < len(s) && s[off] != '{' {
			off++
		}
		if off > start {
			segs = append(segs, segment{s: s[start:off]})
		}
		if off == len(s) {
			break
		}
		// Find the next '}'.
		start = off
		for off < len(s) && s[off] != '}' {
			off++
		}
		if off == len(s) {
			return nil, fmt.Errorf("unmatched '{' in %q", s)
		}
		if off == start+1 {
			return nil, fmt.Errorf("empty '{}' in %q", s)
		}
		segs = append(segs, segment{s: s[start+1 : off], wildcard: true})
		off++

	}
	return segs, nil
}

func matchSegments(segs []segment, s string) (map[string]string, bool) {
	params := map[string]string{}
	off := 0
	for i, seg := range segs {
		if !seg.wildcard {
			if !strings.HasPrefix(s[off:], seg.s) {
				return nil, false
			}
			off += len(seg.s)
			continue
		}
		if i == len(segs)-1 {
			params[seg.s] = s[off:]
			return params, true
		}
		end := off
		nextSeg := segs[i+1]
		for end < len(s) && !strings.HasPrefix(s[end:], nextSeg.s) {
			end++
		}
		params[seg.s] = s[off:end]
		off = end
	}
	return params, off == len(s)
}
