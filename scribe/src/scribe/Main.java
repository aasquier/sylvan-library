// Copyright (C) 2026 sylvan-library contributors
//
// This file is part of the Forge scribe, which links against Forge and is
// therefore licensed under the GNU General Public License version 3.
// See LICENSE in this directory.
package scribe;

import java.io.File;
import java.io.PrintStream;
import java.util.ArrayList;
import java.util.EnumSet;
import java.util.List;
import java.util.Random;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

import forge.GuiDesktop;
import forge.deck.Deck;
import forge.deck.io.DeckSerializer;
import forge.game.Game;
import forge.game.GameOutcome;
import forge.game.GameRules;
import forge.game.GameType;
import forge.game.Match;
import forge.game.player.RegisteredPlayer;
import forge.gui.GuiBase;
import forge.model.FModel;
import forge.player.GamePlayerUtil;
import forge.util.MyRandom;
import forge.view.TimeLimitedCodeBlock;

/**
 * The scribe's runner: Forge's own match, with one more listener on it.
 *
 * **Everything here mirrors `forge.view.SimulateMatch`, and the mirroring is
 * the contract.** Forge builds each `Game` with its own private `EventBus` and
 * registers only its game log on it — `Match.subscribeToEvents` does not
 * propagate — so there is no way to attach a listener from outside without
 * owning the two lines between `createGame()` and `startGame()`. That is the
 * whole reason this class exists, and it is deliberately the smallest thing
 * that can hold those two lines.
 *
 * The consequence, and ADR 42's third decision: if this drives Forge even
 * slightly differently from `sim`, every match already in the ledger becomes
 * incomparable with every match after it. The five things that must stay
 * identical are marked below with the word PARITY. Do not change one without
 * re-running the parity gate.
 *
 * Usage, deliberately positional and dumb — the Go side builds this argv and
 * nothing else ever will:
 *
 *   scribe.Main <clock-seconds> <games> <seed|-> <deck.dck> <deck.dck> [...]
 */
public final class Main {
    public static void main(String[] args) {
        if (args.length < 5) {
            System.err.println("usage: scribe <clock> <games> <seed|-> <deck.dck> <deck.dck> [...]");
            System.exit(2);
        }
        int clock = Integer.parseInt(args[0]);
        int games = Integer.parseInt(args[1]);
        String seed = args[2];

        // Forge's own boot, in Forge's own order. `forge.view.Main` installs
        // the desktop GUI interface before it dispatches to `sim`, and
        // `ForgeConstants` reads it in a static initialiser — so without this
        // the very first touch of the card database is a
        // NullPointerException out of `GuiBase.getInterface()`. It draws
        // nothing; it is where Forge keeps its asset paths.
        GuiBase.setInterface(new GuiDesktop());
        // Everything below depends on the card database being loaded, and
        // this is the call that loads it — the ~15s of the subprocess that is
        // neither our fault nor avoidable.
        FModel.initialize(null, null);

        // PARITY 1: a *global* RNG, seeded once before any game is created.
        // Seeding later, or per game, plays different Magic.
        if (!"-".equals(seed)) {
            MyRandom.setRandom(new Random(Long.parseLong(seed)));
        }

        GameType type = GameType.Commander;
        List<RegisteredPlayer> seats = new ArrayList<>();
        for (int i = 3; i < args.length; i++) {
            Deck deck = DeckSerializer.fromFile(new File(args[i]));
            if (deck == null) {
                System.err.println("scribe: unreadable deck " + args[i]);
                System.exit(3);
            }
            // PARITY 2: `forCommander`, not `new RegisteredPlayer` — the
            // command zone is the difference.
            RegisteredPlayer seat = RegisteredPlayer.forCommander(deck);
            // PARITY 3: the AI profile and the seat index it is created with.
            seat.setPlayer(GamePlayerUtil.createAiPlayer(deck.getName(), i - 3));
            seats.add(seat);
        }

        GameRules rules = new GameRules(type);
        // PARITY 4: the applied variants set, which is what makes this
        // Commander rather than Constructed with a funny deck.
        rules.setAppliedVariants(EnumSet.of(type));
        // PARITY 5: the clock, enforced below by Forge's own timed block.
        rules.setSimTimeout(clock);

        Match match = new Match(rules, seats, "Scribe");
        PrintStream out = System.out;
        Scribe scribe = new Scribe(out);

        for (int number = 1; number <= games; number++) {
            // The two lines this whole class exists for. `createGame` builds
            // the Game and its bus; `startGame` runs it. Between them is the
            // only moment a listener can be attached.
            Game game = match.createGame();
            // The game goes over with the number: the scribe reaches back
            // through it for `Card.wasCast()`, which is the one question a
            // `CardView` cannot answer. See `Scribe#entered`.
            scribe.nextGame(number, game);
            game.subscribeToEvents(scribe);

            long began = System.nanoTime();
            boolean clocked = false;
            try {
                TimeLimitedCodeBlock.runWithTimeout(() -> match.startGame(game),
                        rules.getSimTimeout(), TimeUnit.SECONDS);
            } catch (TimeoutException e) {
                // Forge's own wording for this state is "Stopping slow match
                // as draw". A clock-out is the measurement giving up rather
                // than a game ending, and it is reported apart from a draw all
                // the way up.
                clocked = true;
            } catch (Exception e) {
                System.err.println("scribe: game " + number + " failed: " + e);
            }
            long ms = (System.nanoTime() - began) / 1_000_000L;

            GameOutcome outcome = game.getOutcome();
            RegisteredPlayer won = null;
            if (!clocked && outcome != null && !outcome.isDraw()) {
                won = outcome.getWinningPlayer();
            }
            Json line = new Json("result").put("game", number)
                    .put("ms", (int) ms).put("timed_out", clocked);
            if (won != null) {
                // **The seat, one-based, and the name.** The seat is what the
                // Go side works in — `SimRun.Seats` maps 1..n to slugs in the
                // order the decks were passed, and the parity gate compares
                // seats because two decks can share a name and never share a
                // seat. The name rides along because it is what Forge's own
                // result line carries, so a human reading this stream sees
                // what they would have seen in the log.
                line.put("seat", seats.indexOf(won) + 1)
                    .put("winner", won.getPlayer().getName());
            } else if (!clocked) {
                line.put("draw", true);
            }
            out.println(line);
            out.flush();
        }
    }

    private Main() { }
}
