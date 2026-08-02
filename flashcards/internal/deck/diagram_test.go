package deck_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	flashcards "github.com/iadk/k8s-flashcards"
	"github.com/iadk/k8s-flashcards/internal/deck"
)

// fence is the markdown code fence. It is a constant because a Go raw string
// literal cannot contain a backtick, and the fixtures below are raw strings.
const fence = "```"

// arrow is the set of Mermaid link operators as a regexp fragment: solid,
// dotted and thick arrows, with their `>`, `o` and `x` heads and an optional
// `<` tail.
//
// Every alternative needs two or more operator characters, which is what keeps
// a lone hyphen inside a term — `kube-apiserver` — from reading as an arrow.
const arrow = `(?:<?-{2,3}[->ox]?|<?-\.-+[->ox]?|<?={2,3}[=>ox]?|~~~)`

var (
	// arrowAdjacentToWord matches a link operator touching a word character:
	// `kubelet-->CRI`. That form hides the term from termUse, whose word
	// boundary class counts `-` and `_` as word characters, so the card leaves
	// the vocabulary gate while still rendering and loading fine.
	arrowAdjacentToWord = regexp.MustCompile(`[0-9A-Za-z_]` + arrow + `|` + arrow + `[0-9A-Za-z_]`)

	// underscoreAdjacentToWord matches `_` against an alphanumeric —
	// `kubelet_status` — which hides the term beside it for the same reason.
	underscoreAdjacentToWord = regexp.MustCompile(`[0-9A-Za-z]_|_[0-9A-Za-z]`)
)

// mermaidBlocks returns the body of every fenced block tagged `mermaid`.
//
// Untagged fences are skipped deliberately. The decks are full of kubectl and
// YAML examples carrying identifiers these rules would reject — `AWS_PROFILE`,
// `http_requests_total` — so widening the scan past the `mermaid` tag would
// force an allowlist, which is a check disabled in all but name. The rules are
// a Mermaid-authoring convention, not a deck-wide one.
func mermaidBlocks(text string) []string {
	var (
		blocks  []string
		current []string
		inBlock bool
	)

	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case !inBlock:
			inBlock = trimmed == fence+"mermaid"
		case strings.HasPrefix(trimmed, fence):
			blocks = append(blocks, strings.Join(current, "\n"))
			current, inBlock = nil, false
		default:
			current = append(current, line)
		}
	}

	return blocks
}

// firstDirective returns a block's first line that is neither blank nor a `%%`
// comment: the line naming the diagram type.
func firstDirective(block string) string {
	for line := range strings.SplitSeq(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "%%") {
			return trimmed
		}
	}

	return ""
}

// diagramStyleIssues reports every house-style violation in one mermaid block.
// The rules and the reasoning behind each are in flashcards/decks/README.md.
func diagramStyleIssues(block string) []string {
	var issues []string

	if d := firstDirective(block); !strings.HasPrefix(d, "flowchart") {
		issues = append(issues, fmt.Sprintf("diagram type is %q, want flowchart", d))
	}

	for _, m := range arrowAdjacentToWord.FindAllString(block, -1) {
		issues = append(issues, fmt.Sprintf(
			"%q: put spaces around the arrow, or the term beside it leaves the vocabulary gate", m))
	}

	for _, m := range underscoreAdjacentToWord.FindAllString(block, -1) {
		issues = append(issues, fmt.Sprintf(
			"%q: use a space or a hyphen, not an underscore, or the term beside it leaves the vocabulary gate", m))
	}

	return issues
}

