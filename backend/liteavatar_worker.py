#!/usr/bin/env python3
import argparse
import io
import os
import subprocess
import sys
import tempfile
import threading
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import cv2


class Renderer:
    def __init__(self, open_avatar_chat_dir, data_dir, use_gpu=False, fps=30):
        self.open_avatar_chat_dir = os.path.abspath(open_avatar_chat_dir)
        self.data_dir = os.path.abspath(data_dir)
        self.fps = fps
        self.lock = threading.Lock()

        src_dir = os.path.join(self.open_avatar_chat_dir, "src")
        algo_dir = os.path.join(
            src_dir,
            "handlers",
            "avatar",
            "liteavatar",
            "algo",
            "liteavatar",
        )
        sys.path.insert(0, src_dir)
        sys.path.insert(0, algo_dir)
        os.chdir(algo_dir)

        from lite_avatar import liteAvatar

        self.avatar = liteAvatar(data_dir=self.data_dir, fps=self.fps, use_gpu=use_gpu)
        self.avatar.load_dynamic_model(self.data_dir)
        for _ in range(5):
            self.avatar.audio2param(bytes(16000 * 2))

    def render(self, wav_bytes):
        # ponytail: one model, serialize renders; add a worker pool only if concurrency matters.
        with self.lock:
            return self._render_locked(wav_bytes)

    def _render_locked(self, wav_bytes):
        pcm = wav_to_pcm_16k_mono(wav_bytes)
        params = self.avatar.audio2param(pcm, is_complete=True)

        with tempfile.TemporaryDirectory(prefix="audio-ai-agent-liteavatar-render-") as temp_dir:
            wav_path = os.path.join(temp_dir, "input.wav")
            frames_dir = os.path.join(temp_dir, "frames")
            out_path = os.path.join(temp_dir, "out.mp4")
            os.mkdir(frames_dir)

            with open(wav_path, "wb") as f:
                f.write(wav_bytes)

            for index, param in enumerate(params):
                bg_frame_id = bounce_frame_id(index, self.avatar.bg_video_frame_count)
                mouth_img = self.avatar.param2img(param, bg_frame_id)
                full_img, _ = self.avatar.merge_mouth_to_bg(mouth_img, bg_frame_id)
                cv2.imwrite(os.path.join(frames_dir, f"{index + 1:05d}.jpg"), full_img)

            subprocess.run(
                [
                    "ffmpeg",
                    "-hide_banner",
                    "-loglevel",
                    "error",
                    "-r",
                    str(self.fps),
                    "-i",
                    os.path.join(frames_dir, "%05d.jpg"),
                    "-i",
                    wav_path,
                    "-framerate",
                    str(self.fps),
                    "-c:v",
                    "libx264",
                    "-pix_fmt",
                    "yuv420p",
                    "-b:v",
                    "5000k",
                    "-shortest",
                    "-y",
                    out_path,
                ],
                check=True,
                stderr=subprocess.PIPE,
            )

            with open(out_path, "rb") as f:
                return f.read()


def wav_to_pcm_16k_mono(wav_bytes):
    with wave.open(io.BytesIO(wav_bytes), "rb") as wav_file:
        if wav_file.getnchannels() != 1:
            raise ValueError("expected mono WAV")
        if wav_file.getsampwidth() != 2:
            raise ValueError("expected 16-bit WAV")
        if wav_file.getframerate() != 16000:
            raise ValueError("expected 16kHz WAV")
        return wav_file.readframes(wav_file.getnframes())


def bounce_frame_id(index, frame_count):
    if int(index / frame_count) % 2 == 0:
        return index % frame_count
    return frame_count - 1 - index % frame_count


class Handler(BaseHTTPRequestHandler):
    renderer = None

    def do_GET(self):
        if self.path != "/healthz":
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}\n')

    def do_POST(self):
        if self.path != "/render":
            self.send_error(404)
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
            wav_bytes = self.rfile.read(length)
            video = self.renderer.render(wav_bytes)
        except Exception as exc:
            self.send_response(500)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(str(exc).encode("utf-8"))
            return

        self.send_response(200)
        self.send_header("Content-Type", "video/mp4")
        self.send_header("Content-Length", str(len(video)))
        self.end_headers()
        self.wfile.write(video)

    def log_message(self, fmt, *args):
        print(fmt % args, file=sys.stderr, flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--open-avatar-chat-dir", required=True)
    parser.add_argument("--data-dir", required=True)
    parser.add_argument("--use-gpu", action="store_true")
    parser.add_argument("--fps", type=int, default=30)
    args = parser.parse_args()

    Handler.renderer = Renderer(
        args.open_avatar_chat_dir,
        args.data_dir,
        use_gpu=args.use_gpu,
        fps=args.fps,
    )
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    host, port = server.server_address
    print(f"READY http://{host}:{port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
