// Copyright (C) 2026 sylvan-library contributors
//
// This file is part of the Forge scribe, which links against Forge and is
// therefore licensed under the GNU General Public License version 3.
// See LICENSE in this directory.
package scribe;

import java.io.PrintStream;
import java.util.HashMap;
import java.util.Map;

import com.google.common.collect.Multimap;
import com.google.common.eventbus.Subscribe;

import forge.card.MagicColor;
import forge.card.mana.ManaAtom;
import forge.game.GameEntityView;
import forge.game.card.CardView;
import forge.game.event.EventValueChangeType;
import forge.game.event.GameEvent;
import forge.game.event.GameEventAttackersDeclared;
import forge.game.event.GameEventBlockersDeclared;
import forge.game.event.GameEventCardAttachment;
import forge.game.event.GameEventCardCounters;
import forge.game.event.GameEventCardDamaged;
import forge.game.event.GameEventCardSacrificed;
import forge.game.event.GameEventCardStatsChanged;
import forge.game.event.GameEventCardTapped;
import forge.game.event.GameEventCombatEnded;
import forge.game.event.GameEventGameOutcome;
import forge.game.event.GameEventGameStarted;
import forge.game.event.GameEventLandPlayed;
import forge.game.event.GameEventManaPool;
import forge.game.event.GameEventMulligan;
import forge.game.event.GameEventPlayerDamaged;
import forge.game.event.GameEventPlayerLivesChanged;
import forge.game.event.GameEventSpellAbilityCast;
import forge.game.event.GameEventTurnBegan;
import forge.game.event.GameEventZone;
import forge.game.event.IGameEventVisitor;
import forge.game.keyword.KeywordView;
import forge.game.player.PlayerView;
import forge.game.spellability.SpellAbilityView;
import forge.game.spellability.StackItemView;
import forge.game.zone.ZoneType;

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
    /**
     * Forge's own player id to a **one-based** seat, learned at game start.
     *
     * `PlayerView.getId()` is zero-based for the first seat while everything
     * downstream of this pipe — `SimRun.Seats`, the result line below,
     * `GameEvent.Seat`, the shaping layer's slug map — counts from one. Both
     * numbering schemes were live in this one stream until now: the board
     * events said seat 0 and the result line said seat 1 about the same
     * player. That is a trap laid for whoever reads this next, so it is
     * resolved here, once, at the source.
     *
     * **Learned rather than computed.** `GameEventGameStarted.players()` is
     * Forge's own list in its own order, so the mapping is read off it and the
     * `seat` lines it emits put it in the stream where a person debugging can
     * see it. `getId() + 1` survives as the fallback for the handful of setup
     * events Forge raises before the game announces it has started.
     */
    private final Map<Integer, Integer> seats = new HashMap<>();

    public Scribe(PrintStream out) {
        this.out = out;
    }

    /** Called by the runner between games, before the next one is created. */
    public void nextGame(int number) {
        this.game = number;
        seen.clear();
        seats.clear();
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

    /**
     * An Aura, Equipment or Fortification finding a host, or losing one.
     *
     * **`newTarget == null` is Forge's own sentinel for coming off**, and it
     * is the one thing here worth checking rather than assuming: the record is
     * `(equipment, oldEntity, newTarget)` and both of the last two are
     * nullable, so "attached to nothing" and "detached from something" are the
     * same shape read two ways. `Card.attachToEntity` fires it with a non-null
     * `newTarget` and an `oldEntity` that is null on a first attach;
     * `Card.unattachFromEntity` fires it with `aconst_null` in the target
     * slot. Read out of the bytecode with `javap -c` against 2.0.14, and
     * Forge's own `toString` on this record branches on exactly the same test.
     *
     * The old host is deliberately **not** reported on a detach. The far side
     * is already holding what this card was attached to — that is the whole
     * point of it having been told — and sending it a second time would be
     * two sources for one fact, with the losing one arriving later.
     *
     * A target is a card most of the time and a *player* for a curse, so the
     * two go out through the two helpers that already exist for exactly this
     * distinction. There is no third case worth a branch: a battle or a
     * planeswalker is a card.
     */
    @Override
    public Void visit(GameEventCardAttachment event) {
        CardView gear = event.equipment();
        if (gear == null) return null;
        GameEntityView onto = event.newTarget();
        Json line = new Json(onto == null ? "detach" : "attach").put("game", game);
        who(line, gear.getController());
        if (onto instanceof CardView host) {
            target(line, host);
        } else if (onto != null) {
            against(line, onto);
        }
        return card(line, gear);
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

    /**
     * The floating mana in one player's pool, whole, every time it moves.
     *
     * This is what a permanent tapping *for* something looks like before the
     * something happens (Aaron, 2026-08-26: *"show the mana pool as things tap
     * into it before it is drained to cast things"*). The board has always had
     * the tap and never the mana, so a land going sideways and a Sol Ring going
     * sideways said the same nothing.
     *
     * **The whole pool crosses rather than the change**, and that is a decoding
     * decision rather than a preference. `GameEventManaPool` carries a *mode*
     * and a set of colours that moved, and the set is **null** on the mode that
     * empties the pool — `ManaPool`'s clear path passes `aconst_null` where the
     * other two pass a set (`javap -c`). A reader that added and subtracted the
     * reported colours would therefore drift on exactly the event that matters
     * most, the one at the end of a step where everything unspent drains away.
     * Sending the totals costs six lookups and cannot drift.
     *
     * **The amounts are read off the view, and they are fresh.** The event
     * carries a `PlayerView` rather than the model's `ManaPool`, which is not
     * reachable from it — but all three sites in `ManaPool` that raise this
     * event call `Player.updateManaForView()` on the line before they construct
     * it, so the view's map is already the pool as it now stands. That was
     * checked in the bytecode rather than assumed, because a stale read here
     * would show mana one event behind and look like a Forge bug.
     *
     * @see #pool for the byte the colourless count hides behind.
     */
    @Override
    public Void visit(GameEventManaPool event) {
        PlayerView player = event.player();
        if (player == null) return null;
        Json line = new Json("mana").put("game", game);
        who(line, player);
        say(line.put("pool", pool(player)));
        return null;
    }

    /**
     * One player's floating mana as the symbols a person would write.
     *
     * `"GGW"` is two green and one white, in Magic's own colour order, and an
     * empty string is an empty pool — which is a real answer and the one that
     * ends every step, so it is said rather than omitted.
     *
     * **The colourless count lives at byte 32, not byte 0**, and that is the
     * trap this method exists to shut. Forge has two constant sets for the same
     * six things: `MagicColor` calls colourless 0 and `ManaAtom` calls it 32.
     * `PlayerView.updateMana` fills its map by iterating `ManaAtom.MANATYPES`,
     * so 32 is the key that is actually there, and `getMana(byte)` is a plain
     * map lookup with no masking — `getMana(MagicColor.COLORLESS)` asks for key
     * 0, misses, and returns zero forever. The two readings are indistinguishable
     * in the data: a pool with no colourless mana in it and a pool whose
     * colourless mana we asked the wrong question about both render `""`.
     * `Mana.isColorless()` is literally `color == 32`, which is what settled it.
     *
     * Mapping the other way is safe — `MagicColor.Color.fromByte` sends
     * everything it does not recognise to COLORLESS, so 32 comes back as the
     * right colour — which is why the symbol is read through it rather than
     * from a table written out here.
     */
    private static String pool(PlayerView player) {
        StringBuilder symbols = new StringBuilder(8);
        for (byte type : ManaAtom.MANATYPES) {
            int held = player.getMana(type);
            if (held <= 0) continue;
            MagicColor.Color colour = MagicColor.Color.fromByte(type);
            String symbol = colour == null ? "?" : colour.getShortName();
            for (int i = 0; i < held; i++) symbols.append(symbol);
        }
        return symbols.toString();
    }

    /**
     * A permanent sacrificed — which is not the same as one that died.
     *
     * Aaron asked for the Treasure that taps and is then cracked (*"they must
     * tap to sacrifice and they go into the ether"*), and that card raised no
     * beat at all: `dies` is creatures and planeswalkers by rule 700.4, and a
     * token leaving the battlefield is rewritten to `gone`, so the whole
     * transaction folded silently into the next step. A fetchland is the same
     * shape and this deck format is full of them.
     *
     * **Sacrifice is the only word on this bus, and the other two are not
     * available.** `GameEventCardDestroyed` is a record with **no components at
     * all** — a bare signal like `GameEventTokenCreated` — so nothing can be
     * said about *which* card was destroyed, and combat deaths are not
     * separately announced anywhere. Rather than guess a word from the
     * circumstances, this reports the one Forge actually names and the board
     * says nothing about the rest (ADR 44).
     */
    @Override
    public Void visit(GameEventCardSacrificed event) {
        return card(new Json("sacrificed").put("game", game), event.card());
    }

    /**
     * Combat is over — Forge's own signal, rather than the next turn beginning.
     *
     * **`GameEventCombatUpdate` is the wrong event and it looks like the right
     * one.** ADR 44 names it as the end-of-combat signal this board was missing,
     * and it is not one: a reference scan across all of Forge's classes finds it
     * constructed in exactly two places, `InputAttack` and `InputBlock` — the
     * *human* declare-attackers and declare-blockers handlers, which fire it on
     * every click so a person's own screen can keep up. Nothing in
     * `forge.game.**` posts it. In a headless AI match it never fires once, so a
     * listener built on it would compile, subscribe, and sit silent forever
     * while the board went on guessing.
     *
     * `GameEventCombatEnded` is the engine's own, posted from
     * `PhaseHandler.onPhaseEnd()` while `inCombat()`, and it carries the
     * attackers and blockers that were in the fight. Only the fact is used: the
     * far side is already holding who was fighting, and sending the roster again
     * would be two sources for one thing with the losing one arriving later.
     */
    @Override
    public Void visit(GameEventCombatEnded event) {
        say(new Json("combat_end").put("game", game));
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

    /**
     * A turn beginning, whose it is, and **Forge's own number for it**.
     *
     * The number was dropped until now and the far side counted turn lines to
     * recover it. It is on the event (`turnNumber()`), so it is reported.
     *
     * **It is a player-turn, not a round.** Forge increments once per player
     * and alternates seats, so its "turn 15" is one player's eighth. That is
     * Magic's own rule and it is not what a Magic *player* means by "turn
     * eight", which is why `web/src/lib/theater.ts` converts at the last
     * moment. Nothing here rounds it: the wire keeps what Forge said.
     *
     * The turn owner's life rides along. Forge announces life only when it
     * *changes*, so a board with no life event yet would have to assume the
     * format's opening total — and assuming is exactly what this class does
     * not do. A life on every turn line seeds both seats within a round and
     * re-states the truth fifteen times a game for the price of one integer.
     */
    @Override
    public Void visit(GameEventTurnBegan event) {
        Json line = new Json("turn").put("game", game).put("turn", event.turnNumber());
        who(line, event.turnOwner());
        if (event.turnOwner() != null) line.put("life", event.turnOwner().getLife());
        say(line);
        return null;
    }

    /**
     * The seats, in Forge's own order, with the life each starts on.
     *
     * This is what [#seats] is learned from, and the reason it is an event
     * rather than something the runner tells this class: `players()` is
     * Forge's list of who is actually at the table, so a seat number in this
     * stream is Forge's own ordering rather than our guess at it.
     */
    @Override
    public Void visit(GameEventGameStarted event) {
        if (event.players() == null) return null;
        int seat = 0;
        for (PlayerView player : event.players()) {
            if (player == null) continue;
            seats.put(player.getId(), ++seat);
            Json line = new Json("seat").put("game", game);
            who(line, player);
            say(line.put("life", player.getLife()));
        }
        return null;
    }

    /** A hand thrown back. The count is not on the event; the Hand zone says. */
    @Override
    public Void visit(GameEventMulligan event) {
        Json line = new Json("mulligan").put("game", game);
        who(line, event.player());
        say(line);
        return null;
    }

    /**
     * A spell being cast — and **only a spell**, which is the distinction the
     * prose parser had to make by reading a verb.
     *
     * `isSpell()` separates a cast from an activated ability and from a
     * trigger, where the log offered `cast`, `activated` and `triggered` as
     * three shapes of one sentence and the parser matched the first by hand.
     * Triggers are the bulk of stack traffic in a real game and are not a beat
     * anybody is watching for.
     */
    @Override
    public Void visit(GameEventSpellAbilityCast event) {
        SpellAbilityView sa = event.sa();
        if (sa == null) return null;
        CardView host = sa.getHostCard();
        if (host == null) return null;
        if (!sa.isSpell()) return ability(event, host);
        Json line = new Json("cast").put("game", game);
        who(line, host.getController());
        return card(line, host);
    }

    /**
     * An ability going on the stack: which card is using it, and from where.
     *
     * **Abilities never reached this stream at all** — the method above returned
     * on anything that was not a spell — so a commander sitting in the command
     * zone doing the thing it is in the deck to do was invisible. Aaron wants
     * eminence drawn (*"It can be used on the battlefield or from the command
     * zone… It should just visually indicate that an ability is being used"*),
     * and eminence is a triggered ability whose source never has to move, so
     * there was nothing anywhere in the pipe to draw it from.
     *
     * **This is not the stack, and it does not reopen the stack.** The board
     * refuses to model the stack as a zone because those events never balance —
     * 52 `Stack in` against 14 `Stack out` in one measured game — and that
     * remains true and untouched: `GameEventZone` for the Stack is still dropped
     * whole on the far side. This is a different event on a different subject.
     * It says *an ability was used*, once, at a moment, and nothing downstream
     * holds it afterwards or waits for it to come off anything. There is nothing
     * to leak, because nothing accumulates.
     *
     * **The zone is the eminence half.** `CardView.getZone()` is on the view, so
     * the far side can tell an ability used from the command zone from one used
     * on the battlefield without being told which cards are commanders — a
     * question ADR 44 deliberately left off this wire.
     *
     * `StackItemView` is where the kind lives. `SpellAbilityView` has only
     * `isSpell()`; the stack item knows `isTrigger()` as well, which separates
     * the ability a player chose to activate from the one the game raised on
     * their behalf. It is nullable, so its absence simply leaves the flag off.
     */
    private Void ability(GameEventSpellAbilityCast event, CardView host) {
        Json line = new Json("ability").put("game", game);
        who(line, host.getController());
        StackItemView item = event.si();
        if (item != null) line.put("trigger", item.isTrigger());
        ZoneType zone = host.getZone();
        if (zone != null) line.put("zone", zone.name());
        return card(line, host);
    }

    /**
     * Attackers, one line each, with what they were sent at.
     *
     * The map is keyed on the *defender* — a player, or a planeswalker, or a
     * battle — so the target is a `GameEntityView` and only its name is
     * dependable. When it is a player it gets a seat as well, which is what a
     * board needs to point the arrow at a side of the table.
     */
    @Override
    public Void visit(GameEventAttackersDeclared event) {
        Multimap<GameEntityView, CardView> map = event.attackersMap();
        if (map == null) return null;
        for (Map.Entry<GameEntityView, CardView> at : map.entries()) {
            Json line = new Json("attack").put("game", game);
            who(line, event.player());
            against(line, at.getKey());
            card(line, at.getValue());
        }
        return null;
    }

    /**
     * Blockers, one line per blocker, naming the attacker it stopped — and an
     * `unblocked` line for an attacker nobody stopped.
     *
     * **An attacker mapped to itself means it was not blocked**, and that is
     * Forge's own encoding rather than a guess at one. `PhaseHandler` builds
     * the multimap this event carries like this:
     *
     * <pre>
     *   map.putAll(attacker, combat.getBlockers(attacker).isEmpty()
     *           ? List.of(attacker)
     *           : combat.getBlockers(attacker));
     * </pre>
     *
     * so the empty case puts the attacker in as its own value. Reading that
     * sentinel is decoding, not deciding: without it fourteen of sixteen
     * blocks in a measured game read as a creature blocking itself. The
     * bytecode was checked rather than the pattern guessed, because the two
     * readings — "unblocked" and "blocked by itself" — look identical in the
     * data and only one of them is a fact.
     *
     * Forge's *log* said this by dropping its `Combat: ` prefix on every line
     * of a group after the first, which cost the old parser two unblocked
     * attackers out of three before anybody noticed. There is no prefix here
     * to lose.
     */
    @Override
    public Void visit(GameEventBlockersDeclared event) {
        if (event.blockers() == null) return null;
        for (Map.Entry<GameEntityView, Multimap<CardView, CardView>> defended
                : event.blockers().entrySet()) {
            if (defended.getValue() == null) continue;
            for (Map.Entry<CardView, CardView> pair : defended.getValue().entries()) {
                CardView attacker = pair.getKey();
                CardView blocker = pair.getValue();
                if (attacker == null || blocker == null) continue;
                boolean stopped = attacker.getId() != blocker.getId();
                Json line = new Json(stopped ? "block" : "unblocked").put("game", game);
                // The defending player either way: the one who blocked, or the
                // one who chose not to. It is their decision the line is about.
                who(line, event.defendingPlayer());
                if (stopped) target(line, attacker);
                card(line, blocker);
            }
        }
        return null;
    }

    /** Damage to a permanent: how much, from what, onto what. */
    @Override
    public Void visit(GameEventCardDamaged event) {
        Json line = new Json("damage").put("game", game)
                .put("amount", event.amount());
        target(line, event.card());
        return card(line, event.source());
    }

    /** Damage to a player — the kind that ends a game. */
    @Override
    public Void visit(GameEventPlayerDamaged event) {
        Json line = new Json("damage").put("game", game)
                .put("amount", event.amount()).put("combat", event.combat());
        against(line, event.target());
        return card(line, event.source());
    }

    /**
     * The end of a game, in Forge's own sentences.
     *
     * `outcomeStrings()` is a list, and [Json] writes flat objects only, so
     * each sentence is its own line carrying the same turn and winner. That is
     * not a workaround: a consumer wants them one at a time anyway, and Forge
     * writes all nine of them to follow "&lt;player&gt; has won/lost" — which
     * is what made the old regex have to be non-greedy, because "has lost
     * because an opponent has won by spell" holds both verbs and the last one
     * is the wrong one. A sentence handed over whole cannot be misread.
     */
    @Override
    public Void visit(GameEventGameOutcome event) {
        Json base = new Json("outcome").put("game", game)
                .put("turn", event.lastTurnNumber())
                .put("winner", event.winningPlayerName());
        if (event.outcomeStrings() == null || event.outcomeStrings().isEmpty()) {
            say(base);
            return null;
        }
        for (String said : event.outcomeStrings()) {
            say(new Json("outcome").put("game", game)
                    .put("turn", event.lastTurnNumber())
                    .put("winner", event.winningPlayerName())
                    .put("said", said));
        }
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
        // **This card is a copy, and this is the card whose ability made it.**
        //
        // Aaron on populate: *"It really is making a clone, or splitting one
        // thing into two"* — and to draw that you need to know a copy happened
        // at all. `GameEventTokenCreated` cannot help: it is a record with no
        // components, a bare signal. `CardView.getCloneOrigin()` can, and it is
        // on the view rather than model-only, so it costs nothing but this line.
        //
        // **It is not what the token was copied *from*, and the wrong reading
        // looks exactly like the right one.** In a real match a Centaur Token
        // populated by Growing Ranks came back with a clone origin of *Growing
        // Ranks* — the enchantment, not the Centaur Token beside it that was
        // actually copied. `TokenEffectBase` hands `sa.getHostCard()` to
        // `setCloneOrigin`, which the bytecode says plainly and a name like
        // "copied from" would quietly contradict. The permanent that was copied
        // is `Card.getCopiedPermanent()`, which is model-only and does not cross
        // this boundary; the far side is told what is true and not told what is
        // not (ADR 44).
        //
        // Only ever set by a copy effect, so **its presence is the copy**: a
        // Centaur Token minted fresh by Call of the Conclave carries nothing
        // here, and the populated one carries this. A whole-jar scan finds
        // `setCloneOrigin` called from one game effect and otherwise only from
        // the AI's own state-copying machinery, so there is no second meaning
        // waiting in another card.
        CardView copier = card.getCloneOrigin();
        if (copier != null) {
            line.put("copied_by", copier.getId())
                .put("copied_by_card", copier.getName());
        }
        CardView.CardStateView state = card.getCurrentState();
        if (state != null) {
            line.put("power", state.getPower()).put("toughness", state.getToughness());
            if (state.getType() != null) line.put("types", state.getType().toString());
            String keywords = keywords(state);
            if (!keywords.isEmpty()) line.put("keywords", keywords);
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

    /**
     * This card instance's keywords **as it currently has them**, not as it was
     * printed.
     *
     * The difference is the whole point. A board that reads keywords off the
     * printing gives every copy of a card the same marks forever, so Kaheera
     * standing beside a Beast changes the Beast's power and toughness on screen
     * and not the vigilance it just gained (Aaron, 2026-08-26: *"Some cards like
     * Kaheera give vigilance or another effect to other cards, we currently are
     * not representing that symbolically"*). `CardStateView.getKeywords()` is
     * recomputed by `updateKeywords`, which calls `Card.updateKeywordsCache`
     * first — so it is the live set, granted keywords included.
     *
     * **`original()` rather than `title()`.** The title is a display string
     * Forge localises; the original is the keyword as Forge's own card scripts
     * write it, which is the stable thing to match on and the thing that
     * survives a language setting.
     *
     * **Which keywords are *granted* is not decided here**, and cannot be:
     * `KeywordView` is a four-field record — original, keyword, title, reminder
     * — and carries no host, no source and not even the `isIntrinsic` flag the
     * model has. Attribution is erased at Forge's view boundary. So this sends
     * the whole live set and the far side, which knows what the card was
     * printed with, works out the difference.
     *
     * A comma joins them because [Json] writes flat objects of scalars on
     * purpose and no keyword Forge writes contains one — the parameterised ones
     * use a colon (`Ward:2`), which is why the far side splits on the comma and
     * keeps whatever is inside.
     */
    private static String keywords(CardView.CardStateView state) {
        StringBuilder all = new StringBuilder(32);
        for (KeywordView keyword : state.getKeywords()) {
            if (keyword == null || keyword.original() == null) continue;
            String word = keyword.original().trim();
            if (word.isEmpty() || word.indexOf(',') >= 0) continue;
            if (all.length() > 0) all.append(',');
            all.append(word);
        }
        return all.toString();
    }

    /**
     * The card on the *other* end of a beat — blocked, or damaged.
     *
     * Name and id only. A board already knows that card's stats from its own
     * events, and repeating them here would be two sources for one fact.
     */
    private void target(Json line, CardView card) {
        if (card == null) return;
        line.put("target_id", card.getId()).put("target", card.getName());
    }

    /**
     * The entity a beat is aimed at: a player, a planeswalker, or a battle.
     *
     * Only the name is dependable on a `GameEntityView`. A player gets a seat
     * too, because pointing an arrow at a side of the table needs one and a
     * name cannot be trusted to be unique — two seats can play decks with the
     * same title, and never share a seat.
     */
    private void against(Json line, GameEntityView entity) {
        if (entity == null) return;
        line.put("against", entity.getName());
        if (entity instanceof PlayerView player) {
            line.put("against_seat", seat(player));
        }
    }

    private void who(Json line, PlayerView player) {
        if (player == null) return;
        line.put("seat", seat(player)).put("who", player.getName());
    }

    /**
     * Forge's player id as a one-based seat.
     *
     * The learned mapping when the game has announced itself, and `getId() + 1`
     * for the setup events Forge raises before it does — the commanders
     * arriving in the command zone are the measured case. Both agree on every
     * game observed; the map is preferred because it is Forge's own ordering
     * rather than arithmetic on an id that is only conventionally dense.
     */
    private int seat(PlayerView player) {
        Integer known = seats.get(player.getId());
        return known != null ? known : player.getId() + 1;
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
