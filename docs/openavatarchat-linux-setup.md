# OpenAvatarChat setup on Linux

This repo intentionally does not track `external/`.

`external/OpenAvatarChat` contains third-party source, Python virtualenvs,
models, build artifacts, and logs. Rebuild it on the Linux machine instead of
copying the local macOS folder.

## Restore external/OpenAvatarChat

Run from the `audio-ai-agent` repo root:

```sh
mkdir -p external
git clone https://github.com/HumanAIGC-Engineering/OpenAvatarChat.git external/OpenAvatarChat
cd external/OpenAvatarChat
git checkout dcfba11
git submodule update --init --recursive src/handlers/avatar/liteavatar/algo/liteavatar
git apply ../../docs/patches/openavatarchat-local.patch
cp ../../docs/openavatarchat-configs/*.yaml config/
cd src/handlers/avatar/liteavatar/algo/liteavatar
git apply ../../../../../../../../docs/patches/liteavatar-local.patch
```

Then install the OpenAvatarChat dependencies on Linux from a clean environment:

```sh
cd external/OpenAvatarChat
python3 -m venv .venv
. .venv/bin/activate
python install.py
```

Set the keys on the Linux machine, not in Git:

```sh
export OPENAI_API_KEY="..."
```

Use the preserved config when starting OpenAvatarChat:

```sh
python src/demo.py --config config/chat_with_openai_compatible_edge_tts_mac.yaml
```
