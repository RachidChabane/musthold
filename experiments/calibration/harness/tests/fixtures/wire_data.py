"""Wire data recorded from real servers: Ollama 0.35.1 serving tev1:0.8b and a Kev server serving kev-0.8b.

The state is "the sky is green". The constants are the recorded bytes, unchanged.
"""

from __future__ import annotations

import json

RECORDED_OLLAMA_CHOICE_REQUEST = '{"model":"tev1:0.8b","state":"the sky is green","questions":{"color":{"type":"choice","instructions":"Which color does the text say the sky is?","criteria":{"blue":"blue","green":"green","grey":"grey"}}}}'

RECORDED_OLLAMA_CHOICE_REPLY = '{"model":"tev1:0.8b","answers":{"color":{"type":"choice","choice":"green","probabilities":{"blue":0.0028416608947914386,"green":0.9948621078449252,"grey":0.0022962312602833944},"confidence":0.9674686077174837}},"usage":{"input_tokens":125,"output_tokens":1}}'

RECORDED_OLLAMA_NOUL_REPLY = '{"model":"tev1:0.8b","answers":{"sky_is_green":{"type":"noul","noul":0.9861792840259738}},"usage":{"input_tokens":136,"output_tokens":1}}'

RECORDED_KEV_CHOICE_REPLY = '{"model":"kev-latest","answers":{"color":{"type":"choice","choice":"green","confidence":0.8802,"probabilities":{"blue":0.0422,"green":0.9202,"grey":0.0377}}},"usage":{"input_tokens":32,"output_tokens":63},"latency_ms":29.9}'

RECORDED_KEV_NOUL_REPLY = '{"model":"tev1:0.8b","answers":{"sky_is_green":{"type":"noul","noul":0.8516}},"usage":{"input_tokens":46,"output_tokens":24},"latency_ms":2103.3}'

RECORDED_KEV_RUN = "jaredpalmer/kev-0.8b@0123456789abcdef0123456789abcdef01234567"

RECORDED_KEV_MODELS_CARD = json.dumps(
    {
        "models": [
            {
                "name": "kev-latest",
                "description": "Kev pointer head on Qwen/Qwen3.5-0.8B, serving "
                + RECORDED_KEV_RUN
                + " at temperature 1.00",
                "release_date": "2026-05-01",
                "run": RECORDED_KEV_RUN,
                "base": "Qwen/Qwen3.5-0.8B",
                "lora": True,
                "device": "mps",
                "backend": "mlx",
                "dtype": "bfloat16",
                "temperature": 1.0,
                "max_state_tokens": 65536,
                "truncate_states": False,
                "cuda_graphs": None,
                "prefix_cache": {
                    "size": 8,
                    "min_state_tokens": 1024,
                    "max_tokens": 65536,
                    "hits": 0,
                    "misses": 0,
                    "cached_states": 0,
                    "oom_retries": 0,
                },
                "batches": {"count": 0, "requests": 0, "queued": 0},
            }
        ]
    }
)
