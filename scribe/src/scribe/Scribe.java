// Copyright (C) 2026 sylvan-library contributors
//
// This file is part of the Forge scribe, which links against Forge and is
// therefore licensed under the GNU General Public License version 3.
// See LICENSE in this directory.
package scribe;

import java.io.PrintStream;
import java.util.HashMap;
import java.util.Map;

import com.google.common.eventbus.Subscribe;

import forge.game.card.CardView;
import forge.game.event.EventValueChangeType;
import forge.game.event.GameEvent;
import forge.game.event.GameEventCardCounters;
import forge.game.event.GameEventCardStatsChanged;
import forge.game.event.GameEventCardTapped;
import forge.game.event.GameEventGameOutcome;
import forge.game.event.GameEventLandPlayed;
import forge.game.event.GameEventPlayerLivesChanged;
import forge.game.event.GameEventTurnBegan;
import forge.game.event.GameEventZone;
import forge.game.event.IGameEventVisitor;
import forge.game.player.PlayerView;

/**
 * A listener on Forge's game event bus, writing the board as it changes.
 *
 * Forge's *log* renders about twenty-five of the fifty-nine events its bus
 * carries, and none of the ones a battlefield is made of: there is no TOKEN
 * and no COUNTER in `GameLogEntryType` at all, which is why a Food deck can
 * play fifteen turns without the log mentioning a single Food. The events
 * themselves have always been there. This subscribes to them.
 *
 * **It listens; it never decides.** No rule is evaluated here and nothing is
 * inferred from anything else — every line out of this class is one event
 * Forge announced, rendered as itself. Reconstructing a board from these
 * belongs on the far side of the pipe, where it is testable without a JVM
 * (ADR 14's division, applied to a subprocess).
 *
 * The one piece of state is [#seen], and it is bookkeeping rather than
 * judgement: `GameEventCardStatsChanged` fires on almost every priority pass
 * and re-sends the whole card whether anything moved or not. Measured on one
 * real game (Gyome/Food against Atla/Eggs, 2026-08-25): **3,300 of 3,750
 * lines were stats, and almost none of them were news.** Suppressing a
 * repeat is not an opinion about the game; sending a hundred identical lines
 * about a 0/0 Food token is.
 *
 * Guava's `EventBus` dispatches on the declared parameter type, so the single
 * `@Subscribe` below receives every event and the double dispatch through
 * `visit` sorts them. Anything not overridden falls through
 * `IGameEventVisitor.Base` and costs nothing — which is the point: this is a
 * partial listener by design, and a Forge release that adds an event is
 * silence rather than a crash.
 */
public final class Scribe extends IGameEventVisitor.Base<Void> {
    private final PrintStream out;
    /** Which game of the match this is; the far side keys beats on it. */
    private int game;
    /**
     * The last thing said about each card, by Forge's own instance id.
     *
     * Cleared between games because ids are per game, and a stale entry would
     * silence the *first* fact about a card in game two.
     */
    private final Map<Integer, String> seen = new HashMap<>();

    public Scribe(PrintStream out) {
        this.out = out;
    }

    /** Called by the runner between games, before the next one is created. */
    public void nextGame(int number) {
        this.game = number;
        seen.clear();
        say(new Json("game").put("game", number));
    }

    /**
     * The one entry point the bus knows about.
     *
     * `GameEvent` is the declared type, and Guava matches by assignability, so
     * this receives all of them. Forge's own `GameLogFormatter` spells its
     * equivalent `recieve`; the spelling is not part of the contract, the
     * annotation is.
     */
    @Subscribe
    public void receive(GameEvent event) {
        try {
            event.visit(this);
        } catch (RuntimeException e) {
            // **A scribe must never take the match down.** This runs inside
            // Forge's own event dispatch, on the thread playing the game, so
            // an exception here would end a match that was otherwise fine —
            // and the whole point of the worker is to produce a result. A
            // dropped line is a hole in a picture; a thrown one is a lost
            // game.
            say(new Json("scribe_error").put("detail", String.valueOf(e)));
        }
    }

    // ------------------------------------------------------------- the board

