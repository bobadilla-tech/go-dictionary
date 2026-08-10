// wiktionary-coverage-check
//
// Streams the raw wiktextract JSONL dump (raw-wiktextract-data.jsonl),
// filters to English entries (lang_code == "en") whose word appears in
// -wordlist, and generates the full dictionary JSON: one Entry per word,
// with one Variant per distinct etymology (grouped by etymology_number,
// not by part of speech — see the trade-off document for why).
//
// Usage:
//
//	go run main.go -in raw-wiktextract-data.jsonl -wordlist en_50k.txt -out dictionary.json
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
)

// ---- raw wiktextract shapes (input) ----

type rawSound struct {
	IPA  string   `json:"ipa"`
	Tags []string `json:"tags"`
}

type rawSense struct {
	Glosses  []string          `json:"glosses"`
	Examples []json.RawMessage `json:"examples"`
	Tags     []string          `json:"tags"`
}

type rawEntry struct {
	Word            string     `json:"word"`
	Pos             string     `json:"pos"`
	LangCode        string     `json:"lang_code"`
	EtymologyNumber string     `json:"etymology_number"`
	EtymologyText   string     `json:"etymology_text"`
	Sounds          []rawSound `json:"sounds"`
	Senses          []rawSense `json:"senses"`
}

// extractedExample holds the text and type of one example entry. type is
// typically "example" (a short illustrative sentence) or "quotation" (a
// literary/historical citation, often longer and more archaic) —
// "example" is preferred when building a Variant's definitions.
type extractedExample struct {
	Text string
	Type string
}

func extractExample(raw json.RawMessage) extractedExample {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return extractedExample{Text: asString}
	}
	var asObject struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return extractedExample{Text: asObject.Text, Type: asObject.Type}
	}
	return extractedExample{}
}

// cleanEtymologyText strips two kinds of machine-generated noise
// wiktextract adds around the readable prose sentence for words with deep
// PIE ancestry (e.g. "dictionary", "name"):
//
//  1. A leading "Etymology tree" block — a line-by-line rendering of the
//     full ancestor chain (Proto-Indo-European down to English).
//  2. A trailing "Cognates" section — a long list of related words across
//     dozens of languages.
//
// The readable sentence always starts with "From " once the tree ends, so
// this keeps only the text from the LAST occurrence of "\nFrom " onward,
// then cuts again at the next "\nCognates" if present. If neither pattern
// is found (most words have no tree/cognates block), the text is returned
// unchanged — this is a best-effort cleanup, not a guaranteed one.
func cleanEtymologyText(raw string) string {
	text := raw
	if idx := strings.LastIndex(text, "\nFrom "); idx != -1 {
		text = strings.TrimSpace(text[idx+1:])
	}
	if idx := strings.Index(text, "\nCognates"); idx != -1 {
		text = strings.TrimSpace(text[:idx])
	}
	return text
}

// ---- pkg/dictionary-data target shape (output) ----

type Definition struct {
	PartOfSpeech string   `json:"partOfSpeech"`
	Definition   string   `json:"definition"`
	Examples     []string `json:"examples,omitempty"`
}

type Variant struct {
	Etymology     string       `json:"etymology,omitempty"`
	PhoneticUK    string       `json:"phoneticUK,omitempty"`
	PhoneticUS    string       `json:"phoneticUS,omitempty"`
	PhoneticOther string       `json:"phoneticOther,omitempty"`
	Definitions   []Definition `json:"definitions"`
	SenseCount    int          `json:"senseCount"`
}

type Entry struct {
	Word     string    `json:"word"`
	Variants []Variant `json:"variants"`
}

// sensitiveTagSet lists sense-level tags that exclude a sense from the
// dataset. Chosen from real frequency data across the wordlist-filtered
// corpus: these are the tags Wiktionary itself uses to flag content
// needing special handling — not general register tags like "slang" or
// "euphemistic", which are common and mostly not sensitive in nature.
// Known limitation, accepted: this only catches senses Wiktionary tagged
// consistently — coverage is not perfect (e.g. some slang-only senses
// with sensitive content are not caught).
var sensitiveTagSet = map[string]bool{
	"vulgar":     true,
	"derogatory": true,
	"offensive":  true,
	"slur":       true,
}

func hasSensitiveTag(tags []string) bool {
	for _, t := range tags {
		if sensitiveTagSet[t] {
			return true
		}
	}
	return false
}

