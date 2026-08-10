// Package dictionarydata provides read-only access to two embedded
// dictionary datasets:
//
//   - dataset/curated.json      — the original hand-curated dataset (~30
//     words), previously the hardcoded map literal in words/data.go,
//     externalized to JSON per the earlier data-externalization ticket.
//   - dataset/dictionary.json.gz — the new, much larger Wiktionary-derived
//     dataset (generated via wiktextract), gzip-compressed.
//
// This package is intentionally scoped to DATA ACCESS ONLY. It exposes no
// Lookup(), no word normalization beyond lowercasing for map keys, and no
// decision about which source wins when both have an entry for the same
// word. That precedence decision — curated-first, Wiktionary-fallback,
// mirroring the pattern already used by thesaurus-go for
// curated-synonyms-first, OEWN-fallback — belongs in the calling service
// (apps/api/services/text/words/service.go), not here. This keeps each
// dataset independently swappable and versionable without carrying
// lookup semantics tied to one API's response shape. See the companion
// trade-off analysis document for the full rationale.
package dictionarydata

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"strings"
)

//go:embed dataset/curated.json
var curatedRaw []byte

//go:embed dataset/dictionary.json.gz
var dictionaryRaw []byte

// ---- curated dataset shape (words.json) ----

// CuratedDefinition is one definition entry in the hand-curated dataset.
// Mirrors the original words/data.go definitionEntry — one phonetic, one
// example per definition, no etymology/Variant concept, since the
// curated set never needed to represent homographs.
type CuratedDefinition struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Definition   string `json:"definition"`
	Example      string `json:"example,omitempty"`
}

// CuratedEntry is the full hand-curated record for one word. Mirrors the
// original words/data.go dictionaryEntry.
type CuratedEntry struct {
	Phonetic    string              `json:"phonetic"`
	Definitions []CuratedDefinition `json:"definitions"`
	Synonyms    []string            `json:"synonyms,omitempty"`
}

// ---- Wiktionary-derived dataset shape (dictionary.json.gz) ----

// Definition is one sense within a Variant. PartOfSpeech is kept per
// definition (not hoisted to Variant level) because a single etymology
// commonly mixes parts of speech — e.g. "melancholy" (noun + adjective,
// same origin).
type Definition struct {
	PartOfSpeech string   `json:"partOfSpeech"`
	Definition   string   `json:"definition"`
	Examples     []string `json:"examples,omitempty"`
}

// Variant groups everything that shares a single etymology. Multiple
// Variants on the same Entry mean genuine homographs with unrelated
// origins (e.g. "name" the identifier vs. "name" the Caribbean yam,
// "pond" the body of water vs. the archaic verb "to ponder") — the
// caller decides how to select or present multiple Variants, this
// package does not.
//
// PhoneticUK and PhoneticUS are extracted from a first-pass dialect tag
// whitelist (UK: "UK"/"Received-Pronunciation"/"British"; US:
// "US"/"General-American"). PhoneticOther holds the first IPA that
// didn't match either — this can be either an untagged transcription or
// a real third dialect (e.g. Australian, Scottish); the two cases are
// not distinguished, by design (see trade-off document — a full
// multi-dialect map was out of scope).
//
// Known limitation: for words with multiple etymologies, wiktextract
// sometimes replicates the full sounds[] list across etymology sections
// rather than scoping each dialect tag to the section it belongs to.
// This can cause a Variant's phonetic fields to reflect a sibling
// etymology's pronunciation rather than its own (observed with "name").
// Not corrected here — treated as a known, accepted data-source
// limitation rather than papered over with an unverified heuristic.
type Variant struct {
	Etymology     string       `json:"etymology,omitempty"`
	PhoneticUK    string       `json:"phoneticUK,omitempty"`
	PhoneticUS    string       `json:"phoneticUS,omitempty"`
	PhoneticOther string       `json:"phoneticOther,omitempty"`
	Definitions   []Definition `json:"definitions"`
	SenseCount    int          `json:"senseCount"`
}

// Entry is the full Wiktionary-derived record for one word: one or more
// Variants, one per distinct etymology found in the source. Most words
// have exactly one.
type Entry struct {
	Word     string    `json:"word"`
	Variants []Variant `json:"variants"`
}

// ---- loading ----

var (
	curatedData    map[string]CuratedEntry
	dictionaryData map[string]Entry
)

func init() {
	if err := loadCurated(); err != nil {
		panic("dictionarydata: failed to load embedded dataset/curated.json: " + err.Error())
	}
	if err := loadDictionary(); err != nil {
		panic("dictionarydata: failed to load embedded dataset/dictionary.json.gz: " + err.Error())
	}
}

func loadCurated() error {
	var entries map[string]CuratedEntry
	if err := json.Unmarshal(curatedRaw, &entries); err != nil {
		return err
	}
	curatedData = make(map[string]CuratedEntry, len(entries))
	for word, e := range entries {
		curatedData[strings.ToLower(word)] = e
	}
	return nil
}

func loadDictionary() error {
	gz, err := gzip.NewReader(bytes.NewReader(dictionaryRaw))
	if err != nil {
		return err
	}
	defer gz.Close()

	decompressed, err := io.ReadAll(gz)
	if err != nil {
		return err
	}

	var entries []Entry
	if err := json.Unmarshal(decompressed, &entries); err != nil {
		return err
	}

	dictionaryData = make(map[string]Entry, len(entries))
	for _, e := range entries {
		dictionaryData[strings.ToLower(e.Word)] = e
	}
	return nil
}

// ---- public accessors ----

// GetCurated returns the hand-curated entry for word (case-insensitive),
// or false if it's not among the curated set. Pure data access — no
// normalization beyond lowercasing for lookup.
func GetCurated(word string) (CuratedEntry, bool) {
	e, ok := curatedData[strings.ToLower(word)]
	return e, ok
}

// Get returns the Wiktionary-derived entry for word (case-insensitive),
// or false if the word is not in the dataset. Pure data access — no
// dialect selection, no Variant collapsing, no precedence over
// GetCurated. Callers needing a single flattened result, or a decision
// on which of GetCurated/Get should win for a given word, must build
// that logic themselves — see the trade-off document for why this
// responsibility deliberately lives outside this package.
func Get(word string) (Entry, bool) {
	e, ok := dictionaryData[strings.ToLower(word)]
	return e, ok
}
