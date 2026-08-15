package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- shouldIncludeWord ----
// Covers the real "NAmE" collision bug found during verification: a
// mixed-case acronym must never be silently folded into a lowercase
// common word just because its lowercased form matches.

func TestShouldIncludeWord(t *testing.T) {
	wordlist := map[string]bool{"name": true, "head": true}

	cases := []struct {
		name string
		word string
		want bool
	}{
		{"lowercase word in wordlist", "name", true},
		{"lowercase word not in wordlist", "banana", false},
		{"mixed-case acronym colliding with a wordlist entry when lowercased", "NAmE", false},
		{"all-caps acronym", "NATO", false},
		{"capitalized proper noun", "London", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIncludeWord(tc.word, wordlist)
			assert.Equal(t, tc.want, got, "shouldIncludeWord(%q)", tc.word)
		})
	}
}

// ---- hasSensitiveTag ----

func TestHasSensitiveTag(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want bool
	}{
		{"no tags", nil, false},
		{"unrelated register tags", []string{"slang", "informal", "dated"}, false},
		{"vulgar", []string{"vulgar"}, true},
		{"derogatory", []string{"derogatory"}, true},
		{"offensive", []string{"offensive"}, true},
		{"slur", []string{"slur"}, true},
		{"sensitive tag mixed with others", []string{"slang", "vulgar", "transitive"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSensitiveTag(tc.tags)
			assert.Equal(t, tc.want, got, "hasSensitiveTag(%v)", tc.tags)
		})
	}
}

// ---- cleanEtymologyText ----
// Covers the real "dictionary" and "name" cases: a leading "Etymology
// tree" block and/or a trailing "Cognates" block, both of which are
// machine-generated noise that should be stripped, leaving only the
// readable prose sentence.

func TestCleanEtymologyText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text with no tree or cognates is unchanged",
			in:   "Borrowed from Spanish ñame, substituting n for the unfamiliar Spanish letter ñ. Doublet of yam.",
			want: "Borrowed from Spanish ñame, substituting n for the unfamiliar Spanish letter ñ. Doublet of yam.",
		},
		{
			name: "leading Etymology tree block is stripped",
			in:   "Etymology tree\nProto-Indo-European *deyḱ-\nLatin dictiō\nEnglish dictionary\nFrom Middle English dixionare, a learned borrowing from Medieval Latin dictiōnārium.",
			want: "From Middle English dixionare, a learned borrowing from Medieval Latin dictiōnārium.",
		},
		{
			name: "trailing Cognates block is stripped",
			in:   "From Middle English namen, from Old English namian.\nCognates\nGermanic Cognates: Yola naame, name, naume (\u201cname\u201d)",
			want: "From Middle English namen, from Old English namian.",
		},
		{
			name: "both leading tree and trailing cognates are stripped together",
			in:   "PIE word\n *h\u2081n\u00f3mn\u0325\nEtymology tree\nProto-Indo-European *h\u2081n\u00f3mn\u0325\nEnglish name\nFrom Middle English name, from Old English nama.\nCognates\nGermanic Cognates: Yola naame",
			want: "From Middle English name, from Old English nama.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanEtymologyText(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---- extractExample ----

func TestExtractExample(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantText string
		wantType string
	}{
		{
			name:     "plain string example",
			raw:      `"a law dictionary"`,
			wantText: "a law dictionary",
			wantType: "",
		},
		{
			name:     "object with example type",
			raw:      `{"text":"Stop calling me names!","type":"example"}`,
			wantText: "Stop calling me names!",
			wantType: "example",
		},
		{
			name:     "object with quotation type",
			raw:      `{"text":"That which we call a rose","type":"quotation","ref":"Shakespeare"}`,
			wantText: "That which we call a rose",
			wantType: "quotation",
		},
		{
			name:     "malformed input returns zero value",
			raw:      `42`,
			wantText: "",
			wantType: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractExample(json.RawMessage(tc.raw))
			assert.Equal(t, tc.wantText, got.Text)
			assert.Equal(t, tc.wantType, got.Type)
		})
	}
}