func main() {
	inPath := flag.String("in", "raw-wiktextract-data.jsonl", "path to the decompressed raw wiktextract JSONL dump")
	outPath := flag.String("out", "dictionary.json.gz", "where to write the full generated dictionary, gzip-compressed")
	wordlistPath := flag.String("wordlist", "en_50k.txt", "path to a plain text file, one word per line or 'word count' per line (e.g. en_50k.txt)")
	flag.Parse()

	wordlist, err := loadWordlist(*wordlistPath)
	if err != nil {
		log.Fatalf("loading wordlist: %v", err)
	}
	fmt.Printf("Loaded wordlist with %d words from %s\n", len(wordlist), *wordlistPath)

	f, err := os.Open(*inPath)
	if err != nil {
		log.Fatalf("opening input file: %v", err)
	}
	defer f.Close()

	// Lines in this dump can be long. bufio.Scanner's default 64KB token
	// limit is too small for some entries, so we read with bufio.Reader
	// and ReadString instead, which has no such ceiling.
	reader := bufio.NewReaderSize(f, 1<<20) // 1MB read buffer

	wordGroups := make(map[string][]rawEntry)
	var wordOrder []string
	totalLines := 0

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			totalLines++
			if entry, ok := processLine(line); ok {
				// Only match words that are ALREADY lowercase in the
				// source. Wiktextract stores common lowercase words
				// as-is (e.g. "name", "head"), while acronyms and proper
				// nouns keep their original casing (e.g. "NAmE", "NATO").
				// Lowercasing before comparing — without this guard —
				// collapses "NAmE" into "name".
				if shouldIncludeWord(entry.Word, wordlist) {
					groupKey := entry.Word
					if _, seen := wordGroups[groupKey]; !seen {
						wordOrder = append(wordOrder, groupKey)
					}
					wordGroups[groupKey] = append(wordGroups[groupKey], entry)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("reading line %d: %v", totalLines, err)
		}
	}
	fmt.Printf("Read %d total lines, matched %d words from the wordlist\n", totalLines, len(wordOrder))

	var entries []Entry
	for _, word := range wordOrder {
		rawVariants := wordGroups[word]
		entry := Entry{Word: word}

		// Group by etymology_number, NOT by pos. Wiktionary sections
		// pages by etymology: entries that share an etymology_number
		// (including all being empty, i.e. the word has only one
		// etymology) belong to the same origin and flatten into one
		// Variant with mixed pos — matching how "melancholy" mixes
		// noun+adjective in the existing curated dataset. A genuinely
		// different etymology_number (e.g. "name" the identifier vs.
		// "name" the Caribbean yam) must NOT merge.
		etymGroups := make(map[string][]rawEntry)
		var etymOrder []string
		for _, re := range rawVariants {
			key := re.EtymologyNumber
			if _, exists := etymGroups[key]; !exists {
				etymOrder = append(etymOrder, key)
			}
			etymGroups[key] = append(etymGroups[key], re)
		}
		sort.Slice(etymOrder, func(i, j int) bool {
			if etymOrder[i] == "" {
				return true
			}
			if etymOrder[j] == "" {
				return false
			}
			return etymOrder[i] < etymOrder[j]
		})

		for _, key := range etymOrder {
			v := toVariant(etymGroups[key])
			if len(v.Definitions) > 0 {
				entry.Variants = append(entry.Variants, v)
			}
		}

		if len(entry.Variants) > 0 {
			entries = append(entries, entry)
		}
	}

	if err := writeJSON(*outPath, entries); err != nil {
		log.Fatalf("writing output file: %v", err)
	}
	fmt.Printf("Wrote %d Entry objects to %s\n", len(entries), *outPath)
}

// shouldIncludeWord reports whether word should be grouped into the
// output, given the -wordlist lookup set. Two conditions must both hold:
//
//  1. word is in the wordlist (case-insensitive on the wordlist side —
//     see loadWordlist, which lowercases every entry it loads).
//  2. word is ALREADY lowercase in the source, as-is. Wiktextract stores
//     common lowercase words unchanged (e.g. "name", "head"), while
//     acronyms and proper nouns keep their original casing (e.g.
//     "NAmE", "NATO"). Without this second condition, lowercasing word
//     before comparing would collapse "NAmE" into "name" and silently
//     merge an unrelated abbreviation into the common word's results —
//     this was a real bug found during dataset verification.
func shouldIncludeWord(word string, wordlist map[string]bool) bool {
	if word != strings.ToLower(word) {
		return false
	}
	return wordlist[word]
}

