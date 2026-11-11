//go:build !qa_code_embed

package embedded

// Default builds carry NO embedded QA corpus — neither the code snapshots nor
// the docs site pages. The code tools report "corpus not available" and
// search_n9e_docs only has the local integrations/ samples to search.
// Release builds opt in with `-tags qa_code_embed`, which selects
// code_embed.go instead of this stub and bakes the filtered source snapshots
// of nightingale / categraf / n9e-fe (built by scripts/build-qa-code-assets.sh)
// into the binary. This keeps the default repo build free of generated
// multi-MB assets — same pattern as pkg/sandbox's sandbox_embed.
func CodeTarballs() map[string][]byte { return nil }
func CodeManifest() []byte            { return nil }
func DocsTarball() []byte             { return nil }
