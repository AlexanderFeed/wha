package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed static/*
var embeddedFiles embed.FS

type Word struct {
	ID                int64    `json:"id"`
	Noun              string   `json:"noun"`
	Article           string   `json:"article"`
	Level             string   `json:"level"`
	Definition        string   `json:"definition"`
	Translations      []string `json:"translations"`
	TranslationSource string   `json:"translationSource"`
	SourceURL         string   `json:"sourceUrl"`
}

type Server struct {
	words       []Word
	byID        map[int64]Word
	rektion     []RektionEntry
	rektionByID map[string]RektionEntry
}

type RektionEntry struct {
	ID                   string   `json:"id"`
	Verb                 string   `json:"verb"`
	Preposition          string   `json:"preposition"`
	AcceptedPrepositions []string `json:"acceptedPrepositions,omitempty"`
	GapExercise          *bool    `json:"gapExercise,omitempty"`
	Case                 string   `json:"case"`
	Reflexive            bool     `json:"reflexive"`
	Level                string   `json:"level"`
	Translation          string   `json:"translation"`
	Examples             []string `json:"examples"`
	Notes                string   `json:"notes,omitempty"`
}

type RektionDataset struct {
	LevelUnverified bool            `json:"levelUnverified"`
	Entries         []RektionEntry  `json:"entries"`
	NeedsReview     []RektionReview `json:"needsReview"`
}

type RektionReview struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func main() {
	path := os.Getenv("WORDS_JSONL")
	if path == "" {
		path = "data/words.jsonl"
	}
	s, err := loadWords(path)
	if err != nil {
		log.Fatal(err)
	}
	rektionPath := os.Getenv("REKTION_JSON")
	if rektionPath == "" {
		rektionPath = "backend/data/rektion.json"
	}
	entries, err := loadRektion(rektionPath)
	if err != nil {
		log.Fatal(err)
	}
	s.rektion = entries
	s.rektionByID = make(map[string]RektionEntry, len(entries))
	for _, entry := range entries {
		s.rektionByID[entry.ID] = entry
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "words": len(s.words)})
	})
	mux.HandleFunc("GET /api/words/random", s.randomWord)
	mux.HandleFunc("POST /api/answer", s.checkAnswer)
	mux.HandleFunc("GET /api/rektion/exercise", s.rektionExercise)
	mux.HandleFunc("POST /api/rektion/answer", s.rektionAnswer)
	staticFiles, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", frontendHandler(staticFiles))
	addr := strings.TrimSpace(os.Getenv("LISTEN_ADDR"))
	if addr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		addr = ":" + port
	}
	log.Printf("Loaded %d words and %d Rektion patterns. API listening on http://%s", len(s.words), len(s.rektion), addr)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func frontendHandler(files fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" {
			if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFileFS(w, r, files, "index.html")
	})
}

func loadRektion(path string) ([]RektionEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var dataset RektionDataset
	if err := json.NewDecoder(file).Decode(&dataset); err != nil {
		return nil, errors.New(path + ": invalid JSON: " + err.Error())
	}
	if !dataset.LevelUnverified {
		return nil, errors.New(path + ": levelUnverified must be true until level labels are verified")
	}
	if err := validateRektionEntries(dataset.Entries); err != nil {
		return nil, errors.New(path + ": " + err.Error())
	}
	knownIDs := make(map[string]bool, len(dataset.Entries))
	for _, entry := range dataset.Entries {
		knownIDs[entry.ID] = true
	}
	reviewedIDs := make(map[string]bool, len(dataset.NeedsReview))
	for _, item := range dataset.NeedsReview {
		if !knownIDs[item.ID] || strings.TrimSpace(item.Reason) == "" || reviewedIDs[item.ID] {
			return nil, errors.New(path + ": needsReview must contain unique known ids and a reason")
		}
		reviewedIDs[item.ID] = true
	}
	return dataset.Entries, nil
}

var reflexivePronounPattern = regexp.MustCompile(`(?i)\b(mich|dich|sich|uns|euch)\b`)

func validateRektionEntries(entries []RektionEntry) error {
	if len(entries) == 0 {
		return errors.New("no Rektion entries")
	}
	ids := make(map[string]bool, len(entries))
	patterns := make(map[string]bool, len(entries))
	for i, entry := range entries {
		where := "entry " + strconv.Itoa(i) + " (" + entry.ID + ")"
		if entry.ID == "" || entry.Verb == "" || !validCase(entry.Case) || !validLevel(entry.Level) || strings.TrimSpace(entry.Translation) == "" {
			return errors.New("invalid required field in " + where)
		}
		if entry.ID != expectedRektionID(entry) {
			return errors.New("id does not match verb, preposition, and case in " + where + "; expected " + expectedRektionID(entry))
		}
		if entry.Preposition == "" && entry.Case != "genitiv" {
			return errors.New("empty preposition is reserved for direct genitive entries in " + where)
		}
		if ids[entry.ID] {
			return errors.New("duplicate id " + entry.ID)
		}
		ids[entry.ID] = true
		key := strings.ToLower(strings.TrimSpace(entry.Verb)) + "|" + strings.ToLower(strings.TrimSpace(entry.Preposition)) + "|" + entry.Case
		if patterns[key] {
			return errors.New("duplicate verb + preposition + case in " + where)
		}
		patterns[key] = true
		if entry.Preposition == "" && len(entry.AcceptedPrepositions) > 0 {
			return errors.New("accepted prepositions require a canonical preposition in " + where)
		}
		validGaps := append([]string{entry.Preposition}, entry.AcceptedPrepositions...)
		seenPrepositions := map[string]bool{entry.Preposition: true}
		for _, accepted := range entry.AcceptedPrepositions {
			if strings.TrimSpace(accepted) == "" || seenPrepositions[accepted] {
				return errors.New("invalid or duplicate accepted preposition in " + where)
			}
			seenPrepositions[accepted] = true
		}
		if entry.Preposition != "" && len(entry.Examples) == 0 {
			return errors.New("non-empty preposition has no example in " + where)
		}
		if len(entry.Examples) == 0 {
			return errors.New("missing example in " + where)
		}
		for _, example := range entry.Examples {
			if strings.Count(example, "{") != 1 || strings.Count(example, "}") != 1 {
				return errors.New("example must contain exactly one marked gap in " + where)
			}
			start, end := strings.Index(example, "{"), strings.Index(example, "}")
			if end <= start {
				return errors.New("malformed marked gap in " + where)
			}
			gap := example[start+1 : end]
			// An empty {} marker is intentional for direct genitive objects with no preposition.
			if !containsString(validGaps, gap) {
				return errors.New("example gap does not match a valid preposition in " + where)
			}
			if entry.Reflexive && !reflexivePronounPattern.MatchString(example) {
				return errors.New("reflexive entry example lacks a reflexive pronoun in " + where)
			}
		}
	}
	return nil
}

func expectedRektionID(entry RektionEntry) string {
	preposition := "ohne-praep"
	if entry.Preposition != "" {
		preposition = rektionSlug(entry.Preposition)
	}
	caseSuffix := map[string]string{"akkusativ": "akk", "dativ": "dat", "genitiv": "gen"}[entry.Case]
	return rektionSlug(entry.Verb) + "-" + preposition + "-" + caseSuffix
}

func rektionSlug(value string) string {
	value = strings.ToLower(value)
	value = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(value)
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

type RektionExercise struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Level  string `json:"level"`
	Prompt string `json:"prompt"`
}
type RektionAnswerRequest struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Answer string `json:"answer"`
}

func (s *Server) rektionExercise(w http.ResponseWriter, r *http.Request) {
	level := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("level")))
	if level != "" && !validLevel(level) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level must be A1, A2, B1, B2, or C1"})
		return
	}
	typ := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if !validRektionType(typ) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be preposition or case"})
		return
	}
	candidates := rektionCandidates(s.rektion, level, typ)
	if len(candidates) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no Rektion entries for this level"})
		return
	}
	entry := candidates[rand.IntN(len(candidates))]
	exercise := RektionExercise{ID: entry.ID, Type: typ, Level: entry.Level}
	switch typ {
	case "preposition":
		exercise.Prompt = gapPreposition(entry)
	case "case":
		exercise.Prompt = caseExercisePrompt(entry)
	}
	writeJSON(w, http.StatusOK, exercise)
}

func usesGapExercise(entry RektionEntry) bool {
	return entry.GapExercise == nil || *entry.GapExercise
}

func caseExercisePrompt(entry RektionEntry) string {
	prompt := entryCue(entry)
	if !usesGapExercise(entry) && len(entry.Examples) > 0 {
		prompt += ": " + cleanExample(entry.Examples[0])
	}
	return prompt
}

func rektionCandidates(entries []RektionEntry, level, typ string) []RektionEntry {
	var candidates []RektionEntry
	for _, entry := range entries {
		if (level == "" || entry.Level == level) && (typ != "preposition" || (entry.Preposition != "" && usesGapExercise(entry))) {
			candidates = append(candidates, entry)
		}
	}
	return candidates
}

func (s *Server) rektionAnswer(w http.ResponseWriter, r *http.Request) {
	var input RektionAnswerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	if input.ID == "" || !validRektionType(input.Type) || strings.TrimSpace(input.Answer) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id, valid type, and answer are required"})
		return
	}
	entry, ok := s.rektionByID[input.ID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Rektion entry not found"})
		return
	}
	correctAnswer := rektionCorrectAnswer(entry, input.Type)
	writeJSON(w, http.StatusOK, map[string]any{"correct": answerMatchesAny(input.Answer, rektionValidAnswers(entry, input.Type)), "correctAnswer": correctAnswer, "example": cleanExample(entry.Examples[0])})
}

func answerMatches(answer, correctAnswer string) bool {
	return strings.EqualFold(strings.TrimSpace(answer), correctAnswer)
}
func answerMatchesAny(answer string, accepted []string) bool {
	for _, value := range accepted {
		if answerMatches(answer, value) {
			return true
		}
	}
	return false
}
func rektionValidAnswers(entry RektionEntry, typ string) []string {
	switch typ {
	case "preposition":
		return append([]string{entry.Preposition}, entry.AcceptedPrepositions...)
	case "case":
		return []string{displayCase(entry.Case)}
	default:
		return nil
	}
}
func rektionCorrectAnswer(entry RektionEntry, typ string) string {
	return strings.Join(rektionValidAnswers(entry, typ), " / ")
}
func validCase(value string) bool {
	return value == "akkusativ" || value == "dativ" || value == "genitiv"
}
func validRektionType(value string) bool {
	return value == "preposition" || value == "case"
}
func displayCase(value string) string {
	switch value {
	case "akkusativ":
		return "Akkusativ"
	case "dativ":
		return "Dativ"
	default:
		return "Genitiv"
	}
}
func displayVerb(entry RektionEntry) string {
	if entry.Reflexive {
		return "sich " + entry.Verb
	}
	return entry.Verb
}
func entryCue(entry RektionEntry) string {
	cue := displayVerb(entry)
	if entry.Preposition != "" {
		cue += " " + entry.Preposition
	}
	return cue
}
func gapPreposition(entry RektionEntry) string {
	if len(entry.Examples) == 0 {
		return "Fill in the preposition for: " + entryCue(entry)
	}
	return gapMarkerPattern.ReplaceAllString(entry.Examples[0], "___")
}

var gapMarkerPattern = regexp.MustCompile(`\{([^{}]*)\}`)

func cleanExample(example string) string {
	return strings.Join(strings.Fields(gapMarkerPattern.ReplaceAllString(example, "$1")), " ")
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func loadWords(path string) (*Server, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := &Server{byID: make(map[int64]Word)}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		var word Word
		if err := json.Unmarshal(scanner.Bytes(), &word); err != nil {
			return nil, errors.New(path + ": invalid JSON on line " + strconv.Itoa(line) + ": " + err.Error())
		}
		if word.ID <= 0 || word.Noun == "" || word.Definition == "" || !validArticle(word.Article) || !validLevel(word.Level) {
			return nil, errors.New(path + ": invalid word record on line " + strconv.Itoa(line))
		}
		if _, exists := s.byID[word.ID]; exists {
			return nil, errors.New(path + ": duplicate word ID on line " + strconv.Itoa(line))
		}
		s.words = append(s.words, word)
		s.byID[word.ID] = word
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(s.words) == 0 {
		return nil, errors.New(path + ": dataset is empty; run the dataset importer first")
	}
	return s, nil
}

func (s *Server) randomWord(w http.ResponseWriter, r *http.Request) {
	level := strings.ToUpper(r.URL.Query().Get("level"))
	if !validLevel(level) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level must be A1, A2, B1, B2, or C1"})
		return
	}
	excluded := map[int64]bool{}
	for _, raw := range strings.Split(r.URL.Query().Get("exclude"), ",") {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			excluded[id] = true
		}
	}
	var words []Word
	for _, word := range s.words {
		if word.Level == level && !excluded[word.ID] {
			words = append(words, word)
		}
	}
	if len(words) == 0 && len(excluded) > 0 {
		for _, word := range s.words {
			if word.Level == level {
				words = append(words, word)
			}
		}
	}
	if len(words) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no words available for this level yet"})
		return
	}
	writeJSON(w, http.StatusOK, words[rand.IntN(len(words))])
}

func (s *Server) checkAnswer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID      int64  `json:"id"`
		Article string `json:"article"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if input.ID <= 0 || !validArticle(input.Article) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and a valid article are required"})
		return
	}
	word, ok := s.byID[input.ID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "word not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"correct": input.Article == word.Article, "article": word.Article})
}

func validArticle(article string) bool {
	return article == "der" || article == "die" || article == "das"
}
func validLevel(level string) bool {
	return level == "A1" || level == "A2" || level == "B1" || level == "B2" || level == "C1"
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
