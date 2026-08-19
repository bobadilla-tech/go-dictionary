// Package dictionarydata provides read-only access to two embedded
// dictionary datasets:
//
//   - dataset/curated.json      — the original hand-curated dataset (~30
//     words), previously the hardcoded map literal in words/data.go,
//     externalized to JSON per the earlier data-externalization ticket.
//   - dataset/dictionary.json.gz — the new, much larger Wiktionary-derived
//     dataset (generated via wiktextract), gzip-compressed.
//
// This package is scoped to data access: it looks words up by
// lowercased key and returns the raw stored record. The precedence
// decision for which source wins when both have an entry for the same
// word — curated-first, Wiktionary-fallback, mirroring the pattern
// already used by thesaurus-go for curated-synonyms-first, OEWN-fallback
// — belongs in the calling service (apps/api/services/text/words/service.go).
// This keeps each dataset independently swappable and versionable, with
// lookup semantics owned by whichever API shape consumes them
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
// Mirrors the original words/data.go definitionEntry: a part of speech,
// a definition, and a single example.
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

// Definition is one sense within a Variant: a part of speech, the
// definition text, and up to several example sentences (Examples). Each
// Definition carries its own PartOfSpeech because a single etymology
// commonly spans multiple parts of speech — e.g. "melancholy" (noun and
// adjective, same origin). The generator deduplicates definitions within
// a Variant by partOfSpeech + gloss text together, since identical gloss
// text can legitimately span different parts of speech within the same
// etymology (e.g. "3rd" as an abbreviated adjective vs. verb).
type Definition struct {
	PartOfSpeech string   `json:"partOfSpeech"`
	Definition   string   `json:"definition"`
	Examples     []string `json:"examples,omitempty"`
}

// Variant groups everything that shares a single etymology: the
// etymology text, its Definitions, and SenseCount (the number of
// definitions). Multiple Variants on the same Entry represent genuine
// homographs with unrelated origins — e.g. "name" the identifier vs.
// "name" the Caribbean yam, "pond" the body of water vs. the archaic
// verb "to ponder".
//
// Phonetic fields live on Entry rather than here — see Entry.PhoneticUK/
// PhoneticUS/PhoneticOther for why pronunciation is modeled at word
// scope.
type Variant struct {
	Etymology   string       `json:"etymology,omitempty"`
	Definitions []Definition `json:"definitions"`
	SenseCount  int          `json:"senseCount"`
}

// Entry is the full Wiktionary-derived record for one word: a word-level
// phonetic (PhoneticUK/PhoneticUS/PhoneticOther) plus one or more
// Variants, one per distinct etymology found in the source. Most words
// have exactly one Variant.
//
// PhoneticUK and PhoneticUS are extracted from a first-pass dialect tag
// whitelist (UK: "UK"/"Received-Pronunciation"/"British"; US:
// "US"/"General-American"). PhoneticOther holds the first IPA found that
// carried a different tag or none at all, covering both untagged
// transcriptions and real third dialects (e.g. Australian, Scottish)
// under one field..
//
// Phonetics are modeled at word scope (here, on Entry) rather than per
// Variant. This reflects an empirical finding: wiktextract duplicates
// the identical sounds[] array across etymology sections in 85.2% of
// multi-etymology words (3,507 of 4,115 in the wordlist-filtered
// corpus) — first observed with "name", where the yam etymology carried
// a UK pronunciation that actually belongs to the identifier etymology.
// Given that frequency, the generator aggregates sounds[] across ALL of
// a word's raw entries (every etymology_number) before assigning
// PhoneticUK/US/Other, producing one word-level pronunciation set that
// reflects what the source data reliably supports. See the trade-off
// document's "Known Trade-offs and Limitations" section for the
// resulting scope: a genuinely different etymology with its own
// distinct pronunciation, on the occasions the source does provide that
// distinction cleanly, is represented by the same word-level fields as
// its sibling etymologies.
type Entry struct {
	Word          string    `json:"word"`
	PhoneticUK    string    `json:"phoneticUK,omitempty"`
	PhoneticUS    string    `json:"phoneticUS,omitempty"`
	PhoneticOther string    `json:"phoneticOther,omitempty"`
	Variants      []Variant `json:"variants"`
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

// GetCurated returns the hand-curated entry for word (case-insensitive
// lookup), or false if it's not among the curated set.
func GetCurated(word string) (CuratedEntry, bool) {
	e, ok := curatedData[strings.ToLower(word)]
	return e, ok
}

// Get returns the Wiktionary-derived entry for word (case-insensitive
// lookup), or false if the word is not in the dataset. It returns the
// stored record as-is; combining it with GetCurated's result for the
// same word, or selecting among multiple Variants, is the calling
// service's responsibility.
func Get(word string) (Entry, bool) {
	e, ok := dictionaryData[strings.ToLower(word)]
	return e, ok
}
