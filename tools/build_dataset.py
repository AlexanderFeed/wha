#!/usr/bin/env python3
"""Build the quiz JSONL from Kaikki, Goethe lists, and Leipzig frequency data."""

from __future__ import annotations

import gzip
import json
import re
import tarfile
import unicodedata
from pathlib import Path
from urllib.request import urlretrieve

from pypdf import PdfReader

ROOT = Path(__file__).resolve().parents[1]
SOURCES = ROOT / "data" / "sources"
OUTPUT = ROOT / "data" / "words.jsonl"
KAIKKI_URL = "https://kaikki.org/dewiktionary/raw-wiktextract-data.jsonl.gz"
LEIPZIG_URL = "https://downloads.wortschatz-leipzig.de/corpora/deu_mixed-typical_2011_1M.tar.gz"
GOETHE = {
    "A1": ("goethe-a1.pdf", "https://www.goethe.de/pro/relaunch/prf/de/A1_SD1_Wortliste_02.pdf"),
    "A2": ("goethe-a2.pdf", "https://www.goethe.de/pro/relaunch/prf/en/Goethe-Zertifikat_A2_Wortliste.pdf"),
    "B1": ("goethe-b1.pdf", "https://www.goethe.de/pro/relaunch/prf/de/Goethe-Zertifikat_B1_Wortliste.pdf"),
}
ARTICLES = {"m": "der", "f": "die", "n": "das", "masculine": "der", "feminine": "die", "neuter": "das"}


def fetch(path: Path, url: str) -> None:
    if not path.exists():
        print(f"Downloading {url}", flush=True)
        urlretrieve(url, path)


def norm(value: str) -> str:
    return unicodedata.normalize("NFC", value).strip().casefold()


def extract_goethe(level: str, pdf: Path) -> dict[str, str]:
    text = "\n".join(page.extract_text() or "" for page in PdfReader(pdf).pages)
    if level == "A1":
        marker = "VS_02_280312 Seite 9"
        pos = text.find(marker)
        if pos >= 0:
            text = text[pos + len(marker):]
    else:
        positions = [m.start() for m in re.finditer("WORTLISTE", text)]
        if len(positions) > 1:
            text = text[positions[1]:]

    found: dict[str, str] = {}
    # Use Unicode letter classes rather than spelling out umlauts; PDFs often
    # place two vocabulary columns on one line.
    pattern = re.compile(r"(?:^|\n|\s{3,})(der|die|das)\s+([^\W\d_][\w'-]*)", re.UNICODE)
    for match in pattern.finditer(text):
        article, noun = match.groups()
        if not noun[0].isupper():
            continue
        noun = noun.rstrip("-'")
        if len(noun) < 2:
            continue
        key = norm(noun)
        previous = found.get(key)
        if previous is None:
            found[key] = article
        elif previous != article:
            found[key] = "AMBIGUOUS"
    return {word: article for word, article in found.items() if article != "AMBIGUOUS"}


def leipzig_ranks(archive: Path) -> dict[str, int]:
    """Read word frequencies and return most-common-first normalized ranks."""
    totals: dict[str, int] = {}
    with tarfile.open(archive, "r:gz") as bundle:
        member = next((item for item in bundle.getmembers() if item.name.endswith("-words.txt")), None)
        if member is None:
            raise RuntimeError("Leipzig archive does not contain a -words.txt file")
        stream = bundle.extractfile(member)
        if stream is None:
            raise RuntimeError("Could not read the Leipzig word-list file")
        for raw in stream:
            columns = raw.decode("utf-8").rstrip("\r\n").split("\t")
            if len(columns) < 3:
                continue
            try:
                count = int(columns[2])
            except ValueError:
                continue
            key = norm(columns[1])
            totals[key] = totals.get(key, 0) + count
    ordered = sorted(totals.items(), key=lambda row: (-row[1], row[0]))
    return {word: rank for rank, (word, _) in enumerate(ordered, 1)}


def gender_from_entry(entry: dict) -> str | None:
    values: set[str] = set()
    for tag in entry.get("tags", []) or []:
        if str(tag).casefold() in ARTICLES:
            values.add(ARTICLES[str(tag).casefold()])
    for template in entry.get("head_templates", []) or []:
        for key, value in (template.get("args", {}) or {}).items():
            label = str(key).casefold()
            if "genus" in label or label in {"1", "g", "gender"}:
                for part in re.split(r"[,/ ]+", str(value).strip().casefold()):
                    if part in ARTICLES:
                        values.add(ARTICLES[part])
    if len(values) == 1:
        return values.pop()
    return None


