package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestAnswerMatches(t *testing.T) {
	tests := []struct {
		name, answer, correct string
		want                  bool
	}{
		{name: "exact match", answer: "auf", correct: "auf", want: true},
		{name: "trimmed spaces", answer: "  AUF ", correct: "auf", want: true},
		{name: "case insensitive umlaut", answer: "AKKUSATIV", correct: "Akkusativ", want: true},
		{name: "wrong answer", answer: "an", correct: "auf", want: false},
		{name: "internal spacing remains significant", answer: "warten  auf + Akkusativ", correct: "warten auf + Akkusativ", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := answerMatches(test.answer, test.correct); got != test.want {
				t.Errorf("answerMatches(%q, %q) = %v, want %v", test.answer, test.correct, got, test.want)
			}
		})
	}
}

func TestValidRektionType(t *testing.T) {
	for _, test := range []struct {
		name, value string
		want        bool
	}{
		{name: "preposition", value: "preposition", want: true},
		{name: "case", value: "case", want: true},
		{name: "full pattern removed", value: "full", want: false},
		{name: "unknown", value: "random", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validRektionType(test.value); got != test.want {
				t.Errorf("validRektionType(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestRektionDatasetLoadsAndValidates(t *testing.T) {
	path := filepath.Join("..", "..", "backend", "data", "rektion.json")
	entries, err := loadRektion(path)
	if err != nil {
		t.Fatalf("loadRektion(%q): %v", path, err)
	}
	if len(entries) < 30 {
		t.Fatalf("loaded only %d entries; want at least 30", len(entries))
	}
	batches := []struct {
		name       string
		start, end int
	}{
		{name: "original migrated batch", start: 0, end: 30},
		{name: "A2 additions", start: 30, end: 65},
		{name: "B1 additions first batch", start: 65, end: 95},
		{name: "B1 additions second batch", start: 95, end: 115},
		{name: "B2 additions first batch", start: 115, end: 145},
		{name: "B2 additions second batch", start: 145, end: 150},
	}
	for _, batch := range batches {
		t.Run(batch.name, func(t *testing.T) {
			if batch.end > len(entries) {
				t.Fatalf("dataset has %d entries; batch ends at %d", len(entries), batch.end)
			}
			if err := validateRektionEntries(entries[batch.start:batch.end]); err != nil {
				t.Fatalf("invalid batch: %v", err)
			}
		})
	}
}

func TestValidateRektionEntriesRejectsInvalidData(t *testing.T) {
	base := RektionEntry{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A2", Translation: "to wait for", Examples: []string{"Ich warte {auf} den Bus."}}
	tests := []struct {
		name    string
		entries []RektionEntry
	}{
		{name: "duplicate ids", entries: []RektionEntry{base, base}},
		{name: "id does not encode pattern", entries: []RektionEntry{{ID: "warten-an-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A2", Translation: "wait", Examples: []string{"Ich warte {auf}."}}}},
		{name: "invalid case", entries: []RektionEntry{{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "nominativ", Level: "A2", Translation: "wait", Examples: []string{"Ich warte {auf}."}}}},
		{name: "invalid level", entries: []RektionEntry{{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A6", Translation: "wait", Examples: []string{"Ich warte {auf}."}}}},
		{name: "gap differs from preposition", entries: []RektionEntry{{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A2", Translation: "wait", Examples: []string{"Ich warte {an}."}}}},
		{name: "multiple gaps", entries: []RektionEntry{{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A2", Translation: "wait", Examples: []string{"Ich warte {auf} {auf}."}}}},
		{name: "non-empty preposition without example", entries: []RektionEntry{{ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "A2", Translation: "wait"}}},
		{name: "reflexive sample lacks pronoun", entries: []RektionEntry{{ID: "freuen-auf-akk", Verb: "freuen", Preposition: "auf", Case: "akkusativ", Reflexive: true, Level: "A2", Translation: "look forward", Examples: []string{"Er freut auf den Urlaub."}}}},
		{name: "duplicate pattern", entries: []RektionEntry{base, {ID: "warten-auf-akk", Verb: "warten", Preposition: "auf", Case: "akkusativ", Level: "B1", Translation: "wait", Examples: []string{"Sie wartet {auf} den Bus."}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRektionEntries(test.entries); err == nil {
				t.Fatal("validateRektionEntries() accepted invalid data")
			}
		})
	}
}

func TestAcceptedPrepositionsAndMarkedGaps(t *testing.T) {
	entry := RektionEntry{ID: "helfen-bei-dat", Verb: "helfen", Preposition: "bei", AcceptedPrepositions: []string{"mit"}, Case: "dativ", Examples: []string{"Kannst du mir {bei} der Aufgabe helfen?"}}
	if !answerMatchesAny(" MIT ", rektionValidAnswers(entry, "preposition")) {
		t.Fatal("accepted preposition was rejected")
	}
	if got := gapPreposition(entry); got != "Kannst du mir ___ der Aufgabe helfen?" {
		t.Fatalf("gapPreposition() = %q", got)
	}
	directGenitive := "Das Projekt bedarf {} einer klaren Struktur."
	if got := cleanExample(directGenitive); got != "Das Projekt bedarf einer klaren Struktur." {
		t.Fatalf("cleanExample() = %q", got)
	}
}

func TestRequestedAcceptedPrepositions(t *testing.T) {
	path := filepath.Join("..", "..", "backend", "data", "rektion.json")
	entries, err := loadRektion(path)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]RektionEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	tests := []struct {
		id, canonical string
		accepted      []string
	}{
		{"bewerben-um-akk", "um", []string{"auf"}},
		{"streiten-ueber-akk", "über", []string{"um"}},
		{"leiden-an-dat", "an", []string{"unter"}},
		{"anmelden-fuer-akk", "für", []string{"zu"}},
		{"arbeiten-bei-dat", "bei", []string{"in"}},
		{"einigen-auf-akk", "auf", []string{"über"}},
		{"einigen-ueber-akk", "über", []string{"auf"}},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			entry := byID[test.id]
			if entry.Preposition != test.canonical || strings.Join(entry.AcceptedPrepositions, "|") != strings.Join(test.accepted, "|") {
				t.Fatalf("data pattern is %q with accepted %v; want %q with accepted %v", entry.Preposition, entry.AcceptedPrepositions, test.canonical, test.accepted)
			}
			answers := rektionValidAnswers(entry, "preposition")
			cases := []struct {
				name, answer string
				want         bool
			}{
				{"canonical", test.canonical, true},
				{"case and whitespace ignored", "  " + strings.ToUpper(test.canonical) + "  ", true},
				{"wrong answer", "falsch", false},
			}
			for _, accepted := range test.accepted {
				cases = append(cases, struct {
					name, answer string
					want         bool
				}{"accepted alternative", "  " + strings.ToUpper(accepted) + " ", true})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					if got := answerMatchesAny(tc.answer, answers); got != tc.want {
						t.Errorf("answerMatchesAny(%q, %v) = %v, want %v", tc.answer, answers, got, tc.want)
					}
				})
			}
		})
	}
	thinking := byID["denken-an-akk"]
	if len(thinking.AcceptedPrepositions) != 0 || !strings.Contains(thinking.Notes, "Denken \u00fcber") {
		t.Errorf("denken-an-akk should retain no accepted alternative and note the distinct meaning: %#v", thinking)
	}
}

func TestGapCandidatesExcludeDisabledAndDirectGenitive(t *testing.T) {
	path := filepath.Join("..", "..", "backend", "data", "rektion.json")
	entries, err := loadRektion(path)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]RektionEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	for _, id := range []string{"beduerfen-ohne-praep-gen", "enthalten-ohne-praep-gen"} {
		entry := byID[id]
		if entry.Preposition != "" || len(entry.Examples) == 0 || strings.Count(entry.Examples[0], "{}") != 1 {
			t.Errorf("%s should have empty preposition and one {} marker: %#v", id, entry)
		}
	}
	candidates := rektionCandidates(entries, "", "preposition")
	for _, candidate := range candidates {
		if !usesGapExercise(candidate) || candidate.Preposition == "" {
			t.Errorf("invalid gap candidate: %s", candidate.ID)
		}
	}
	for _, id := range []string{"beduerfen-ohne-praep-gen", "enthalten-ohne-praep-gen", "legen-auf-akk", "liegen-auf-dat", "stellen-in-akk", "stehen-in-dat"} {
		for _, candidate := range candidates {
			if candidate.ID == id {
				t.Errorf("gap candidates included excluded entry %s", id)
			}
		}
	}
	for _, typ := range []string{"case"} {
		all := rektionCandidates(entries, "", typ)
		for _, id := range []string{"legen-auf-akk", "liegen-auf-dat", "stellen-in-akk", "stehen-in-dat"} {
			found := false
			for _, candidate := range all {
				if candidate.ID == id {
					found = true
				}
			}
			if !found {
				t.Errorf("%s excluded from %s exercises", id, typ)
			}
		}
	}
	for _, id := range []string{"legen-auf-akk", "liegen-auf-dat", "stellen-in-akk", "stehen-in-dat"} {
		prompt := caseExercisePrompt(byID[id])
		if !strings.Contains(prompt, cleanExample(byID[id].Examples[0])) {
			t.Errorf("case prompt for %s lacks its completed sentence: %q", id, prompt)
		}
	}
}

func TestRektionJSONUTF8AndNoObviousCorruption(t *testing.T) {
	path := filepath.Join("..", "..", "backend", "data", "rektion.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(data) {
		t.Fatal("rektion.json is not valid UTF-8")
	}
	if len(data) >= 3 && string(data[:3]) == "\xef\xbb\xbf" {
		t.Fatal("rektion.json must not have a UTF-8 BOM")
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	var inspect func(any)
	inspect = func(item any) {
		switch v := item.(type) {
		case string:
			if strings.ContainsRune(v, '\ufffd') {
				t.Errorf("string contains Unicode replacement character: %q", v)
			}
			runes := []rune(v)
			for i := 1; i+1 < len(runes); i++ {
				if runes[i] == '?' && unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1]) {
					t.Errorf("string contains ? between letters: %q", v)
				}
			}
		case []any:
			for _, child := range v {
				inspect(child)
			}
		case map[string]any:
			for key, child := range v {
				inspect(key)
				inspect(child)
			}
		}
	}
	inspect(value)
}
