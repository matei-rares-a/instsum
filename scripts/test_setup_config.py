import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parent.parent
HELPER = ROOT / "scripts" / "setup-config.py"


class SetupConfigTests(unittest.TestCase):
    def test_defaults_custom_settings_and_no_overwrite(self):
        with tempfile.TemporaryDirectory(prefix="instsum config ") as directory:
            root = Path(directory)
            source = root / "source.yaml"
            target = root / "generated.yaml"
            runtime = root / "runtime with spaces"
            original = yaml.safe_load((ROOT / "configs" / "config.yaml").read_text(encoding="utf-8"))
            original["output"]["dir"] = "custom output"
            original["allow_cloud"] = False
            source.write_text(yaml.safe_dump(original), encoding="utf-8")
            command = [sys.executable, str(HELPER), str(source), str(target), str(runtime)]
            subprocess.run(command, check=True, capture_output=True)
            generated = yaml.safe_load(target.read_text(encoding="utf-8"))
            self.assertEqual(generated["output"]["dir"], "custom output")
            self.assertFalse(generated["allow_cloud"])
            self.assertEqual(generated["gpu"], original["gpu"])
            self.assertEqual(generated["ollama"], original["ollama"])
            self.assertEqual(generated["asr"]["model_path"], str(runtime / "models" / "whisper.bin"))
            self.assertEqual(generated["models"]["qwen2.5vl-7b"]["gguf_path"], str(runtime / "models" / "vision.gguf"))
            self.assertTrue(generated["llamacpp"]["manage_server"])
            self.assertEqual(yaml.safe_load(source.read_text(encoding="utf-8")), original)
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)
            self.assertEqual(yaml.safe_load(target.read_text(encoding="utf-8")), generated)

            target.unlink()
            original["asr"]["model_path"] = "my-model.bin"
            original["llamacpp"]["executable"] = "my-server"
            original["llamacpp"]["manage_server"] = False
            original["models"]["qwen2.5vl-7b"]["gguf_path"] = "my-vision.gguf"
            source.write_text(yaml.safe_dump(original), encoding="utf-8")
            subprocess.run(command, check=True, capture_output=True)
            generated = yaml.safe_load(target.read_text(encoding="utf-8"))
            self.assertEqual(generated["asr"]["model_path"], "my-model.bin")
            self.assertEqual(generated["models"]["qwen2.5vl-7b"]["gguf_path"], "my-vision.gguf")
            self.assertEqual(generated["llamacpp"]["executable"], "my-server")
            self.assertFalse(generated["llamacpp"]["manage_server"])


if __name__ == "__main__":
    unittest.main()
