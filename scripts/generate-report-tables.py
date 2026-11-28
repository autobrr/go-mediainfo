#!/usr/bin/env python3
"""Extract report policy from local, pinned MediaInfoLib source checkouts.

Usage: generate-report-tables.py <v23.04 source root> <v26.10 source root>
The roots must be Git checkouts at the pinned revisions with unchanged
Source/Resource/Text and License.html inputs. No network is used.
Codec-ID tables are extracted only for ordinary v23.04 container namespaces;
v26.10 supplies DVD field, format, and language policy.
"""

import hashlib
import re
from pathlib import Path
import subprocess
import sys

VERSIONS = ("23.04", "26.10")
REVISIONS = ("dd3501560d777cd18d274254c6d3bd1c6d72b432", "153ad67a62172534b401e9d9a5752dc759fff77b")
KINDS = ("General", "Video", "Audio", "Text", "Image", "Other", "Menu")
DEST = Path(__file__).resolve().parents[1] / "internal/mediainfo/report_tables"


def generate(root, version, revision):
    checkout, actual = subprocess.check_output(
        ["git", "-C", str(root), "rev-parse", "--show-toplevel", "HEAD"], text=True
    ).splitlines()
    if Path(checkout).resolve() != root.resolve() or actual != revision:
        raise SystemExit(f"MediaInfoLib v{version} requires checkout revision {revision}")
    state = subprocess.run(
        ["git", "-C", str(root), "diff", "--quiet", revision, "--", "Source/Resource/Text", "Source/MediaInfo/MediaInfo_Config_Automatic.cpp", "License.html"]
    )
    if state.returncode == 1:
        raise SystemExit(f"MediaInfoLib v{version} extraction inputs differ from {revision}")
    state.check_returncode()
    manifest = [f"MediaInfoLib v{version}: {revision}"]

    def source_bytes(relative):
        data = (root / relative).read_bytes()
        manifest.append(f"{hashlib.sha256(data).hexdigest()}  {relative}")
        return data

    def read(relative):
        data = source_bytes(relative)
        return [line.split(";") for line in data.decode("utf-8-sig").splitlines() if line]

    base = "Source/Resource/Text/"
    language = dict((row[0], row[1]) for row in read(base + "Language/DefaultLanguage.csv") if len(row) > 1)
    names = sorted((key.removeprefix("Language_"), value) for key, value in language.items() if key.startswith("Language_"))
    (DEST / f"{version}-languages.tsv").write_text("".join(code + "\t" + name + "\n" for code, name in names), encoding="utf-8", newline="\n")
    automatic = source_bytes("Source/MediaInfo/MediaInfo_Config_Automatic.cpp").decode("utf-8-sig")
    aliases = automatic.split("void MediaInfo_Config_Iso639_1 (", 1)[1].split("\n}", 1)[0]
    aliases = sorted(dict(re.findall(r'"([^";\\]+);([^"\\]+)\\n"', aliases)).items())
    (DEST / f"{version}-language-aliases.tsv").write_text("".join(code + "\t" + primary + "\n" for code, primary in aliases), encoding="utf-8", newline="\n")
    (DEST / f"{version}-values.tsv").write_text("".join(key + "\t" + value + "\n" for key, value in language.items() if key.startswith(("StoreMethod_", "Interlaced_", "MuxingMode_"))), encoding="utf-8", newline="\n")
    rows = []
    for kind in KINDS:
        for row in read(base + "Stream/" + kind + ".csv"):
            name = row[0]
            label_key = name.split("/String")[0]
            label = language.get(label_key, label_key)
            if "/" in label_key:
                # Language_Set translates each side of the first schema slash.
                parent, suffix = label_key.split("/", 1)
                label = language.get(parent, parent) + "/" + language.get(suffix, suffix)
            rows.append("\t".join((kind, name, row[3], label)))
    (DEST / f"{version}-fields.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8", newline="\n")
    rows = []
    for row in read(base + "DataBase/Format.csv"):
        row += [""] * 8
        rows.append("\t".join((row[0], row[5])))
    (DEST / f"{version}-formats.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8", newline="\n")
    # DVD has no container codec namespace; only ordinary reports use these.
    if version == "23.04":
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
