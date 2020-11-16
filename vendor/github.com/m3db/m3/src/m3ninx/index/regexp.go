// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package index

import (
	"fmt"
	re "regexp"
	"regexp/syntax"

	fstregexp "github.com/m3db/m3/src/m3ninx/index/segment/fst/regexp"
)

var (
	// dotStartCompiledRegex is a CompileRegex that matches any input.
	// NB: It can be accessed through DotStartCompiledRegex().
	dotStarCompiledRegex CompiledRegex
)

func init() {
	re, err := CompileRegex([]byte(".*"))
	if err != nil {
		panic(err.Error())
	}
	dotStarCompiledRegex = re
}

// DotStarCompiledRegex returns a regexp which matches ".*".
func DotStarCompiledRegex() CompiledRegex {
	return dotStarCompiledRegex
}

// CompileRegex compiles the provided regexp into an object that can be used to query the various
// segment implementations.
func CompileRegex(r []byte) (CompiledRegex, error) {
	// NB(prateek): We currently use two segment implementations: map-backed, and fst-backed (Vellum).
	// Due to peculiarities in the implementation of Vellum, we have to make certain modifications
	// to all incoming regular expressions to ensure compatibility between them.

	// first, we parse the regular expression into the equivalent regex
	reString := string(r)
	reAst, err := parseRegexp(reString)
	if err != nil {
		return CompiledRegex{}, err
	}

	// Issue (a): Vellum does not allow regexps which use characters '^', or '$'.
	// To address this issue, we strip these characters from appropriate locations in the parsed syntax.Regexp
	// for Vellum's RE.
	vellumRe, err := ensureRegexpUnanchored(reAst)
	if err != nil {
		return CompiledRegex{}, fmt.Errorf("unable to create FST re: %v", err)
	}

	// Issue (b): Vellum treats every regular expression as anchored, where as the map-backed segment does not.
	// To address this issue, we ensure that every incoming regular expression is modified to be anchored
	// when querying the map-backed segment, and isn't anchored when querying Vellum's RE.
	simpleRe, err := ensureRegexpAnchored(vellumRe)
	if err != nil {
		return CompiledRegex{}, fmt.Errorf("unable to create map re: %v", err)
	}

	simpleRE, err := re.Compile(simpleRe.String())
	if err != nil {
		return CompiledRegex{}, err
	}
	compiledRegex := CompiledRegex{
		Simple:    simpleRE,
		FSTSyntax: vellumRe,
	}

	fstRE, start, end, err := fstregexp.ParsedRegexp(vellumRe.String(), vellumRe)
	if err != nil {
		return CompiledRegex{}, err
	}
	compiledRegex.FST = fstRE
	compiledRegex.PrefixBegin = start
	compiledRegex.PrefixEnd = end

	return compiledRegex, nil
}

func parseRegexp(re string) (*syntax.Regexp, error) {
	return syntax.Parse(re, syntax.Perl)
}

// ensureRegexpAnchored adds '^' and '$' characters to appropriate locations in the parsed syntax.Regexp,
// to ensure every input regular expression is converted to it's equivalent anchored regular expression.
// NB: assumes input regexp AST is un-anchored.
func ensureRegexpAnchored(unanchoredRegexp *syntax.Regexp) (*syntax.Regexp, error) {
	ast := &syntax.Regexp{
		Op:    syntax.OpConcat,
		Flags: syntax.Perl,
		Sub: []*syntax.Regexp{
			&syntax.Regexp{
				Op:    syntax.OpBeginText,
				Flags: syntax.Perl,
			},
			unanchoredRegexp,
			&syntax.Regexp{
				Op:    syntax.OpEndText,
				Flags: syntax.Perl,
			},
		},
	}
	return simplify(ast.Simplify()), nil
}

// ensureRegexpUnanchored strips '^' and '$' characters from appropriate locations in the parsed syntax.Regexp,
// to ensure every input regular expression is converted to it's equivalent un-anchored regular expression
// assuming the entire input is matched.
func ensureRegexpUnanchored(parsed *syntax.Regexp) (*syntax.Regexp, error) {
	r, _, err := ensureRegexpUnanchoredHelper(parsed, true, true)
	if err != nil {
		return nil, err
	}
	return simplify(r), nil
}

