# OpenAvatarChat setup on Linux

This repo intentionally does not track `external/`.

`external/OpenAvatarChat` contains third-party source, Python virtualenvs,
models, build artifacts, and logs. Rebuild it on the Linux machine instead of
copying the local macOS folder.

Project-owned OpenAvatarChat overrides live in `openavatarchat-overrides/` and
are copied into the clean checkout during setup.

## Restore external/OpenAvatarChat

Run from the `audio-ai-agent` repo root:

```sh
mkdir -p external
git clone https://github.com/HumanAIGC-Engineering/OpenAvatarChat.git external/OpenAvatarChat
cd external/OpenAvatarChat
git checkout dcfba11
git submodule update --init --recursive src/handlers/avatar/flashhead/SoulX-FlashHead
git apply ../../docs/patches/openavatarchat-local.patch
cp -R ../../openavatarchat-overrides/. .
cp ../../docs/openavatarchat-configs/*.yaml config/
```

Then install the OpenAvatarChat dependencies on Linux from a clean environment:

```sh
cd external/OpenAvatarChat
python3 -m venv .venv
. .venv/bin/activate
python install.py --config config/local_openai_flashhead.yaml
python scripts/download_models.py --handler flashhead
```

Create the OpenAvatarChat `.env` on the Linux machine, not in Git:

```sh
ln -sf ../../.env .env
```

Use the preserved config when starting OpenAvatarChat:

```sh
python src/demo.py --config config/local_openai_flashhead.yaml
```
