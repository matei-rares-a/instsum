import sys
from pathlib import Path

import yaml


def main():
    if len(sys.argv) != 4:
        raise ValueError("Expected source config, new output file and runtime directory")
    source, destination, runtime = map(Path, sys.argv[1:])
    with source.open(encoding="utf-8") as file:
        config = yaml.safe_load(file)
    if not isinstance(config, dict):
        raise ValueError("Configuration must be a YAML mapping")

    def replace_default(section, key, default, value):
        settings = config.setdefault(section, {})
        if settings.get(key, default) == default:
            settings[key] = str(value)

    replace_default("asr", "executable", "whisper-cli", runtime / "whisper" / "Release" / "whisper-cli.exe")
    replace_default("asr", "model_path", "./models/ggml-large-v3-turbo.bin", runtime / "models" / "whisper.bin")
    replace_default("ocr", "executable", "tesseract", runtime / "env" / "Library" / "bin" / "tesseract.exe")
    model = config.get("models", {}).get("qwen2.5vl-7b")
    if model is not None:
        if model.get("gguf_path") == "./models/Qwen2.5-VL-7B-Instruct-Q4_K_M.gguf":
            model["gguf_path"] = str(runtime / "models" / "vision.gguf")
        if model.get("mmproj_path") == "./models/mmproj-Qwen2.5-VL-7B-Instruct-f16.gguf":
            model["mmproj_path"] = str(runtime / "models" / "mmproj.gguf")
    llama = config.setdefault("llamacpp", {})
    if llama.get("executable", "llama-server") == "llama-server":
        llama["executable"] = str(runtime / "llama" / "llama-server.exe")
        llama["manage_server"] = True

    with destination.open("x", encoding="utf-8", newline="\n") as file:
        yaml.safe_dump(config, file, allow_unicode=True, sort_keys=False)


if __name__ == "__main__":
    main()
