#!/usr/bin/env python3
"""Extract report policy from local, pinned MediaInfoLib source checkouts.

Usage: generate-report-tables.py <v23.04 source root> <v26.10 source root>
The roots must contain Source/Resource/Text and License.html. No network is used.
"""

import hashlib
from pathlib import Path
import sys

VERSIONS = ("23.04", "26.10")
REVISIONS = ("dd3501560d777cd18d274254c6d3bd1c6d72b432", "153ad67a62172534b401e9d9a5752dc759fff77b")
KINDS = ("General", "Video", "Audio", "Text", "Image", "Other", "Menu")
DEST = Path(__file__).resolve().parents[1] / "internal/mediainfo/report_tables"


def generate(root, version, revision):
    manifest = [f"MediaInfoLib v{version}: {revision}"]

    def read(relative):
        data = (root / relative).read_bytes()
        manifest.append(f"{hashlib.sha256(data).hexdigest()}  {relative}")
        return [line.split(";") for line in data.decode("utf-8-sig").splitlines() if line]

    base = "Source/Resource/Text/"
    language = dict((row[0], row[1]) for row in read(base + "Language/DefaultLanguage.csv") if len(row) > 1)
    (DEST / f"{version}-values.tsv").write_text("".join(key + "\t" + value + "\n" for key, value in language.items() if key.startswith(("StoreMethod_", "Interlaced_", "MuxingMode_"))), encoding="utf-8", newline="\n")
    rows = []
    for kind in KINDS:
        for row in read(base + "Stream/" + kind + ".csv"):
            name = row[0]
            label_key = name.split("/String")[0]
            label = language.get(label_key, label_key)
            if label == label_key and "/" in label_key:
                parent, suffix = label_key.rsplit("/", 1)
                if suffix in ("Info", "Hint", "Url") and parent in language:
                    label = language[parent] + "/" + suffix
            rows.append("\t".join((kind, name, row[3], label)))
    (DEST / f"{version}-fields.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8", newline="\n")
    rows = []
    for row in read(base + "DataBase/Format.csv"):
        row += [""] * 8
        rows.append("\t".join((row[0], row[5])))
    (DEST / f"{version}-formats.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8", newline="\n")
    rows = []
    for namespace, kinds in (("Matroska", ("Audio", "Video", "Text")), ("Riff", ("Audio", "Video", "Text")), ("Mpeg4", ("General", "Audio", "Video", "Text", "Other"))):
        for kind in kinds:
            for row in read(base + f"DataBase/CodecID_{kind}_{namespace}.csv"):
                row += [""] * 4
                rows.append("\t".join((namespace, kind, row[0], row[1], row[2], row[3])))
    (DEST / f"{version}-codecs.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8", newline="\n")
    (DEST / f"{version}-sources.txt").write_text("\n".join(manifest) + "\n", encoding="utf-8", newline="\n")
    (DEST / f"{version}-License.html").write_text("\n".join(line.rstrip() for line in (root / "License.html").read_text(encoding="utf-8").splitlines()) + "\n", encoding="utf-8", newline="\n")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    DEST.mkdir(exist_ok=True)
    for root, version, revision in zip(map(Path, sys.argv[1:]), VERSIONS, REVISIONS):
        generate(root, version, revision)
