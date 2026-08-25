#!/usr/bin/env bash
# Compile the scribe against a Forge jar. GPL-3.0; see LICENSE beside this.
#
# One javac call and no build system, deliberately: this is three files, it
# has no dependencies of its own, and a Docker build stage that needs the
# network to fetch a build tool is a build that fails on somebody else's
# outage.
set -euo pipefail

jar="${1:-}"
if [ -z "$jar" ] || [ ! -f "$jar" ]; then
  echo "usage: build.sh <forge-gui-desktop-*-jar-with-dependencies.jar>" >&2
  exit 2
fi

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# **Which javac.** Forge 2.0.14 ships class file version 61 (Java 17), and the
# maintainer's Mac is pinned to macOS 12 with a stock Java 10 on PATH — so a
# bare `javac` fails with "class file has wrong version 61.0, should be 54.0",
# which is a true statement about the wrong compiler and reads like a corrupt
# jar. Honour JAVA_HOME, then Forge's own bundled JDK, then PATH.
#
# **The version is not probed by running it.** `/usr/bin/javac` on macOS is a
# stub that pops a GUI installer and blocks forever, so `javac -version` as a
# guard hangs the build on exactly the machine the guard was written for. The
# compile below is the probe; the hint after it is the guard.
if [ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/javac" ]; then
  javac="$JAVA_HOME/bin/javac"
elif [ -x "$HOME/.local/share/mtglab/jdk-21/Contents/Home/bin/javac" ]; then
  javac="$HOME/.local/share/mtglab/jdk-21/Contents/Home/bin/javac"
else
  javac="$(command -v javac || true)"
fi
if [ -z "$javac" ]; then
  echo "build.sh: no javac. Set JAVA_HOME to a JDK 17 or newer." >&2
  exit 2
fi

rm -rf "$here/out"
mkdir -p "$here/out"
if ! "$javac" -Xlint:all -cp "$jar" -d "$here/out" "$here"/src/scribe/*.java </dev/null; then
  echo >&2
  echo "build.sh: compile failed with $javac" >&2
  echo "          If that mentions 'class file has wrong version', it is too" >&2
  echo "          old — Forge needs Java 17 or newer. Set JAVA_HOME." >&2
  exit 1
fi
echo "scribe: built into $here/out with $javac"
