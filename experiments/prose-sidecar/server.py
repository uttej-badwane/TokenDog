#!/usr/bin/env python3
"""
Localhost prose-compression sidecar for TokenDog's internal/prose client
(TD_PROSE_ENDPOINT).

POST /compress  {"text": "...", "threshold": 0.6}  ->  {"compressed": "...", "ratio": 0.66}

Wraps compress.py (any extractive token-classification ONNX prose model). The
model loads once at startup. Compression is LOSSY — TokenDog only calls it
inside the reversible pass, where it stashes the original first, so the output
stays recoverable via td_retrieve.

  pip install onnxruntime numpy tokenizers
  python server.py --onnx model.onnx --tokenizer tokenizer.json --port 8071
  export TD_PROSE_ENDPOINT=http://127.0.0.1:8071/compress   # then run td proxy / td gateway
"""
import os
import sys
import json
import argparse
from http.server import BaseHTTPRequestHandler, HTTPServer

import onnxruntime as ort
from tokenizers import Tokenizer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from compress import compress  # noqa: E402


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path.rstrip("/") != "/compress":
            self.send_error(404)
            return
        try:
            n = int(self.headers.get("content-length", 0))
            req = json.loads(self.rfile.read(n) or b"{}")
            text = req.get("text", "")
            threshold = float(req.get("threshold", 0.6))
            out = compress(text, self.server.sess, self.server.tok, threshold) if text else ""
            payload = json.dumps({
                "compressed": out,
                "ratio": len(out) / max(1, len(text)),
            }).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        except Exception as e:  # never take the proxy down; report and move on
            self.send_error(500, str(e))

    def log_message(self, *args):  # quiet
        pass


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--onnx", required=True)
    ap.add_argument("--tokenizer", required=True)
    ap.add_argument("--port", type=int, default=8071)
    a = ap.parse_args()

    srv = HTTPServer(("127.0.0.1", a.port), Handler)
    srv.tok = Tokenizer.from_file(a.tokenizer)
    srv.sess = ort.InferenceSession(a.onnx, providers=["CPUExecutionProvider"])
    print(f"prose sidecar on http://127.0.0.1:{a.port}/compress  (Ctrl-C to stop)")
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
