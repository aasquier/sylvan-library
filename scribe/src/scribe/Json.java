// Copyright (C) 2026 sylvan-library contributors
//
// This file is part of the Forge scribe, which links against Forge and is
// therefore licensed under the GNU General Public License version 3.
// See LICENSE in this directory. The rest of the sylvan-library repository is
// MIT; the boundary between them is a process boundary, never a linkage.
package scribe;

/**
 * The smallest JSON writer that can be trusted, hand-rolled on purpose.
 *
 * Forge's fat jar bundles several JSON libraries, and depending on whichever
 * one happens to be in there is a dependency nobody declared and a breakage
 * the next Forge release gets to choose the timing of. Forty lines of escaping
 * is cheaper than that, and this is the whole of what the scribe emits: flat
 * objects of strings, ints and booleans.
 */
final class Json {
    private final StringBuilder out = new StringBuilder(256);
    private boolean first = true;

    Json(String kind) {
        out.append('{');
        put("t", kind);
    }

    Json put(String key, String value) {
        if (value == null) return this;
        comma();
        quote(key);
        out.append(':');
        quote(value);
        return this;
    }

    Json put(String key, int value) {
        comma();
        quote(key);
        out.append(':').append(value);
        return this;
    }

    Json put(String key, boolean value) {
        if (!value) return this;   // false is the default everywhere here
        comma();
        quote(key);
        out.append(':').append("true");
        return this;
    }

    private void comma() {
        if (!first) out.append(',');
        first = false;
    }

    /** RFC 8259 escaping, including the C1 range a naive writer forgets.
     *
     * **Everything outside printable ASCII leaves as `\\uXXXX`, and that is a
     * bug fix rather than a preference.** This writer's output goes to
     * `System.out`, and a `PrintStream` encodes with `stdout.encoding` --
     * which is *not* `file.encoding` and does not follow JEP 400. Measured on
     * the Forge worker (2026-09-07), where the image sets no locale at all:
     *
     *     file.encoding   = UTF-8
     *     stdout.encoding = ANSI_X3.4-1968      <- US-ASCII
     *
     * So every character above 0x7e was being handed to a US-ASCII encoder,
     * which does not fail -- it substitutes `?`. `Mjolnir, Storm Hammer` with
     * its umlaut went over the wire as `Mj?lnir, Storm Hammer`, missed the
     * card pool's exact-name lookup, and drew a blank plate on the Coliseum
     * board. Ninety-seven cards in the pool have a non-ASCII name -- Lorien
     * Revealed, Barad-dur, Anduril, Nazgul, Eowyn -- and every one of them was
     * blank in every match.
     *
     * **The substitution is lossy, so nothing downstream could have fixed
     * it**: `Mj?lnir` cannot be folded back into a name. It has to not happen,
     * and an ASCII-only wire is the way it cannot: `\\uXXXX` is plain JSON that
     * `encoding/json` decodes without being asked, and it stays correct on any
     * host, under any locale, whatever a future `PrintStream` is built with.
     *
     * Java `char` is a UTF-16 code unit, so an astral character escapes as its
     * two surrogates -- which is exactly what RFC 8259 asks for and what every
     * decoder reassembles. */
    private void quote(String s) {
        out.append('"');
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"':  out.append("\\\""); break;
                case '\\': out.append("\\\\"); break;
                case '\n': out.append("\\n");  break;
                case '\r': out.append("\\r");  break;
                case '\t': out.append("\\t");  break;
                case '\b': out.append("\\b");  break;
                case '\f': out.append("\\f");  break;
                default:
                    if (c < 0x20 || c > 0x7e) {
                        out.append(String.format("\\u%04x", (int) c));
                    } else {
                        out.append(c);
                    }
            }
        }
        out.append('"');
    }

    @Override public String toString() { return out.append('}').toString(); }
}