// processLine parses one JSONL line and returns the entry if it passed the
// lang_code == "en" filter. Malformed lines are skipped, not fatal.
func processLine(line string) (rawEntry, bool) {
	var e rawEntry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return rawEntry{}, false
	}
	if e.LangCode != "en" || e.Word == "" {
		return rawEntry{}, false
	}
	return e, true
}

// toVariant converts a group of raw wiktextract entries that share the
// same etymology_number into a single Variant.
//
// Dialect extraction is a first-pass rule (UK/US/Other), based on real
// tag-frequency data: UK = "UK", "Received-Pronunciation", "British";
// US = "US", "General-American". Everything else (Australian, Canada,
// Scotland, register qualifiers, etc.) falls into PhoneticOther as a
// single value — a multi-dialect map was considered and rejected as
// out of scope (only UK/US were required).
//
// Definitions are deduplicated by gloss text across all entries in the
// group, because real entries (e.g. "head") repeat the identical gloss
// across many senses — one per citation — rather than nesting multiple
// citations under one sense. Within each gloss group, examples of type
// "example" are preferred over type "quotation", deduplicated by exact
// text, capped at 5.
//
// Senses tagged with a sensitiveTagSet entry are excluded entirely.
func toVariant(entries []rawEntry) Variant {
	var v Variant

	type glossGroup struct {
		partOfSpeech   string
		definition     string
		examples       []string
		hasExampleType bool
	}
	groups := make(map[string]*glossGroup)
	var order []string

	for _, e := range entries {
		if v.Etymology == "" && e.EtymologyText != "" {
			v.Etymology = cleanEtymologyText(e.EtymologyText)
		}
		for _, s := range e.Sounds {
			if s.IPA == "" {
				continue
			}
			tagged := false
			for _, t := range s.Tags {
				switch t {
				case "US", "General-American":
					if v.PhoneticUS == "" {
						v.PhoneticUS = s.IPA
					}
					tagged = true
				case "UK", "Received-Pronunciation", "British":
					if v.PhoneticUK == "" {
						v.PhoneticUK = s.IPA
					}
					tagged = true
				}
			}
			if !tagged && v.PhoneticOther == "" {
				v.PhoneticOther = s.IPA
			}
		}

		for _, s := range e.Senses {
			if len(s.Glosses) == 0 {
				continue
			}
			if hasSensitiveTag(s.Tags) {
				continue
			}
			gloss := s.Glosses[0]
			key := strings.ToLower(strings.TrimSpace(gloss))
			if key == "" {
				continue
			}

			g, exists := groups[key]
			if !exists {
				g = &glossGroup{partOfSpeech: e.Pos, definition: gloss}
				groups[key] = g
				order = append(order, key)
			}

			for _, rawEx := range s.Examples {
				ex := extractExample(rawEx)
				if ex.Text == "" {
					continue
				}

				alreadyExists := false
				for _, existing := range g.examples {
					if existing == ex.Text {
						alreadyExists = true
						break
					}
				}
				if alreadyExists {
					continue
				}

				isExampleType := ex.Type == "example"
				if isExampleType {
					g.examples = append(g.examples, ex.Text)
					g.hasExampleType = true
				} else if !g.hasExampleType && len(g.examples) < 2 {
					g.examples = append(g.examples, ex.Text)
				}
			}
		}
	}

	for _, key := range order {
		g := groups[key]
		if len(g.examples) > 5 {
			g.examples = g.examples[:5]
		}
		v.Definitions = append(v.Definitions, Definition{
			PartOfSpeech: g.partOfSpeech,
			Definition:   g.definition,
			Examples:     g.examples,
		})
	}
	v.SenseCount = len(v.Definitions)

	return v
}

// loadWordlist reads a plain text file and returns a lowercase lookup set.
// Two formats are supported per line: a bare word, or "word count"
// separated by whitespace (e.g. hermitdave/FrequencyWords' en_50k.txt) —
// only the first field is used. Blank lines and lines starting with "#"
// are skipped.
func loadWordlist(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	words := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		words[strings.ToLower(fields[0])] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return words, nil
}

func writeJSON(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	enc := json.NewEncoder(gz)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
