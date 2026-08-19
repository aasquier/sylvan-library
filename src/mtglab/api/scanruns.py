"""The scan mode as a background job.

The fifth module of this shape, and the first whose duration **nobody has
measured**. That is stated rather than glossed, because the sentence this
project has been burned by three times is *"it is a few seconds"*: the theme
conversation turn carried it in a docstring and ran 4.3-133.8s, the dossier
was never re-measured after ADR 20 and presented deployed as a spinner and
then Safari's `Load failed`, and the theme proposal was 226s. A vision call
at `low` effort returning two short strings ought to be quick. "Ought to be"
is not a measurement, and the cost of being wrong here is a transport error
with no status code and no access-log line -- so it is a job from its first
commit, like `researchruns.py`, and for the same reason.

**Checking happens in the request; calling happens in the job.** Two things
refuse without touching the network -- a capture that is not an image this
reads, is empty, or is over the size cap (422, via `scan.ScanRefused`), and
no key or SDK (503). Carried into a worker they would both arrive as a job in
state `error`: one string for two cases and a status code for neither.

**Nothing is cached.** A photograph is not a question anybody asks twice --
the next capture is a different card, or the same card photographed better.
What *is* deduplicated is the double press: `Plan.key` is a digest of the
image bytes, so two clicks on one shot are one paid call, matched per owner
the way every other key here is (ADR 5's shape, one layer down).

**The job resolves against the pool before it returns**, and that is the
whole architecture in one line. What Claude produces is a `Sighting` -- the
same shape the browser's WebAssembly reader produces -- and it goes through
`identify` exactly as that one does. So a card read by Claude gets the same
scrutiny as a card read by Tesseract: a corner resolves only if its set code
is one of the pool's real 986, and a title only ever offers a shortlist. The
transcription rides back beside the reading so the page can show what was
actually seen, which is the difference between a wrong answer somebody can
spot and a wrong answer they cannot.

The `NET` lane, for the reason `themeruns` gives: this waits on a socket with
the GIL released, and queueing it behind a Tier 1 sweep would be minutes of
stall for nothing.
"""

from __future__ import annotations

import hashlib
from typing import Any

from mtglab.api.jobs import NET, Plan, Progress
from mtglab.api.service import ClaudeFailed

#: What `/api/jobs` calls one of these, namespaced like `claude.research`.
KIND = "claude.scan"


def plan_scan(*, image: Any, media_type: str = "image/jpeg",
              requested: Any = None, tier: str | None = None) -> Plan:
    """Refuse now if it is refusable; otherwise hand back the work.

    Raises `scan.ScanRefused` and `ClaudeUnavailable` to the caller so their
    422 and 503 survive, rather than flattening both into a job in state
    `error`.
    """
    from mtglab.api import service
    from mtglab.claude import client as claude_client
    from mtglab.claude import modes, scan
    from mtglab.claude.modes import ModeExhausted

    # Built here, before anything is queued: this is what validates the
    # capture, and every one of its refusals is a 422 the page acts on.
    ask = scan.message(image, media_type)
    stance = scan.stance_for(requested)

    digest = hashlib.sha256(
        ask["content"][0]["source"]["data"].encode("ascii")).hexdigest()

    # Raised here rather than a minute into a job that was never going to
    # work. `require` rather than `connect`, for the reason `themeruns` gives:
    # this only needs to know *whether* a call is possible, and connecting
    # would leave an HTTP client nobody closes behind on every request.
    claude_client.require()

    def run(progress: Progress) -> dict[str, Any]:
        try:
            turn = modes.converse(scan.MODE, messages=[ask], stance=stance,
                                  tier=tier, on_turn=progress)
        except ModeExhausted as exc:
            raise ClaudeFailed(str(exc)) from exc
        except claude_client.ClaudeUnavailable:
            # Only reachable if the key vanished between the check above and
            # the worker starting. Left as itself; it is already readable.
            raise
        except Exception as exc:
            # Broad on purpose and narrow in effect: everything landing here
            # is the SDK failing, and `explain` turns a 401 into "your key may
            # have expired" rather than a stack trace in a job's error field.
            raise ClaudeFailed(claude_client.explain(exc)) from exc

        seen = scan.sighting(turn)
        # Through the same door as the browser's reader. Nothing about this
        # reading is trusted more for having come from a better camera.
        read = service.identify_cards([seen] if seen else [])
        return {
            "reading": read["readings"][0] if read["readings"] else None,
            # What was actually transcribed, carried so the page can show it.
            # A wrong reading beside the words it came from is a mistake
            # somebody can see; a wrong reading alone is one they cannot.
            "transcribed": seen,
            "refused": turn.refused,
            "model": turn.model,
        }

    return Plan(KIND, "scan: a photographed card", None, run,
                lane=NET, key=f"scan:{digest}")
