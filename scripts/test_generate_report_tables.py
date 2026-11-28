"""Check source identity before report-table generation publishes provenance."""

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest


class ReportTableSourceIdentityTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name) / "source"
        self.root.mkdir()
        self.output = Path(temporary.name) / "output"
        self.output.mkdir()
        spec = importlib.util.spec_from_file_location(
            "report_table_generator", Path(__file__).with_name("generate-report-tables.py")
        )
        self.generator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.generator)
        self.generator.DEST = self.output
        resources = {"Language/DefaultLanguage.csv": "Format;Format\nLanguage_en-us;English (United States)\nLanguage_en;English\n", "DataBase/Format.csv": "Example;;;;;Description\n"}
        for kind in self.generator.KINDS:
            resources[f"Stream/{kind}.csv"] = "Format;;;Y YTY\n"
        for namespace, kinds in (("Matroska", ("Audio", "Video", "Text")), ("Riff", ("Audio", "Video", "Text")), ("Mpeg4", ("General", "Audio", "Video", "Text", "Other"))):
            for kind in kinds:
                resources[f"DataBase/CodecID_{kind}_{namespace}.csv"] = "1;Example;Hint;Description\n"
        for relative, content in resources.items():
            path = self.root / "Source/Resource/Text" / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        (self.root / "License.html").write_text("Fixture license\n", encoding="utf-8")
        (self.root / "Source/MediaInfo").mkdir()
        (self.root / "Source/MediaInfo/MediaInfo_Config_Automatic.cpp").write_text('void MediaInfo_Config_Iso639_1 (InfoMap &Info)\n{\n"eng;en\\n"\n"scc;sr\\n"\n}\n', encoding="utf-8")
        self.git("init", "--quiet")
        self.git("add", ".")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "--no-verify", "-m", "fixture")
        self.revision = self.git("rev-parse", "HEAD").strip()

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], text=True)

    def test_wrong_revision_writes_nothing(self):
        with self.assertRaises(SystemExit):
            self.generator.generate(self.root, "23.04", "0" * 40)
        self.assertEqual(list(self.output.iterdir()), [])

    def test_modified_source_writes_nothing(self):
        language = self.root / "Source/Resource/Text/Language/DefaultLanguage.csv"
        language.write_text("Format;Changed\n", encoding="utf-8")
        with self.assertRaises(SystemExit):
            self.generator.generate(self.root, "23.04", self.revision)
        self.assertEqual(list(self.output.iterdir()), [])

    def test_clean_revision_records_actual_identity(self):
        self.generator.generate(self.root, "23.04", self.revision)
        manifest = (self.output / "23.04-sources.txt").read_text(encoding="utf-8")
        self.assertEqual(manifest.splitlines()[0], "MediaInfoLib v23.04: " + self.revision)
        self.assertEqual((self.output / "23.04-languages.tsv").read_text(encoding="utf-8"), "en\tEnglish\nen-us\tEnglish (United States)\n")
        self.assertEqual((self.output / "23.04-language-aliases.tsv").read_text(encoding="utf-8"), "eng\ten\nscc\tsr\n")

    def test_modified_alias_source_writes_nothing(self):
        path = self.root / "Source/MediaInfo/MediaInfo_Config_Automatic.cpp"
        path.write_text("Changed\n", encoding="utf-8")
        with self.assertRaises(SystemExit):
            self.generator.generate(self.root, "23.04", self.revision)
        self.assertEqual(list(self.output.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
