"""The depth seam: a Protocol the pipeline uses, a model only the dev Mac runs.

Torch never enters the container (ADR 32) — the deployed machine is a
1GB shared CPU and the image installs `.[api,claude]` — so everything above
this seam takes a `DepthModel` and every test hands in a fake. The real
loader lives behind the `depth` extra, imports torch inside the function,
and caches the model weights under the gitignored data tree, where the pool
and every other heavy download already live.

Determinism stance, recorded in ADR 32: CPU-only inference on pinned
weights. Bit-drift across torch builds is acceptable because nothing
byte-verifies a derivative — the cache key is inputs plus fingerprint, the
same contract-not-bytes stance `verify` takes for encoders.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING, Protocol

from animist import config

if TYPE_CHECKING:
    from PIL.Image import Image

#: Pinned, not floating: the fingerprint story assumes the same art in
#: yields the same depth out, and a silently upgraded checkpoint would break
#: that without a version moving anywhere.
MODEL_ID = "depth-anything/Depth-Anything-V2-Small-hf"


class DepthError(RuntimeError):
    """The model is unavailable or refused the image."""


class DepthModel(Protocol):
    """`infer` returns a grayscale depth map at the art's size — lighter is
    nearer. That polarity is a contract `parallax_frames` relies on."""

    def infer(self, image: Image) -> Image:
        ...


def load_model() -> DepthModel:  # pragma: no cover -- needs the depth extra
    """The real thing. Needs the `depth` extra; downloads ~100MB of weights
    into `data/cache/models/` on first use and reads them thereafter.

    Excluded from coverage deliberately, and the exclusion is the design:
    ADR 32 keeps torch out of dev and CI, so no test may import it -- the
    tested surface is the `DepthModel` Protocol and everything above it,
    and this loader is exercised by the real `cardmotion build` run
    on the maintainer's machine, whose output the deck pages then prove.
    """
    try:
        # HF_HOME must be set before transformers reads it at import time.
        cache = config.DATA_DIR / "cache" / "models"
        cache.mkdir(parents=True, exist_ok=True)
        os.environ.setdefault("HF_HOME", str(cache))
        import torch
        from transformers import pipeline
    except ImportError as exc:
        raise DepthError(
            "the depth extra is not installed -- `pip install -e '.[depth]'` "
            "brings CPU torch and the model loader; it is dev-machine "
            "tooling and never part of the app") from exc

    torch.set_num_threads(max(1, (os.cpu_count() or 2) - 1))
    estimator = pipeline("depth-estimation", model=MODEL_ID, device="cpu")

    class _Loaded:
        def infer(self, image: Image) -> Image:
            from PIL.Image import Image as PILImage

            result = estimator(image.convert("RGB"))
            depth = result["depth"]  # a PIL Image, per the pipeline contract
            if not isinstance(depth, PILImage):
                raise DepthError("the depth pipeline returned something "
                                 f"other than an image: {type(depth)!r}")
            return depth.convert("L").resize(image.size)

    return _Loaded()
