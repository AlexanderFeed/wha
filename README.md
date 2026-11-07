# Artikel — German Article Quiz

A small React quiz with a Go API. It asks for the article (**der**, **die**, or **das**) of a random German noun and shows its German definition and source-provided English translation to distinguish meanings.

## Run the app

Requirements: Go 1.24.1 or newer, Node.js 20 or newer, and Python 3.10 or newer.

1. Build the word dataset once (the Kaikki dump is large and may take a while to download):

   ```powershell
   python -m pip install -r requirements.txt
   python tools/build_dataset.py
   ```

   This produces `data/words.jsonl`. Each line is one compact JSON record for a word sense, including its German definition and any English translation available in Kaikki. The importer uses Kaikki for noun gender and meanings, Goethe for A1–B1 labels, and Leipzig frequency to make provisional B2/C1 tiers for the remaining words.

2. To fill senses without a Kaikki English translation, install the optional offline translator and run it after building the dataset:

   ```powershell
   python -m pip install -r requirements-translation.txt
   python tools/fill_missing_translations.py
   ```

   The Argos German-to-English model downloads the first time, then translates locally. Generated translations are marked `"translationSource":"machine"`; they can be inaccurate, especially for compounds, and should be checked against Wiktionary.

3. In a terminal at the repository root, start the API:

   ```powershell
   go run ./cmd/server
   ```

   The API reads `data/words.jsonl` at startup and listens on port `8080`. Set `PORT`, `LISTEN_ADDR`, or `WORDS_JSONL` to customize its address or dataset path.

4. In another terminal, start the frontend:

   ```powershell
   cd web
   npm install
   npm run dev
   ```

5. Open the local address printed by Vite (usually `http://localhost:5173`).

The **Rektion** trainer is available at `http://localhost:5173/rektion`. It loads its 150 patterns from `backend/data/rektion.json` when the Go API starts. The dataset sets `levelUnverified: true` because CEFR placements are estimates; `needsReview` is reserved for grammatical uncertainties. For a public Docker/Caddy deployment, see [DEPLOYMENT.md](DEPLOYMENT.md).

## Dataset notes

- Goethe list entries retain their A1, A2, or B1 labels. Other nouns with a Leipzig rank in the top 50,000 are placed in provisional B2; rarer or unranked nouns go in provisional C1. B2/C1 are estimates, not official Goethe CEFR labels.
- `data/words.jsonl` is generated and can be rebuilt from the source files. The large dump and source PDFs are kept under `data/sources/` and ignored by Git.
- The importer keeps German nouns with a single Kaikki gender. Goethe-list matches use the source level; remaining nouns use the documented Leipzig frequency cutoff to estimate B2/C1.
- See [data/ATTRIBUTION.md](data/ATTRIBUTION.md) for source links and license notes.