def english_translations(entry: dict, sense: dict) -> list[str]:
    """Find English translations linked to this sense by sense index or gloss."""
    senses = entry.get("senses", []) or []
    sense_index = str(sense.get("sense_index", ""))
    glosses = {norm(gloss) for gloss in sense.get("glosses", []) if isinstance(gloss, str)}
    matched: list[str] = []
    for translation in entry.get("translations", []) or []:
        if translation.get("lang_code") != "en":
            continue
        translation_index = str(translation.get("sense_index", ""))
        translation_sense = norm(str(translation.get("sense", "")))
        linked = bool(sense_index and translation_index == sense_index)
        linked = linked or bool(translation_sense and translation_sense in glosses)
        if not linked and len(senses) == 1 and not translation_index and not translation_sense:
            linked = True
        word = str(translation.get("word", "")).strip()
        if linked and word and word not in matched:
            matched.append(word)
    return matched[:6]


def main() -> None:
    SOURCES.mkdir(parents=True, exist_ok=True)
    dump = SOURCES / "kaikki-dewiktionary.jsonl.gz"
    leipzig = SOURCES / "leipzig-deu-mixed-typical-2011-1M.tar.gz"
    fetch(dump, KAIKKI_URL)
    fetch(leipzig, LEIPZIG_URL)
    for filename, url in GOETHE.values():
        fetch(SOURCES / filename, url)

    goethe = {level: extract_goethe(level, SOURCES / filename) for level, (filename, _) in GOETHE.items()}
    goethe_level: dict[str, str] = {}
    for level in ("A1", "A2", "B1"):
        for noun, article in goethe[level].items():
            if noun not in goethe_level:
                goethe_level[noun] = level
            elif goethe_level[noun] != "AMBIGUOUS" and goethe[goethe_level[noun]][noun] != article:
                goethe_level[noun] = "AMBIGUOUS"

    nouns: dict[str, dict] = {}
    senses_to_export: dict[tuple[str, str, str], dict] = {}
    scanned = 0
    with gzip.open(dump, "rt", encoding="utf-8") as source:
        for line in source:
            scanned += 1
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue
            if entry.get("lang_code") != "de" or entry.get("pos") != "noun":
                continue
            noun = str(entry.get("word", "")).strip()
            if not noun:
                continue
            article = gender_from_entry(entry)
            if article is None:
                continue
            key = norm(noun)
            record = nouns.setdefault(key, {"noun": noun, "articles": set()})
            record["articles"].add(article)
            for sense in entry.get("senses", []) or []:
                glosses = sense.get("glosses", []) or []
                definition = next((str(gloss).strip() for gloss in glosses if str(gloss).strip()), "")
                if not definition:
                    continue
                translations = english_translations(entry, sense)
                sense_key = (key, article, definition)
                exported = senses_to_export.setdefault(sense_key, {"noun": noun, "translations": []})
                for translation in translations:
                    if translation not in exported["translations"]:
                        exported["translations"].append(translation)

    # Keep sense-level meanings from Kaikki, including homographs with distinct
    # genders. Goethe labels are used
    # where available; a frequency cutoff assigns the remainder to provisional
    # B2/C1 tiers. These are estimates, not official CEFR assignments.
    ranks = leipzig_ranks(leipzig)
    # The Leipzig list is frequency-ranked. Reserve B2 for headwords found in
    # its first 50,000 entries; rare and unranked terms go to estimated C1.
    estimated_level = {key: ("B2" if ranks.get(key, 10**12) <= 50_000 else "C1") for key in nouns}

    rows = []
    ordered_senses = sorted(senses_to_export.items(), key=lambda item: item[0])
    for (key, article, definition), exported in ordered_senses:
        noun = exported["noun"]
        translations = exported["translations"]
        level = goethe_level.get(key)
        if level is None or level == "AMBIGUOUS" or goethe[level].get(key) != article:
            level = estimated_level[key]
        rows.append({"id": len(rows) + 1, "noun": noun, "article": article, "level": level,
                     "definition": definition, "translations": list(translations),
                     "translationSource": "wiktionary" if translations else "missing",
                     "sourceUrl": "https://de.wiktionary.org/wiki/" + noun.replace(" ", "_")})

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with OUTPUT.open("w", encoding="utf-8", newline="\n") as destination:
        for row in rows:
            destination.write(json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n")

    counts = {level: sum(row["level"] == level for row in rows) for level in ("A1", "A2", "B1", "B2", "C1")}
    missing_english = sum(not row["translations"] for row in rows)
    print(f"Scanned {scanned:,} Wiktionary records; wrote {len(rows):,} word senses to {OUTPUT.relative_to(ROOT)}")
    print(f"Word senses without an English translation in Kaikki: {missing_english:,}")
    print("Levels: " + ", ".join(f"{level}={count}" for level, count in counts.items()))
    print("B2/C1 labels are provisional tiers: Leipzig ranks 1-50,000 => B2; rarer or unranked => C1.")
    if not rows:
        raise SystemExit("No matching words found. Check the source formats and dump fields before running the app.")


if __name__ == "__main__":
    main()