// ---- extractPhonetics ----
// Phonetics are word-scoped, not etymology-scoped: an empirical check
// against the 50k-word wordlist found that wiktextract duplicates the
// same sounds[] array across etymology sections in 85.2% of
// multi-etymology words (3,507 of 4,115) rather than genuinely scoping
// pronunciation per meaning. extractPhonetics is therefore called once
// per word, over ALL of that word's raw entries — not per etymology
// group like toVariant — and is tested independently here.

func TestExtractPhonetics_DialectMapping(t *testing.T) {
	entries := []rawEntry{
		{
			Pos: "noun",
			Sounds: []rawSound{
				{IPA: "/dɪkʃəˌnɛri/", Tags: []string{"General-American"}},
				{IPA: "/dɪkʃənri/", Tags: []string{"Received-Pronunciation"}},
				{IPA: "/ɖɪkʃ(ə)nəri/", Tags: []string{"South-Asia"}},
			},
			Senses: []rawSense{
				{Glosses: []string{"a reference work"}},
			},
		},
	}

	uk, us, other := extractPhonetics(entries)

	assert.Equal(t, "/dɪkʃəˌnɛri/", us, "General-American should map to US")
	assert.Equal(t, "/dɪkʃənri/", uk, "Received-Pronunciation should map to UK")
	assert.Equal(t, "/ɖɪkʃ(ə)nəri/", other, "South-Asia is neither UK nor US")
}

func TestExtractPhonetics_AggregatesAcrossAllEntries(t *testing.T) {
	// Mirrors the real cross-etymology bleed finding: sounds[] can be
	// spread across multiple raw entries for the same word (e.g. one per
	// etymology_number/pos combination). extractPhonetics must be called
	// with ALL of a word's entries, not just one etymology group, since
	// phonetics is modeled as word-scoped, not etymology-scoped.
	entries := []rawEntry{
		{Pos: "noun", EtymologyNumber: "1", Sounds: []rawSound{{IPA: "/beɪ/", Tags: []string{"US"}}}},
		{Pos: "adj", EtymologyNumber: "2", Sounds: []rawSound{{IPA: "/beɪ/", Tags: []string{"UK"}}}},
	}

	uk, us, other := extractPhonetics(entries)

	assert.Equal(t, "/beɪ/", us)
	assert.Equal(t, "/beɪ/", uk)
	assert.Equal(t, "", other)
}

func TestExtractPhonetics_FirstInSourceOrderWinsPerDialect(t *testing.T) {
	// Documents the deliberate simplification: when multiple sounds share
	// the same dialect tag, only the first one encountered is kept. This
	// is stable across rebuilds of the same pinned dump (see source.lock)
	// but does mean a second, equally valid same-dialect transcription is
	// discarded.
	entries := []rawEntry{
		{
			Pos: "noun",
			Sounds: []rawSound{
				{IPA: "/fɜːrst/", Tags: []string{"US"}},
				{IPA: "/sɛkənd/", Tags: []string{"US"}},
			},
		},
	}

	_, us, _ := extractPhonetics(entries)

	assert.Equal(t, "/fɜːrst/", us, "the first US-tagged sound in source order should win")
}

func TestExtractPhonetics_NoSounds(t *testing.T) {
	entries := []rawEntry{
		{Pos: "noun", Senses: []rawSense{{Glosses: []string{"a sense with no sounds"}}}},
	}

	uk, us, other := extractPhonetics(entries)

	assert.Empty(t, uk)
	assert.Empty(t, us)
	assert.Empty(t, other)
}

// ---- toVariant ----

func rawExample(text, exType string) json.RawMessage {
	if exType == "" {
		b, _ := json.Marshal(text)
		return b
	}
	b, _ := json.Marshal(map[string]string{"text": text, "type": exType})
	return b
}

