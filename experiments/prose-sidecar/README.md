# prose-sidecar (optional)

A small localhost service that compresses **natural-language prose** with an
extractive ONNX model, for TokenDog's opt-in prose route. It is **off by
default** and entirely separate from the lean Go binary — you only run it if
you want ML prose compression on prose surfaces.

## When TokenDog uses it

The Go engine calls the sidecar **only inside the reversible pass**, and **only
for content that looks like prose** (never JSON / logs / code — see
`looksLikeProse` in `internal/core`). Because the reversible pass stashes the
original first, the compression is **lossy but recoverable**: the model's
output goes on the wire, and the full original is retrievable via the
`td_retrieve` MCP tool. So it never costs correctness, only an optional
retrieval — which the `td eval` harness measures (`recover-rate` must stay
100%).

## HTTP contract

```
POST /compress  {"text": "...", "threshold": 0.6}
            ->  {"compressed": "...", "ratio": 0.66}
```

## Run it

Bring any extractive **token-classification ONNX model** that emits a per-token
keep probability (signature: inputs `input_ids` + `attention_mask`, output a
`[batch, seq]` float in `[0,1]`, high = keep) plus its `tokenizer.json`.

```bash
pip install onnxruntime numpy tokenizers
python server.py --onnx model.onnx --tokenizer tokenizer.json --port 8071

# point TokenDog at it (opt-in):
export TD_REVERSIBLE=1
export TD_PROSE_ENDPOINT=http://127.0.0.1:8071/compress
export TD_PROSE_THRESHOLD=0.6        # ≤0.7 is the safe band (no facts dropped)
td gateway --upstream https://api.anthropic.com
```

`compress.py` is the inference (tokenize → run ONNX → threshold → restitch);
`server.py` wraps it as the HTTP sidecar.

## Notes

- **Prose only.** On structured tool output (JSON, logs, code) TokenDog's own
  lossless filters win — the engine's `looksLikeProse` gate keeps this sidecar
  away from that content.
- **Bound the latency.** It sits on the proxy path; the client times out
  (`TD_PROSE_TIMEOUT_MS`, default 2000) and falls back to the head/tail preview
  on any error, so the sidecar can never stall a request.
