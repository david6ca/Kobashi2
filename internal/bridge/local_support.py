"""Kobashi's managed model download. Never executes code from model repositories."""
import json
import os
import pathlib
import sys
import threading


def download(repo, revision, destination):
    from huggingface_hub import HfApi, hf_hub_download

    root = pathlib.Path(destination)
    root.mkdir(parents=True, exist_ok=True)
    info = HfApi().model_info(repo, revision=revision, files_metadata=True, token=False)
    extensions = {".json", ".safetensors", ".model", ".txt", ".tiktoken", ".jinja"}
    files = [f for f in info.siblings if pathlib.PurePosixPath(f.rfilename).suffix in extensions]
    if not any(f.rfilename.endswith(".safetensors") for f in files):
        raise ValueError("Model has no safetensors weights")
    total = sum(f.size or 0 for f in files)
    stop = threading.Event()

    def report():
        completed = sum((root / f.rfilename).stat().st_size for f in files if (root / f.rfilename).is_file())
        partial = sum(p.stat().st_size for p in root.rglob("*.incomplete") if p.is_file())
        print(json.dumps({"Done": min(completed + partial, total), "Total": total}), flush=True)

    def monitor():
        while not stop.wait(1):
            try:
                report()
            except OSError:
                pass

    thread = threading.Thread(target=monitor, daemon=True)
    thread.start()
    try:
        for file in files:
            hf_hub_download(repo, file.rfilename, revision=revision, local_dir=root, token=False)
        report()
    finally:
        stop.set()
        thread.join()


if __name__ == "__main__":
    if len(sys.argv) != 5 or sys.argv[1] != "download":
        raise SystemExit("Invalid managed download request")
    download(*sys.argv[2:])