func TestToVariant_DedupsByPosAndGloss(t *testing.T) {
	// Mirrors the real "head" bug: the same gloss repeated across many
	// senses, one per citation, must collapse into a single Definition.
	entries := []rawEntry{
		{
			Pos: "noun",
			Senses: []rawSense{
				{Glosses: []string{"The topmost, foremost, or leading part."}, Examples: []json.RawMessage{rawExample("first citation", "quotation")}},
				{Glosses: []string{"The topmost, foremost, or leading part."}, Examples: []json.RawMessage{rawExample("second citation", "quotation")}},
				{Glosses: []string{"A leader or expert."}, Examples: []json.RawMessage{rawExample("the head of the department", "example")}},
			},
		},
	}

	v := toVariant(entries)

	require.Len(t, v.Definitions, 2, "definitions should be deduped by pos+gloss")
	assert.Equal(t, 2, v.SenseCount)
}

func TestToVariant_SameGlossDifferentPosAreNotCollapsed(t *testing.T) {
	// Covers the real pos-collision finding: 434 word/etymology groups in
	// the 50k-word wordlist had identical gloss text across DIFFERENT
	// parts of speech within the same etymology (e.g. "3rd" as an
	// abbreviated adjective vs. verb). Deduping on gloss alone would
	// silently drop one of these; the key must include partOfSpeech.
	entries := []rawEntry{
		{Pos: "adj", Senses: []rawSense{{Glosses: []string{"abbreviation of third."}}}},
		{Pos: "verb", Senses: []rawSense{{Glosses: []string{"abbreviation of third."}}}},
	}

	v := toVariant(entries)

	require.Len(t, v.Definitions, 2, "same gloss with different pos must both be kept")
	assert.Equal(t, 2, v.SenseCount)

	var gotPos []string
	for _, d := range v.Definitions {
		gotPos = append(gotPos, d.PartOfSpeech)
	}
	assert.ElementsMatch(t, []string{"adj", "verb"}, gotPos)
}

func TestToVariant_PrefersExampleTypeOverQuotation(t *testing.T) {
	entries := []rawEntry{
		{
			Pos: "noun",
			Senses: []rawSense{
				{
					Glosses: []string{"same sense"},
					Examples: []json.RawMessage{
						rawExample("an old literary quotation", "quotation"),
					},
				},
				{
					Glosses: []string{"same sense"},
					Examples: []json.RawMessage{
						rawExample("a short modern example", "example"),
					},
				},
			},
		},
	}

	v := toVariant(entries)
	require.Len(t, v.Definitions, 1)

	assert.Contains(t, v.Definitions[0].Examples, "a short modern example",
		"the type=example sentence should be preferred and included")
}

func TestToVariant_ExcludesSensitiveSenses(t *testing.T) {
	entries := []rawEntry{
		{
			Pos: "noun",
			Senses: []rawSense{
				{Glosses: []string{"The part of the body containing the brain."}},
				{Glosses: []string{"Fellatio or cunnilingus; oral sex."}, Tags: []string{"vulgar", "slang"}},
			},
		},
	}

	v := toVariant(entries)

	require.Len(t, v.Definitions, 1, "the vulgar-tagged sense should be excluded")
	assert.Equal(t, "The part of the body containing the brain.", v.Definitions[0].Definition)
}

func TestToVariant_EtymologyTakenFromFirstNonEmptyAndCleaned(t *testing.T) {
	entries := []rawEntry{
		{
			Pos:           "noun",
			EtymologyText: "Etymology tree\nProto-Indo-European *h₁nómn̥\nEnglish name\nFrom Middle English name, from Old English nama.\nCognates\nGermanic Cognates: Yola naame",
			Senses:        []rawSense{{Glosses: []string{"an identifier"}}},
		},
	}

	v := toVariant(entries)

	assert.Equal(t, "From Middle English name, from Old English nama.", v.Etymology)
}
