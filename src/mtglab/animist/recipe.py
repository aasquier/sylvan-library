"""The recipe schema: what a `*.recipe.yaml` may say, loaded and validated.

A recipe lives beside the assets it produces and is the record of their
derivation — which source, confirmed under which licence, through which
transformations, to which checkable contract. `ops` and `expect` are separate
on purpose: ops are the derivation (and the executable pipeline for a new
asset), expect is what `mtglab animist verify` can hold the committed file to.
That separation is what lets a reconstruction recipe be honest about an
exploratory parameter — the ivy sprigs' crop boxes were chosen by eye against
a scored matte — while still pinning everything a machine can check.

Deliberately stdlib + yaml only. Validating a recipe, like reading the licence
gate's verdict, must work on an install without the `animist` extra; PIL
enters the picture in `ops.py` and later, inside functions.

`KNOWN_OPS` is this module's copy of the transform vocabulary. `ops.py` owns
the implementations, and `tests/test_animist_ops.py` pins the two sets equal —
the `SIMULATOR_KEYS` seam again, because a registry checked against a schema
in a test fails loudly, where a schema that trusts the registry would let an
unimplemented op into a committed recipe.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

#: The schema version this module reads. Bumped only when a recipe written
#: today would mean something different tomorrow — additive fields do not.
VERSION = 1

#: The still transform vocabulary. Implementations live in `ops.py`.
KNOWN_OPS = frozenset({"crop", "matte_green", "feather", "mirror_tile",
                       "resize"})

#: The motion vocabulary (ADR 31). Implementations live in `motion.py`;
#: these are the schema's copies, pinned equal in
#: `tests/test_animist_motion.py` — the same seam as `KNOWN_OPS`, because
#: this module is deliberately stdlib+yaml and may not import the registries.
#: A generator makes frames from parameters alone; a motion op transforms
#: frames; a seeded op is one whose derivation includes randomness and must
#: therefore be handed an explicit integer seed to stay a pure function.
KNOWN_GENERATOR_OPS = frozenset({"spectral_noise"})
KNOWN_MOTION_OPS = frozenset({"advect", "color_ramp", "ken_burns"})
KNOWN_SEEDED_OPS = frozenset({"spectral_noise", "advect"})

#: Where a source may come from. Each is a dispatch key in `sources.py`.
#: `procedural` is ADR 31's addition: a source with no upstream at all —
#: the declaration (its seed) *is* the source, the licence is necessarily
#: `ours-generated`, and the gate has nothing to ask anybody about.
PROVIDERS = frozenset({"openverse", "wikimedia", "wikimedia-category",
                       "procedural"})

#: Encoding targets, each a dispatch key in `encode.py`. `webp` is a still;
#: `awebp` and `apng` are the animated stills (Pillow, no new dependency);
#: `webm` (VP9) and `mp4` (H.264) are the video loops, encoded through
#: imageio-ffmpeg and always dual-shipped — one for each half of the
#: browser world, per the phase's Safari-16.4-floor decision.
FORMATS = frozenset({"webp", "awebp", "apng", "webm", "mp4"})

#: The formats whose output is a timeline rather than a picture.
ANIMATED_FORMATS = frozenset({"awebp", "apng", "webm", "mp4"})

#: The formats encoded by ffmpeg, whose rate control is `crf`, not `quality`.
VIDEO_FORMATS = frozenset({"webm", "mp4"})


class RecipeError(ValueError):
    """A recipe that does not say what a recipe must say.

    Always carries the recipe path and a sentence naming the field, so the
    refusal reads as an instruction rather than a stack trace.
    """


@dataclass(frozen=True)
class Source:
    """One upstream image (or category of them) and its expected licence.

    `licence` is the *expectation* written down at recipe time. The gate in
    `sources.py` does not string-match it — vocabularies differ per provider —
    it confirms, per file and through the API, that what the provider records
    is on the allowlist, and hands the confirmed value to `provenance.py`.
    The prose fields (`title`, `found_via`, `landing`) exist for the
    PROVENANCE entry — how a thing was found is part of where it came from.
    """

    id: str
    provider: str
    licence: str
    identifier: str = ""          # openverse: the record UUID
    title: str = ""               # wikimedia: the File:... title; prose elsewhere
    category: str = ""            # wikimedia-category: the category name
    require_filename_prefix: str = ""   # the RWS1909 guard; empty = no guard
    found_via: str = ""
    landing: str = ""
    seed: int | None = None       # procedural: the declaration IS the source


@dataclass(frozen=True)
class Op:
    """One transform: a name in `KNOWN_OPS` and its parameters."""

    name: str
    params: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class Encode:
    """How the output is written. `format` dispatches in `encode.py`.

    `quality` is the Pillow-family knob (webp/awebp; apng ignores it, being
    lossless); `crf` is the ffmpeg-family knob (webm/mp4). The loader
    requires exactly the knob the format reads and refuses the other, so a
    recipe cannot say something the encoder would silently ignore. There is
    deliberately no fps here — the rate belongs to the derivation (the
    generator or `ken_burns` op that created the timeline), and the encoder
    reads it off the `FrameSequence`.
    """

    format: str = "webp"
    quality: int = 82
    crf: int | None = None


@dataclass(frozen=True)
class Expect:
    """The checkable contract for a committed output.

    Everything optional except the intent: `verify` checks what is present and
    says nothing about what is not. `count` only makes sense on an `each:`
    output; `height` is optional because a fetched set (the tarot scans) may
    share a width and not a height.
    """

    width: int | None = None
    height: int | None = None
    count: int | None = None
    budget_kb: int | None = None
    total_budget_kb: int | None = None
    metadata_none: bool = True
    frames: int | None = None     # animated formats: exact frame count
    fps: int | None = None        # animated formats: exact rate


@dataclass(frozen=True)
class Output:
    """One committed file (`file`) or one file per fetched source (`each`)."""

    source: str
    ops: tuple[Op, ...]
    encode: Encode
    expect: Expect
    file: str = ""                # exactly one of `file` / `each` in the yaml
    each: bool = False


@dataclass(frozen=True)
class Recipe:
    """A loaded, validated recipe. Paths in it are relative to `directory`."""

    path: Path
    why_committed: str
    sources: dict[str, Source]
    outputs: tuple[Output, ...]

    @property
    def directory(self) -> Path:
        return self.path.parent

    @property
    def slug(self) -> str:
        """The cache key: `ambience.recipe.yaml` -> `ambience`."""
        name = self.path.name
        return name.removesuffix(".recipe.yaml") if \
            name.endswith(".recipe.yaml") else self.path.stem


def _fail(path: Path, message: str) -> RecipeError:
    return RecipeError(f"{path}: {message}")


def _require_str(path: Path, raw: dict[str, Any], key: str, where: str) -> str:
    value = raw.get(key)
    if not isinstance(value, str) or not value.strip():
        raise _fail(path, f"{where} needs a non-empty `{key}`")
    return value.strip()


def _load_source(path: Path, sid: str, raw: Any) -> Source:
    if not isinstance(raw, dict):
        raise _fail(path, f"source `{sid}` must be a mapping")
    provider = _require_str(path, raw, "provider", f"source `{sid}`")
    if provider not in PROVIDERS:
        raise _fail(path, f"source `{sid}` names unknown provider "
                          f"`{provider}` (one of: {', '.join(sorted(PROVIDERS))})")
    licence = _require_str(path, raw, "licence", f"source `{sid}`")
    if provider == "openverse":
        _require_str(path, raw, "identifier", f"openverse source `{sid}`")
    if provider == "wikimedia":
        _require_str(path, raw, "title", f"wikimedia source `{sid}`")
    if provider == "wikimedia-category":
        _require_str(path, raw, "category", f"wikimedia-category source `{sid}`")
    seed: int | None = None
    if provider == "procedural":
        # The declaration is the source (ADR 31): a seed instead of an
        # upstream, and the one licence a thing with no upstream can have.
        raw_seed = raw.get("seed")
        if not isinstance(raw_seed, int) or raw_seed < 0:
            raise _fail(path, f"procedural source `{sid}` needs `seed`, a "
                              "non-negative integer -- it is the whole of "
                              "what this source is")
        seed = raw_seed
        if licence != "ours-generated":
            raise _fail(path, f"procedural source `{sid}` must declare "
                              "`licence: ours-generated` -- generated pixels "
                              "have no upstream licence to confirm")
    return Source(
        id=sid, provider=provider, licence=licence,
        identifier=str(raw.get("identifier", "") or ""),
        title=str(raw.get("title", "") or ""),
        category=str(raw.get("category", "") or ""),
        require_filename_prefix=str(raw.get("require_filename_prefix", "") or ""),
        found_via=str(raw.get("found_via", "") or ""),
        landing=str(raw.get("landing", "") or ""),
        seed=seed,
    )


def _load_op(path: Path, index: int, raw: Any) -> Op:
    # An op is a single-key map: `- resize: {width: 400}`. The shape reads as
    # a step in a list, and the single key is what dispatches.
    if not isinstance(raw, dict) or len(raw) != 1:
        raise _fail(path, f"op #{index + 1} must be a single-key mapping "
                          "like `- resize: {width: 400}`")
    name, params = next(iter(raw.items()))
    vocabulary = KNOWN_OPS | KNOWN_MOTION_OPS | KNOWN_GENERATOR_OPS
    if name not in vocabulary:
        raise _fail(path, f"op #{index + 1} names unknown op `{name}` "
                          f"(one of: {', '.join(sorted(vocabulary))})")
    if params is None:
        params = {}
    if not isinstance(params, dict):
        raise _fail(path, f"op `{name}` parameters must be a mapping")
    return Op(name=str(name), params=dict(params))


def _load_encode(path: Path, raw: Any) -> Encode:
    if raw is None:
        return Encode()
    if not isinstance(raw, dict):
        raise _fail(path, "`encode` must be a mapping")
    fmt = str(raw.get("format", "webp"))
    if fmt not in FORMATS:
        raise _fail(path, f"unknown encode format `{fmt}` "
                          f"(one of: {', '.join(sorted(FORMATS))})")
    if fmt in VIDEO_FORMATS:
        # ffmpeg's knob, and only ffmpeg's knob: a `quality` here would be
        # silently ignored, which is worse than refused.
        if raw.get("quality") is not None:
            raise _fail(path, f"`{fmt}` is rate-controlled by `crf`, not "
                              "`quality` -- one knob per format, the one the "
                              "encoder actually reads")
        crf = raw.get("crf")
        ceiling = 63 if fmt == "webm" else 51
        if not isinstance(crf, int) or not 0 <= crf <= ceiling:
            raise _fail(path, f"`{fmt}` needs `encode.crf`, an integer from "
                              f"0 to {ceiling} (lower is larger and finer)")
        return Encode(format=fmt, quality=82, crf=crf)
    if raw.get("crf") is not None:
        raise _fail(path, f"`{fmt}` is quality-controlled, not crf -- `crf` "
                          "belongs to the video formats")
    quality = raw.get("quality", 82)
    if not isinstance(quality, int) or not 1 <= quality <= 100:
        raise _fail(path, "`encode.quality` must be an integer from 1 to 100")
    return Encode(format=fmt, quality=quality)


def _optional_int(path: Path, raw: dict[str, Any], key: str) -> int | None:
    value = raw.get(key)
    if value is None:
        return None
    if not isinstance(value, int) or value <= 0:
        raise _fail(path, f"`expect.{key}` must be a positive integer")
    return value


def _load_expect(path: Path, raw: Any) -> Expect:
    if not isinstance(raw, dict):
        raise _fail(path, "every output needs an `expect` block -- it is "
                          "what `mtglab animist verify` holds the committed "
                          "file to")
    metadata = raw.get("metadata", "none")
    if metadata != "none":
        raise _fail(path, "`expect.metadata` only takes `none`: a committed "
                          "asset never carries EXIF/XMP/ICC, because CI's "
                          "secret scan cannot read binaries")
    return Expect(
        width=_optional_int(path, raw, "width"),
        height=_optional_int(path, raw, "height"),
        count=_optional_int(path, raw, "count"),
        budget_kb=_optional_int(path, raw, "budget_kb"),
        total_budget_kb=_optional_int(path, raw, "total_budget_kb"),
        metadata_none=True,
        frames=_optional_int(path, raw, "frames"),
        fps=_optional_int(path, raw, "fps"),
    )


def _load_output(path: Path, index: int, raw: Any,
                 sources: dict[str, Source]) -> Output:
    where = f"output #{index + 1}"
    if not isinstance(raw, dict):
        raise _fail(path, f"{where} must be a mapping")
    file_name = str(raw.get("file", "") or "")
    each_source = str(raw.get("each", "") or "")
    if bool(file_name) == bool(each_source):
        raise _fail(path, f"{where} needs exactly one of `file` (one asset) "
                          "or `each` (one asset per fetched file)")
    source_id = _require_str(path, raw, "from", where) if file_name \
        else each_source
    if source_id not in sources:
        raise _fail(path, f"{where} draws from `{source_id}`, which is not "
                          "a declared source")
    ops_raw = raw.get("ops", [])
    if not isinstance(ops_raw, list):
        raise _fail(path, f"{where}: `ops` must be a list")
    ops = tuple(_load_op(path, i, op) for i, op in enumerate(ops_raw))
    expect = _load_expect(path, raw.get("expect"))
    if expect.count is not None and file_name:
        raise _fail(path, f"{where}: `expect.count` only makes sense on an "
                          "`each` output")
    source = sources[source_id]
    _check_motion_rules(path, where, source, ops, each=bool(each_source))
    return Output(source=source_id, ops=ops,
                  encode=_load_encode(path, raw.get("encode")),
                  expect=expect, file=file_name, each=bool(each_source))


def _check_motion_rules(path: Path, where: str, source: Source,
                        ops: tuple[Op, ...], *, each: bool) -> None:
    """ADR 31's derivation rules, refused at load rather than at build.

    A generator makes frames from nothing, so it only makes sense as the
    first op of a procedural output; a fetched image already *is* the frames,
    so a generator anywhere in its derivation would discard the source the
    licence gate just confirmed. And a seeded op with no seed to inherit is a
    derivation the recipe has not actually written down.
    """
    procedural = source.provider == "procedural"
    if procedural:
        if each:
            raise _fail(path, f"{where}: a procedural source yields one "
                              "derivation, so it needs `file:`, not `each:`")
        if not ops or ops[0].name not in KNOWN_GENERATOR_OPS:
            raise _fail(path, f"{where}: a procedural output must start with "
                              "a generator op (one of: "
                              f"{', '.join(sorted(KNOWN_GENERATOR_OPS))}) -- "
                              "there is no upstream image to start from")
    for index, op in enumerate(ops):
        if op.name in KNOWN_GENERATOR_OPS:
            if not procedural:
                raise _fail(path, f"{where}: `{op.name}` generates frames "
                                  "from parameters alone and belongs to a "
                                  "`procedural` source, not a fetched one")
            if index > 0:
                raise _fail(path, f"{where}: `{op.name}` must be the first "
                                  "op -- a generator replaces everything "
                                  "before it")
        if op.name in KNOWN_SEEDED_OPS:
            own = op.params.get("seed")
            if own is None and source.seed is None:
                raise _fail(path, f"{where}: `{op.name}` is stochastic and "
                                  "has no seed to inherit -- give it "
                                  "`seed:` in its params, or draw from a "
                                  "procedural source, whose seed it inherits")
            if own is not None and (not isinstance(own, int) or own < 0):
                raise _fail(path, f"{where}: `{op.name}` `seed` must be a "
                                  "non-negative integer")


def load_recipe(path: Path | str) -> Recipe:
    """Read and validate one `*.recipe.yaml`. Raises `RecipeError`, named."""
    path = Path(path)
    try:
        raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise _fail(path, f"cannot be read: {exc}") from exc
    except yaml.YAMLError as exc:
        raise _fail(path, f"is not valid YAML: {exc}") from exc
    if not isinstance(raw, dict):
        raise _fail(path, "must be a YAML mapping")
    version = raw.get("animist")
    if version != VERSION:
        raise _fail(path, f"declares `animist: {version!r}`; this tool reads "
                          f"version {VERSION}")
    # The one field that is prose, and the one refusal that is doctrine: the
    # sentence saying why this asset is committed rather than hotlinked flows
    # verbatim into PROVENANCE.md, and nothing here will ever write it for
    # you. ADR 8's spirit, applied to pictures.
    why = raw.get("why_committed")
    if not isinstance(why, str) or not why.strip():
        raise _fail(path, "needs `why_committed`: say, in your own words, why "
                          "this asset is committed rather than hotlinked -- "
                          "the pipeline will not invent the sentence")
    sources_raw = raw.get("sources")
    if not isinstance(sources_raw, dict) or not sources_raw:
        raise _fail(path, "needs at least one entry under `sources`")
    sources = {str(sid): _load_source(path, str(sid), source)
               for sid, source in sources_raw.items()}
    outputs_raw = raw.get("outputs")
    if not isinstance(outputs_raw, list) or not outputs_raw:
        raise _fail(path, "needs at least one entry under `outputs`")
    outputs = tuple(_load_output(path, i, out, sources)
                    for i, out in enumerate(outputs_raw))
    return Recipe(path=path, why_committed=why.strip(), sources=sources,
                  outputs=outputs)
