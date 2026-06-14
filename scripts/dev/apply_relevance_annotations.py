#!/usr/bin/env python3
import argparse
import csv
import json
import sys
from pathlib import Path


DOMAINS = ("travel", "saas_opportunity", "app_opportunity")


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-path", required=True)
    parser.add_argument("--annotations-path", required=True)
    parser.add_argument("--output-path", required=True)
    return parser.parse_args()


def apply_annotations(args):
    input_path = Path(args.input_path)
    annotations_path = Path(args.annotations_path)
    output_path = Path(args.output_path)

    with annotations_path.open() as file:
        annotations = json.load(file)
    positives = {
        domain: set(annotations["positive_source_ids"].get(domain, []))
        for domain in DOMAINS
    }
    categories = {}
    for category, source_ids in annotations.get("false_positive_categories", {}).items():
        for source_id in source_ids:
            if source_id in categories:
                raise ValueError(f"source {source_id} has multiple false-positive categories")
            categories[source_id] = category

    with input_path.open(newline="") as file:
        reader = csv.DictReader(file)
        rows = list(reader)
        fieldnames = reader.fieldnames
    if not fieldnames:
        raise ValueError("evaluation fixture has no header")

    fixture_ids = [row["source_id"] for row in rows]
    if len(fixture_ids) != len(set(fixture_ids)):
        raise ValueError("evaluation fixture contains duplicate source IDs")
    fixture_id_set = set(fixture_ids)

    referenced = set(categories)
    for source_ids in positives.values():
        referenced.update(source_ids)
    unknown = sorted(referenced - fixture_id_set)
    if unknown:
        raise ValueError(f"annotations contain unknown source IDs: {unknown}")

    positive_union = set().union(*positives.values())
    categorized_positive = sorted(positive_union.intersection(categories))
    if categorized_positive:
        raise ValueError(
            "relevant candidates cannot have candidate-level false-positive categories: "
            f"{categorized_positive}"
        )

    note = (
        f"{annotations['reviewer']} reviewed under "
        f"{annotations['policy_version']} on {annotations['reviewed_at']}"
    )
    for row in rows:
        source_id = row["source_id"]
        if "text_excerpt" in row:
            row["text_excerpt"] = "\n".join(
                line.rstrip() for line in row["text_excerpt"].splitlines()
            ).strip()
        for domain in DOMAINS:
            row[f"label_{domain}"] = "1" if source_id in positives[domain] else "0"
        row["false_positive_category"] = categories.get(source_id, "")
        row["label_notes"] = note

    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_path = output_path.with_name(output_path.name + ".tmp")
    with temp_path.open("w", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=fieldnames, lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)
    temp_path.replace(output_path)

    return {
        "status": "completed",
        "rows_reviewed": len(rows),
        "positive_counts": {
            domain: len(source_ids) for domain, source_ids in positives.items()
        },
        "categorized_false_positives": len(categories),
        "output_path": str(output_path),
    }


def main():
    args = parse_args()
    try:
        print(json.dumps(apply_annotations(args)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
