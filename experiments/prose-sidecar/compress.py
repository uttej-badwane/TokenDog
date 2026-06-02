#!/usr/bin/env python3
"""
Extractive prose compressor: run ANY extractive token-classification ONNX
model that emits a per-token keep score, with our own ~50-line inference —
tokenize, run the graph, threshold per-token, restitch the kept text.

Expected ONNX signature:
  inputs : input_ids [batch, seq] (int64), attention_mask [batch, seq] (int64)
  output : a [batch, seq] float tensor of per-token keep probabilities in [0,1]
           (high = keep). The decode is robust to the output name.

Provide the model's `model.onnx` + its `tokenizer.json`.

  pip install onnxruntime numpy tokenizers
  echo "your long prose ..." | python compress.py \
      --onnx model.onnx --tokenizer tokenizer.json --threshold 0.6
"""
import sys
import argparse
import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer


def compress(text, session, tok, threshold=0.5):
    enc = tok.encode(text)
    ids, offsets = enc.ids, enc.offsets  # offsets = (start, end) char span per token
    seq = np.array([ids], dtype=np.int64)

    # Feed by introspecting the graph's input names (input_ids + attention_mask).
    feeds = {}
    for inp in session.get_inputs():
        feeds[inp.name] = np.ones_like(seq) if "mask" in inp.name.lower() else seq

    outputs = session.run([o.name for o in session.get_outputs()], feeds)
    # Per-token keep PROBABILITY in [0,1], shape [batch, seq]. Keep a token
    # when its score clears the threshold — higher threshold = more aggressive.
    scores = next(np.asarray(o)[0] for o in outputs if np.asarray(o).ndim == 2)  # [seq]
    keep = scores >= threshold

    # Restitch: keep the original character spans of surviving real tokens
    # (special tokens have offset (0,0)); insert a space across dropped gaps.
    parts, last_end = [], None
    for i, (s, e) in enumerate(offsets):
        if not keep[i] or (s == 0 and e == 0):
            continue
        if last_end is not None and s > last_end:
            parts.append(" ")
        parts.append(text[s:e])
        last_end = e
    return "".join(parts).strip()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--onnx", required=True)
    ap.add_argument("--tokenizer", required=True)
    ap.add_argument("--threshold", type=float, default=0.5,
                    help="keep-probability cutoff in [0,1]; higher = more aggressive (default 0.5)")
    args = ap.parse_args()

    tok = Tokenizer.from_file(args.tokenizer)
    sess = ort.InferenceSession(args.onnx, providers=["CPUExecutionProvider"])
    text = sys.stdin.read()
    out = compress(text, sess, tok, args.threshold)

    nin, nout = len(text.split()), len(out.split())
    sys.stderr.write(f"[prose] {nin} -> {nout} words ({nout / max(1, nin) * 100:.0f}% kept)\n")
    print(out)


if __name__ == "__main__":
    main()