func ensureRegexpUnanchoredHelper(parsed *syntax.Regexp, leftmost, rightmost bool) (output *syntax.Regexp, changed bool, err error) {
	// short circuit when we know we won't make any changes to the underlying regexp.
	if !leftmost && !rightmost {
		return parsed, false, nil
	}

	switch parsed.Op {
	case syntax.OpBeginLine, syntax.OpEndLine:
		// i.e. the flags provided to syntax.Parse did not include the `OneLine` flag, which
		// should never happen as we're using syntax.Perl which does include it (ensured by a test
		// in this package).
		return nil, false, fmt.Errorf("regular expressions are forced to be single line")
	case syntax.OpBeginText:
		if leftmost {
			return &syntax.Regexp{
				Op:    syntax.OpEmptyMatch,
				Flags: parsed.Flags,
			}, true, nil
		}
	case syntax.OpEndText:
		if rightmost {
			return &syntax.Regexp{
				Op:    syntax.OpEmptyMatch,
				Flags: parsed.Flags,
			}, true, nil
		}
	case syntax.OpCapture:
		// because golang regexp's don't allow backreferences, we don't care about maintaining capture
		// group namings and can treate captures the same as we do conactenations.
		fallthrough
	case syntax.OpConcat:
		changed := false
		// strip left-most '^'
		if l := len(parsed.Sub); leftmost && l > 0 {
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[0], leftmost, rightmost && l == 1)
			if err != nil {
				return nil, false, err
			}
			if c {
				parsed.Sub[0] = newRe
				changed = true
			}
		}
		// strip right-most '$'
		if l := len(parsed.Sub); rightmost && l > 0 {
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[l-1], leftmost && l == 1, rightmost)
			if err != nil {
				return nil, false, err
			}
			if c {
				parsed.Sub[l-1] = newRe
				changed = true
			}
		}
		return parsed, changed, nil
	case syntax.OpAlternate:
		changed := false
		// strip left-most '^' and right-most '$' in each sub-expression
		for idx := range parsed.Sub {
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[idx], leftmost, rightmost)
			if err != nil {
				return nil, false, err
			}
			if c {
				parsed.Sub[idx] = newRe
				changed = true
			}
		}
		return parsed, changed, nil
	case syntax.OpQuest:
		if len(parsed.Sub) > 0 {
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[0], leftmost, rightmost)
			if err != nil {
				return nil, false, err
			}
			if c {
				parsed.Sub[0] = newRe
				return parsed, true, nil
			}
		}
	case syntax.OpStar:
		if len(parsed.Sub) > 0 {
			original := deepCopy(parsed)
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[0], leftmost, rightmost)
			if err != nil {
				return nil, false, err
			}
			if !c {
				return parsed, false, nil
			}
			return &syntax.Regexp{
				Op:    syntax.OpConcat,
				Flags: parsed.Flags,
				Sub: []*syntax.Regexp{
					&syntax.Regexp{
						Op:    syntax.OpQuest,
						Flags: parsed.Flags,
						Sub: []*syntax.Regexp{
							newRe,
						},
					},
					original,
				},
			}, true, nil
		}
	case syntax.OpPlus:
		if len(parsed.Sub) > 0 {
			original := deepCopy(parsed)
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[0], leftmost, rightmost)
			if err != nil {
				return nil, false, err
			}
			if !c {
				return parsed, false, nil
			}
			return &syntax.Regexp{
				Op:    syntax.OpConcat,
				Flags: parsed.Flags,
				Sub: []*syntax.Regexp{
					newRe,
					&syntax.Regexp{
						Op:    syntax.OpStar,
						Flags: parsed.Flags,
						Sub: []*syntax.Regexp{
							original.Sub[0],
						},
					},
				},
			}, true, nil
		}
	case syntax.OpRepeat:
		if len(parsed.Sub) > 0 && parsed.Min > 0 {
			original := deepCopy(parsed)
			newRe, c, err := ensureRegexpUnanchoredHelper(parsed.Sub[0], leftmost, rightmost)
			if err != nil {
				return nil, false, err
			}
			if !c {
				return parsed, false, nil
			}
			original.Min--
			if original.Max != -1 {
				original.Max--
			}
			return &syntax.Regexp{
				Op:    syntax.OpConcat,
				Flags: parsed.Flags,
				Sub: []*syntax.Regexp{
					newRe,
					original,
				},
			}, true, nil
		}
	}
	return parsed, false, nil
}

func deepCopy(ast *syntax.Regexp) *syntax.Regexp {
	if ast == nil {
		return nil
	}
	copied := *ast
	copied.Sub = make([]*syntax.Regexp, 0, len(ast.Sub))
	for _, r := range ast.Sub {
		copied.Sub = append(copied.Sub, deepCopy(r))
	}
	if len(copied.Sub0) != 0 && copied.Sub0[0] != nil {
		copied.Sub0[0] = deepCopy(copied.Sub0[0])
	}
	// NB(prateek): we don't copy ast.Rune (which could be a heap allocated slice) intentionally,
	// because none of the transformations we apply modify the Rune slice.
	return &copied
}

var emptyStringOps = []syntax.Op{
	syntax.OpEmptyMatch, syntax.OpQuest, syntax.OpPlus, syntax.OpStar, syntax.OpRepeat,
}

func matchesEmptyString(ast *syntax.Regexp) bool {
	if ast == nil {
		return false
	}
	for _, op := range emptyStringOps {
		if ast.Op == op {
			if len(ast.Sub) > 0 {
				return matchesEmptyString(ast.Sub[0])
			}
			return true
		}
	}
	return false
}

func simplify(ast *syntax.Regexp) *syntax.Regexp {
	newAst, _ := simplifyHelper(ast)
	return newAst
}

func simplifyHelper(ast *syntax.Regexp) (*syntax.Regexp, bool) {
	if ast == nil {
		return nil, false
	}
	switch ast.Op {
	case syntax.OpConcat:
		// a concatenation of a single sub-expression is the same as the sub-expression itself
		if len(ast.Sub) == 1 {
			return ast.Sub[0], true
		}

		changed := false
		// check if we have any concats of concats, if so, we can pull the ones below this level up
		subs := make([]*syntax.Regexp, 0, len(ast.Sub))
		for _, sub := range ast.Sub {
			if sub.Op == syntax.OpConcat {
				subs = append(subs, sub.Sub...)
				changed = true
				continue
			}
			// skip any sub expressions that devolve to matching only the empty string
			if matchesEmptyString(sub) {
				changed = true
				continue
			}
			subs = append(subs, sub)
		}

		// now ensure we simplify all sub-expressions
		for idx := range subs {
			s, c := simplifyHelper(subs[idx])
			if c {
				subs[idx] = s
				changed = true
			}
		}

		// if we have made any changes to sub-expressions, need to continue simplification
		// until we are sure there are no more changes.
		if changed {
			ast.Sub = subs
			return simplifyHelper(ast)
		}
	default:
		changed := false
		for idx := range ast.Sub {
			newRe, c := simplifyHelper(ast.Sub[idx])
			if c {
				ast.Sub[idx] = newRe
				changed = true
			}
		}
		return ast, changed
	}
	return ast, false
}
