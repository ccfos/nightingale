//go:build qa_code_embed

package embedded

// Built with `-tags qa_code_embed`. The assets are produced by
// scripts/build-qa-code-assets.sh: filtered source snapshots of the three
// repos the doc-qa skill answers questions about, a manifest recording
// the exact ref/commit each snapshot was taken from, and the rendered
// Markdown pages of the docs site (https://n9e.github.io). Extracted at startup by
// skill.ExtractCodeCorpus; searched at runtime by the list_code / search_code
// / read_code builtin tools.
//
// The script also generates code_assets_generated.go with the embed declarations
// after all assets are ready. Keep those declarations out of tracked Go files:
// go mod vendor resolves embeds for all build tags, including qa_code_embed,
// and must work in a clean checkout without the generated corpus. A tagged build
// without running the script still fails because the asset variables are absent.

// CodeTarballs returns the embedded corpus tarballs keyed by repo name. The
// keys double as the extraction directory names under <projectRoot>/code/ and
// as the `repo` argument whitelist of the code tools.
func CodeTarballs() map[string][]byte {
	return map[string][]byte{
		"n9e":      codeN9e,
		"categraf": codeCategraf,
		"fe":       codeFe,
	}
}

func CodeManifest() []byte { return codeManifest }

// DocsTarball returns the docs site's rendered Markdown pages (en under docs/,
// zh under zh/docs/), packed from the n9e.github.io gh-pages branch. Read in
// memory by the search_n9e_docs tool; never extracted to disk.
func DocsTarball() []byte { return docsTarball }
