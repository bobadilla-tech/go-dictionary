# Dictionary Go

A lightweight Go package for English dictionary lookups using an embedded,
Wiktionary-derived dataset.

## Features

- **Zero runtime dependencies**: pure Go, standard library only

- **Embedded dataset**: a Wiktionary-derived dataset (via `wiktextract`)
  compiled into the binary using `go:embed`; no external files required at
  runtime

- **Compressed embedded data**: the generated dataset is stored as
  gzip-compressed JSON to reduce binary size

- **Reproducible builds**: the raw dump used to generate the dataset is
  pinned by hash in `source.lock` — see [Regenerating the
  Dataset](#regenerating-the-dataset)

- **Rich, unopinionated shape**: the dataset exposes multiple etymologies per
  word (`Variant`), multiple dialects (UK/US/Other), and multiple examples per
  sense — this package does not collapse or select among them; see
  [How It Works](#how-it-works)

- **Two independent sources, no automatic merge**: a small hand-curated
  dataset (`words.json`) and the Wiktionary-derived dataset
  (`dictionary.json.gz`) are both embedded and exposed through separate
  getters — this package makes no decision about which one should win for a
  given word; see [Design Note](#design-note-two-sources-no-merged-lookup)

- **Case-insensitive lookups**: input is normalized to lowercase before
  lookup

- **Well tested**: dataset-generation logic (deduplication, phonetics
  extraction, sensitive-content filtering, etymology-text cleanup) is
  covered by unit tests in `cmd/datasetbuild`

## Installation

```bash
go get github.com/bobadilla-tech/go-dictionary
```

## Usage

```go
package main

import (
	"fmt"
	"log"

	dictionary "github.com/bobadilla-tech/go-dictionary"
)

func main() {
	entry, ok := dictionary.Get("name")
	if !ok {
		log.Fatal("word not found")
	}

	fmt.Println("UK:", entry.PhoneticUK, "US:", entry.PhoneticUS)

	for _, variant := range entry.Variants {
		fmt.Println("Etymology:", variant.Etymology)
		fmt.Println("Definitions:", len(variant.Definitions))
	}
}
```

For the hand-curated dataset:

```go
curated, ok := dictionary.GetCurated("ephemeral")
if ok {
	fmt.Println("Phonetic:", curated.Phonetic)
}
```

## API

```go
func Get(word string) (Entry, bool)
```

Returns the raw Wiktionary-derived record for `word`, or `ok == false` if the
word isn't in the dataset.

```go
type Entry struct {
	Word          string
	PhoneticUK    string       // empty if no UK-tagged pronunciation was found
	PhoneticUS    string       // empty if no US-tagged pronunciation was found
	PhoneticOther string       // first non-UK/US IPA found, if any — see Known Limitations
	Variants      []Variant
}

type Variant struct {
	Etymology   string       // readable prose, cleaned of generated noise — see How It Works
	Definitions []Definition
	SenseCount  int          // len(Definitions) — a signal for callers choosing among Variants
}

type Definition struct {
	PartOfSpeech string
	Definition   string
	Examples     []string // 0 to 5 example sentences, deduplicated
}
```

`PhoneticUK`/`PhoneticUS`/`PhoneticOther` live on `Entry`, not on `Variant` —
they describe the word's pronunciation as a whole, not one specific to a
given etymology. This is deliberate, not an oversight: see [How It
Works](#how-it-works) for why the source data doesn't reliably support
scoping pronunciation to a single etymology.

Most words have exactly one `Variant`. More than one means a genuine
homograph with unrelated etymologies — e.g. `"name"` (the identifier), `"name"`
the verb, and `"name"` the Caribbean yam (borrowed from Spanish *ñame*) are
three separate `Variant`s under one `Entry`. This package does not decide
which one to show; see [How It Works](#how-it-works).

```go
func GetCurated(word string) (CuratedEntry, bool)
```

Returns the hand-curated entry for `word`, or `ok == false` if it's not among
the curated set (~30 words as of writing).

```go
type CuratedEntry struct {
	Phonetic    string
	Definitions []CuratedDefinition
	Synonyms    []string
}

type CuratedDefinition struct {
	PartOfSpeech string
	Definition   string
	Example      string
}
```

## Design Note: Two Sources, No Merged Lookup

Unlike [`thesaurus-go`](https://github.com/bobadilla-tech/thesaurus-go)'s
single `Lookup()` — which internally applies curated-first, OEWN-fallback
precedence and returns one merged `Entry` — this package deliberately exposes
`Get` and `GetCurated` as two independent, unopinionated getters, with no
merge logic and no precedence decision baked in.

This is intentional: this package is scoped to **data access only**. Which
source wins for a given word, how multiple `Variant`s are collapsed into a
single API response, and which dialect (UK/US/Other) is shown are all
decisions specific to a consumer's response contract — they belong in the
calling service, not in this package. This keeps the dataset swappable and
reusable by a different consumer without carrying lookup semantics tied to
one particular API shape.

## How It Works

The embedded dataset is generated by `cmd/datasetbuild` from the **raw**
`wiktextract` JSONL dump (English Wiktionary, extracted via the
[`wiktextract`](https://github.com/tatuylonen/wiktextract) tool, distributed
by [kaikki.org](https://kaikki.org/dictionary/rawdata.html) — the raw feed
was chosen over the postprocessed one because the postprocessed download
links are actively being deprecated by the project, and the raw feed's
license chain is cleaner: no "merged from other sources" caveat the
postprocessed dataset carries).

Several decisions were made while building the generator, based on
inspecting real entries rather than assuming the source's documented schema
matched its actual content:

- **Filtered to a common-vocabulary wordlist before generating**, not the
  full ~1.48M-entry raw feed. An empirical pass showed phonetics/example
  coverage across the *unfiltered* dump was worse than the previous OEWN
  source (9.3% vs. OEWN's 27.6%), because the bulk of Wiktionary's English
  entries are long-tail technical/rare terms with little editorial
  attention. Restricted to a
  [50k common-word list](https://github.com/hermitdave/FrequencyWords),
  coverage rose to 60.6% — well above OEWN — confirming the low global
  numbers were a long-tail artifact, not a real limitation for the
  vocabulary this package actually serves.

- **Entries are grouped by word AND `etymology_number`, not by
  word+part-of-speech.** Wiktionary sections pages by etymology, and a
  single etymology commonly spans multiple parts of speech (e.g.
  "melancholy" as both noun and adjective) — these are flattened into one
  `Variant`. A genuinely different etymology (e.g. "name" the identifier
  vs. "name" the yam) produces a separate `Variant`, never merged into the
  other, even when they share a part of speech.

- **Phonetics are extracted once per word, across every etymology group —
  not once per `Variant`.** An empirical check found wiktextract duplicates
  the identical `sounds[]` array across etymology sections in 85.2% of
  multi-etymology words (3,507 of 4,115 in the wordlist-filtered corpus)
  rather than genuinely scoping pronunciation per meaning — first noticed
  on `"name"`, where the yam etymology carried a UK pronunciation that
  actually belongs to the identifier etymology. Attaching phonetics to
  `Variant` would therefore misattribute a dialect transcription to the
  wrong sense far more often than it would correctly attribute it, so
  `PhoneticUK`/`PhoneticUS`/`PhoneticOther` are extracted at word scope
  onto `Entry` instead, aggregating `sounds[]` across all of a word's raw
  entries. When multiple sounds share a dialect, the first one encountered
  in source order wins — deterministic and stable across rebuilds of the
  same pinned dump, since the input is fixed by `source.lock`.

- **UK/US dialect tags were mapped from real frequency data, not
  assumption.** An inventory pass over `sounds[].tags` across the
  wordlist-filtered corpus showed the tags `General-American` and
  `Received-Pronunciation`/`British` carried far more volume than the bare
  `US`/`UK` tags alone — both are now recognized. Everything else
  (Australian, Canadian, Scottish, register qualifiers like "dialectal" or
  "archaic", etc.) falls into `PhoneticOther` as a single value; a
  multi-dialect map was considered and explicitly rejected as out of
  scope — only UK/US were required.

- **Definitions are deduplicated by part of speech + gloss text
  together, not gloss alone.** Some real entries (e.g. "head") repeat the
  identical definition text across many raw senses — one per citation —
  rather than nesting multiple citations under a single sense; those
  collapse into one `Definition`, keeping up to 5 example sentences,
  deduplicated by exact text. But a single `Variant` can also mix multiple
  parts of speech (grouping is by etymology, not by pos — see above), so
  two senses can legitimately share identical gloss text while being
  genuinely different senses (e.g. `"3rd"` as an abbreviated adjective vs.
  verb). An empirical check found 434 such word/etymology groups in the
  50k-word wordlist (out of 37,357 matched words) — deduplicating on gloss
  alone would have silently dropped one of the two. Sense-level tags
  (e.g. `dialectal`, `obsolete`) are not part of the dedup key: a check of
  same-pos/same-gloss senses with differing tags found the large majority
  are grammatical-usage tags (`countable`/`uncountable`,
  `transitive`/`intransitive`) that the source records once per valid
  usage combination rather than as genuinely distinct senses, so including
  tags in the key would reintroduce far more false "duplicates" than
  distinctions preserved.

- **Example sentences of type `"example"` are preferred over
  `"quotation"`** when both exist for the same sense — short illustrative
  sentences over long, often archaic literary citations.

- **Etymology text is cleaned of two kinds of machine-generated noise**
  wiktextract prepends/appends for words with deep Proto-Indo-European
  ancestry: a leading "Etymology tree" block (a line-by-line rendering of
  the full ancestor chain) and a trailing "Cognates" block (a list of
  related words across dozens of languages). Only the readable prose
  sentence in between is kept. This is a best-effort heuristic based on
  patterns observed in real entries, not a guaranteed parse of every
  possible etymology format.

- **Senses tagged `vulgar`, `derogatory`, `offensive`, or `slur` are
  excluded.** These four were chosen from real tag-frequency data as the
  tags Wiktionary itself uses to flag content needing special handling —
  deliberately not `slang` or `euphemistic`, which are common (7,000+
  occurrences) and mostly not sensitive in nature. See
  [Known Limitations](#known-limitations) for what this filter does not
  catch.

- **A lowercase guard prevents acronym/proper-noun collisions with common
  words.** Wiktextract stores common lowercase words as-is (`"name"`,
  `"head"`) but keeps acronyms and proper nouns in their original casing
  (`"NAmE"`, `"NATO"`). Matching against the wordlist without requiring the
  source word to already be lowercase caused `"NAmE"` (abbreviation for
  *North American English*) to silently merge into `"name"`'s results — a
  real bug found during verification, now covered by a test.

## Known Limitations

- **Phonetic dialect coverage is UK/US only, by design.** `PhoneticOther`
  does not distinguish "no dialect tag at all" from "a real third dialect
  (e.g. Australian) that wasn't specifically requested." Both land in the
  same field.
- **Phonetics are word-scoped, not etymology-scoped, so they cannot be
  attributed to a specific `Variant`.** As covered in [How It
  Works](#how-it-works), the source data does not reliably scope
  pronunciation per etymology (85.2% of multi-etymology words duplicate
  the same `sounds[]` across etymology sections), so `PhoneticUK`/`US`/
  `Other` live on `Entry` rather than per-`Variant`. The trade-off this
  accepts: a genuinely different etymology with its own distinct
  pronunciation, on the rare occasions the source does provide that
  distinction cleanly, is not preserved separately — a caller cannot infer
  which etymology/sense a given phonetic value "belongs to."
- **Sense-level tags (dialectal, obsolete, archaic, etc.) are not
  reflected in the response.** Two senses with the same part of speech and
  gloss but different tags collapse into one `Definition`, merging their
  examples. This was a deliberate choice after empirical review found most
  same-gloss/same-pos tag differences are grammatical-usage variants
  (`countable`/`uncountable`), not distinct meanings — see [How It
  Works](#how-it-works). A smaller subset of genuinely distinct
  regional/register senses (e.g. `UK` vs. `Canada`/`US`,
  `dialectal`/`obsolete`) is merged away as a result.
- **The sensitive-content filter only catches what Wiktionary tagged
  explicitly.** Coverage depends on how consistently the source tagged a
  given sense; some sensitive senses without one of the four filtered tags
  (e.g. tagged only `slang`) will still appear.
- **`Etymology` cleanup is a heuristic**, not a guaranteed parse — formats
  not seen during verification (only "Etymology tree" and "Cognates" blocks
  were confirmed and handled) may still leak through unclean.

## Project Layout

```text
go-dictionary/
├── data.go                    ← package source (this file's home), go:embed directives live here
├── go.mod
├── source.lock                ← pins the raw dump used to generate dictionary.json.gz:
│                                  source URL, fetch timestamp, SHA-256 — see Regenerating
│                                  the Dataset
├── dataset/                   ← EMBEDDED via go:embed — consumed at runtime
│   ├── curated.json           ← hand-curated dataset (renamed from words.json)
│   └── dictionary.json.gz     ← generated Wiktionary-derived dataset
├── data/                      ← NOT embedded — build-time inputs to the generator only
│   ├── en_50k.txt              (hermitdave/FrequencyWords wordlist)
│   └── raw-wiktextract-data.jsonl  (decompressed raw wiktextract dump, ~23GB — not committed)
└── cmd/
    └── datasetbuild/          ← the generator that produces dataset/dictionary.json.gz
        ├── main.go
        ├── lock.go            ← source.lock generation/verification (-generate-lock, -skip-verify)
        └── main_test.go
```

`data/` and `dataset/` are deliberately separate: `data/` holds the large,
build-time-only inputs the generator reads (the raw dump is far too large to
embed or commit — see `.gitignore`); `dataset/` holds only the small,
already-processed outputs the compiled binary actually embeds.

## Regenerating the Dataset

The raw dump is pinned by `source.lock` (source URL, fetch timestamp,
SHA-256) so builds are reproducible and a mismatched or stale dump is
caught before it silently produces a different dataset. The dump itself
(~23GB) is not committed — see [Project Layout](#project-layout).

**First time, or after intentionally updating to a newer dump:**

```bash
go run ./cmd/datasetbuild -in data/raw-wiktextract-data.jsonl -generate-lock
```

This hashes the dump and writes/updates `source.lock`. Commit `source.lock`
(not the dump) after running this.

**Normal build**, verifying the dump against `source.lock` before
generating anything:

```bash
go run ./cmd/datasetbuild \
  -in data/raw-wiktextract-data.jsonl \
  -wordlist data/en_50k.txt \
  -out dataset/dictionary.json.gz
```

If the dump's hash doesn't match `source.lock`, the build aborts with a
descriptive error instead of generating from an unexpected source. Pass
`-skip-verify` to bypass this for local experimentation — not recommended
for reproducible or production builds.

`data/raw-wiktextract-data.jsonl` is the decompressed raw dump from
[kaikki.org/dictionary/rawdata.html](https://kaikki.org/dictionary/rawdata.html)
(large — not committed to the repository).
`data/en_50k.txt` is a word-frequency list in `word count` or
one-word-per-line format (e.g.
[`hermitdave/FrequencyWords`](https://github.com/hermitdave/FrequencyWords)'s
`en_50k.txt`).

`dataset/dictionary.json.gz` and `dataset/curated.json` (the hand-curated
dataset) are consumed automatically through `go:embed` at build time.

## Testing

Run the tests:

```bash
go test -v ./...
```

Test coverage focuses on the generator logic in `cmd/datasetbuild` — gloss
deduplication (by part of speech + gloss, including the case where
identical gloss text spans different parts of speech), example-type
preference, phonetics extraction (word-scoped, aggregating across
etymology groups), sensitive-tag filtering, etymology-text cleanup, and the
lowercase acronym guard — since that is where the real bugs found during
dataset verification lived.

## Used in Production

This package powers the
[Dictionary](https://requiems.xyz/en/apis/dictionary) endpoint on
[Requiems API](https://requiems.xyz), an all-in-one backend API for SaaS
products (auth, fraud detection, payments intelligence, global data, data
integrity).

- Full API docs: https://requiems.xyz/en/apis
- Systems overview: https://requiems.xyz/en/systems

Need more language tooling? Requiems API's **Text & Language** system also
provides thesaurus lookups, spell checking, language detection, sentiment
analysis, text similarity, and more through a single API.

## License

This project's code is licensed under the MIT License.

## Credits

- Word data derived from **Wiktionary**, via the
  [`wiktextract`](https://github.com/tatuylonen/wiktextract) tool
  (MIT-licensed), raw data distributed by
  [kaikki.org](https://kaikki.org/dictionary/rawdata.html).
- Wiktionary content is made available under the same licenses as
  Wiktionary itself — **CC BY-SA** and **GFDL**.
- The common-word filter list is
  [`hermitdave/FrequencyWords`](https://github.com/hermitdave/FrequencyWords)
  (MIT-licensed), derived from OpenSubtitles frequency data.