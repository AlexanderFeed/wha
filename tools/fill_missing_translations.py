#!/usr/bin/env python3
"""Fill untranslated German word senses with offline Argos German-to-English MT."""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DATASET = ROOT / "data" / "words.jsonl"
BATCH_SIZE = 48


def get_translator():
    try:
        import argostranslate.package
        import argostranslate.translate
    except ImportError as exc:
        raise SystemExit(
            "Install the optional offline translation dependency first: "
            "python -m pip install -r requirements-translation.txt"
        ) from exc

    languages = argostranslate.translate.get_installed_languages()
    german = next((language for language in languages if language.code == "de"), None)
    english = next((language for language in languages if language.code == "en"), None)
    if not german or not english:
        print("Downloading the Argos German-to-English model (one-time setup)...", flush=True)
        argostranslate.package.update_package_index()
        package = next(
            (item for item in argostranslate.package.get_available_packages()
             if item.from_code == "de" and item.to_code == "en"),
            None,
        )
        if package is None:
            raise SystemExit("The Argos package index has no German-to-English model.")
        argostranslate.package.install_from_path(package.download())
        languages = argostranslate.translate.get_installed_languages()
        german = next(language for language in languages if language.code == "de")
        english = next(language for language in languages if language.code == "en")

    # Argos wraps the local CTranslate2 model in CachedTranslation. Use its
    # underlying translator so many short glosses can be translated per batch.
    translation = german.get_translation(english)
    if hasattr(translation, "underlying"):
        translation = translation.underlying
    translation.translate("Ready.")  # Load the model once.
    return translation


def translate_batch(translation, definitions: list[str]) -> list[str]:
    package = translation.pkg
    tokenized = [package.tokenizer.encode(definition) for definition in definitions]
    translated = translation.translator.translate_batch(
        tokenized,
        replace_unknowns=True,
        max_batch_size=BATCH_SIZE,
        batch_type="tokens",
        beam_size=1,
        num_hypotheses=1,
    )
    return [package.tokenizer.decode(result.hypotheses[0]).strip() for result in translated]


def main() -> None:
    if not DATASET.exists():
        raise SystemExit(f"Dataset not found: {DATASET}. Run tools/build_dataset.py first.")

    rows = [json.loads(line) for line in DATASET.open("r", encoding="utf-8")]
    missing_definitions = list(dict.fromkeys(
        str(row.get("definition", "")).strip()
        for row in rows if not row.get("translations") and row.get("definition")
    ))
    print(
        f"Loaded {len(rows):,} senses; translating {len(missing_definitions):,} "
        "unique missing definitions offline.", flush=True
    )
    translation = get_translator()
    translated_definitions: dict[str, str] = {}

    for start in range(0, len(missing_definitions), BATCH_SIZE):
        batch = missing_definitions[start:start + BATCH_SIZE]
        for source, english in zip(batch, translate_batch(translation, batch)):
            translated_definitions[source] = english
        if (start + len(batch)) % 480 < BATCH_SIZE or start + len(batch) == len(missing_definitions):
            done = start + len(batch)
            print(f"Translated {done:,}/{len(missing_definitions):,} unique definitions.", flush=True)

    generated = 0
    still_missing = 0
    for row in rows:
        if row.get("translations"):
            row["translationSource"] = row.get("translationSource") or "wiktionary"
            continue
        english = translated_definitions.get(str(row.get("definition", "")).strip(), "")
        if english:
            row["translations"] = [english]
            row["translationSource"] = "machine"
            generated += 1
        else:
            row["translationSource"] = "missing"
            still_missing += 1

    with tempfile.NamedTemporaryFile(
        "w", encoding="utf-8", newline="\n", dir=DATASET.parent,
        suffix=".jsonl.tmp", delete=False,
    ) as output:
        temporary = Path(output.name)
        try:
            for row in rows:
                output.write(json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n")
            output.flush()
            os.fsync(output.fileno())
            output.close()
            temporary.replace(DATASET)
        except BaseException:
            temporary.unlink(missing_ok=True)
            raise

    print(f"Generated {generated:,} machine translations.")
    if still_missing:
        print(f"{still_missing:,} senses could not be translated and remain marked missing.")


if __name__ == "__main__":
    main()