// A diagram is prose as far as the vocabulary gate is concerned: the scan is
// raw text over `q` + `a` with no markdown awareness, so a glossary term used
// only inside a diagram still needs its `requires:` edge.
//
// This is a permanent guard rather than a one-time observation. It is what
// fails if the scanner is later taught to strip fenced blocks, which would drop
// every diagram out of the gate without a single test going red.
func TestDiagramTermsAreScanned(t *testing.T) {
	t.Parallel()

	body := `
deck: Diagrams
cards:
  - id: term-kubelet
    term: kubelet
    q: What is the kubelet?
    a: The node agent.
  - id: diagram-without-edge
    q: What acts?
    a: |
      Prose naming nothing gated.

      ` + fence + `mermaid
      flowchart LR
        K[kubelet] --> C[containerd]
      ` + fence + `
  - id: diagram-with-edge
    q: What acts?
    a: |
      Prose naming nothing gated.

      ` + fence + `mermaid
      flowchart LR
        K[kubelet] --> C[containerd]
      ` + fence + `
    requires: [term-kubelet]
`

	lib, err := loadDeck(body)
	if err != nil {
		t.Fatal(err)
	}

	glossary := lib.Glossary()

	tests := []struct {
		name string
		card string
		want []string
	}{
		{
			name: "a term used only in a diagram is reported",
			card: "diagram-without-edge",
			want: []string{"kubelet"},
		},
		{
			name: "the same diagram is clean once the card requires the term",
			card: "diagram-with-edge",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, ok := lib.Get(tt.card)
			if !ok {
				t.Fatalf("no such card %q", tt.card)
			}

			if got := usesUnrequiredTerms(c, glossary); !slices.Equal(got, tt.want) {
				t.Errorf("usesUnrequiredTerms(%q) = %v, want %v", tt.card, got, tt.want)
			}
		})
	}
}

// The house style exists to keep diagrams inside the vocabulary gate, so it is
// enforced rather than remembered: every `mermaid` block in every deck is
// checked, and the table pins the rules themselves.
func TestDiagramStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		block  string
		wantOK bool
	}{
		{
			name:   "spaced arrows and bracket labels",
			block:  "flowchart LR\n  API[kube-apiserver] --> K[kubelet]\n  K -->|CRI| C[containerd]",
			wantOK: true,
		},
		{
			// The trap: `-` is a word character to termUse, so "no hyphen beside a
			// word character" looks like the general rule and is not. It rejects
			// every hyphenated Kubernetes term, and every arrow besides.
			name:   "hyphenated terms are not arrows",
			block:  "flowchart LR\n  A[kube-apiserver] --> B[kube-proxy]",
			wantOK: true,
		},
		{
			name:   "a comment may precede the directive",
			block:  "%% the pull loop\nflowchart TD\n  A[kubelet] --> B[containerd]",
			wantOK: true,
		},
		{
			name:   "a bare term node is fine once the arrow is spaced",
			block:  "flowchart LR\n  kubelet --> containerd",
			wantOK: true,
		},
		{
			name:   "solid arrow touching a label",
			block:  "flowchart LR\n  kubelet-->CRI",
			wantOK: false,
		},
		{
			name:   "dotted arrow touching a label",
			block:  "flowchart LR\n  kubelet-.->CRI",
			wantOK: false,
		},
		{
			name:   "thick arrow touching a label",
			block:  "flowchart LR\n  kubelet==>CRI",
			wantOK: false,
		},
		{
			name:   "arrow touching a label on one side only",
			block:  "flowchart LR\n  K[kubelet] -->C[containerd]",
			wantOK: false,
		},
		{
			name:   "underscore inside an identifier",
			block:  "flowchart LR\n  A[kubelet_status] --> B[api-server]",
			wantOK: false,
		},
		{
			name:   "a diagram type other than flowchart",
			block:  "sequenceDiagram\n  A ->> B: assigned",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			issues := diagramStyleIssues(tt.block)
			if ok := len(issues) == 0; ok != tt.wantOK {
				t.Errorf("diagramStyleIssues(%q) = %v, want ok = %v", tt.block, issues, tt.wantOK)
			}
		})
	}

	checkDeckDiagrams(t)
}

// checkDeckDiagrams runs the style rules over every diagram the decks ship.
func checkDeckDiagrams(t *testing.T) {
	t.Helper()

	lib, err := deck.Load(flashcards.Decks, "decks")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range lib.Cards {
		for i, block := range mermaidBlocks(c.Q + "\n" + c.A) {
			t.Run(fmt.Sprintf("%s diagram %d", c.ID, i+1), func(t *testing.T) {
				t.Parallel()

				for _, issue := range diagramStyleIssues(block) {
					t.Errorf("%s: %s\n%s", c.File, issue, block)
				}
			})
		}
	}
}