    /**
     * Every card entering or leaving any zone, with whose zone it was.
     *
     * This is the event the board is built out of, and the reason tokens are
     * reachable at all: `GameEventTokenCreated` is a record with no fields —
     * a bare signal — but a token arriving on the battlefield is a zone change
     * carrying the token's own `CardView`, name, id and all.
     */
    @Override
    public Void visit(GameEventZone event) {
        CardView card = event.card();
        if (card == null || event.zoneType() == null) return null;
        Json line = new Json("zone")
                .put("game", game)
                .put("zone", event.zoneType().name())
                .put("mode", event.mode() == EventValueChangeType.Added ? "in" : "out");
        who(line, event.player());
        return card(line, card);
    }

    /** Tapped state, exactly — rather than inferred from a mana line. */
    @Override
    public Void visit(GameEventCardTapped event) {
        Json line = new Json("tapped").put("game", game).put("tapped", event.tapped());
        return card(line, event.card());
    }

    /**
     * Counters, exactly.
     *
     * Both totals cross rather than a delta: a consumer that added deltas
     * would drift the first time one was dropped, and `newValue` is the
     * answer to the only question a board asks.
     */
    @Override
    public Void visit(GameEventCardCounters event) {
        Json line = new Json("counters")
                .put("game", game)
                .put("counter", event.type() == null ? "?" : event.type().getName())
                .put("was", event.oldValue())
                .put("now", event.newValue());
        return card(line, event.card());
    }

    /** Power and toughness as they change — pumps, anthems, counters landing. */
    @Override
    public Void visit(GameEventCardStatsChanged event) {
        if (event.cards() == null) return null;
        for (CardView card : event.cards()) {
            if (card == null) continue;
            // Only when something actually changed — see `seen`.
            card(new Json("stats").put("game", game), card, true);
        }
        return null;
    }

    @Override
    public Void visit(GameEventPlayerLivesChanged event) {
        Json line = new Json("life").put("game", game);
        who(line, event.player());
        say(line.put("life", event.newLives()));
        return null;
    }

    @Override
    public Void visit(GameEventLandPlayed event) {
        Json line = new Json("land").put("game", game);
        who(line, event.player());
        return card(line, event.land());
    }

    @Override
    public Void visit(GameEventTurnBegan event) {
        Json line = new Json("turn").put("game", game);
        who(line, event.turnOwner());
        say(line);
        return null;
    }

    @Override
    public Void visit(GameEventGameOutcome event) {
        say(new Json("outcome").put("game", game));
        return null;
    }

    // ------------------------------------------------------------- rendering

    /**
     * A card, as much of it as the view will say.
     *
     * The **id is the identity**, not the name: a deck holds four Forests and
     * a board has to tell them apart, and Forge numbers every card in a game.
     * `isToken` rides along because a token's id is real but its card is not,
     * and a consumer resolving names to art must know not to look one up.
     */
    private Void card(Json line, CardView card) {
        return card(line, card, false);
    }

    /**
     * `onlyIfChanged` is for the events that fire on every priority pass and
     * re-send a card that has not moved. The comparison is on the rendered
     * text rather than on fields, because the rendering is exactly what the
     * far side would receive — anything a reader cannot tell apart is a line
     * not worth sending.
     */
    private Void card(Json line, CardView card, boolean onlyIfChanged) {
        if (card == null) return null;
        line.put("id", card.getId()).put("card", card.getName())
            .put("token", card.isToken());
        CardView.CardStateView state = card.getCurrentState();
        if (state != null) {
            line.put("power", state.getPower()).put("toughness", state.getToughness());
            if (state.getType() != null) line.put("types", state.getType().toString());
        }
        String rendered = line.toString();
        if (onlyIfChanged && rendered.equals(seen.put(card.getId(), rendered))) {
            return null;
        }
        if (!onlyIfChanged) seen.put(card.getId(), rendered);
        out.println(rendered);
        out.flush();
        return null;
    }

    private void who(Json line, PlayerView player) {
        if (player == null) return;
        line.put("seat", player.getId()).put("who", player.getName());
    }

    /**
     * One line, flushed.
     *
     * Flushed per line because the far side reads this stream *live* — a
     * buffered scribe would deliver a whole match at once and the room would
     * watch a blank screen for two minutes and then a flood.
     */
    private void say(Json line) {
        out.println(line);
        out.flush();
    }
}
