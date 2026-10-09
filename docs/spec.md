# retroarch-presence spec

retroarch-presence is a Linux user daemon that replaces RetroArch's built-in
Discord presence. It shows what you are playing as `[N64] Ocarina of Time`.
Every task on this repo reads this file first. When behavior changes, this file
changes in the same commit.

## Behavior

**Polling.** Every `poll_interval` (default 5s) the daemon sends `GET_STATUS` to
RetroArch's Network Control Interface over UDP at `127.0.0.1:55355`. Each poll
uses a fresh socket with a one second read deadline, so a late reply can never
be read as the answer to the next poll. A timeout or a refused port means
RetroArch is not running. One miss is ignored. Two misses in a row clear the
presence. Recovery is silent apart from debug logs.

**Status replies.** RetroArch 1.22.2 builds the reply in `command_get_status`
in `command.c`. The exact format is:

```
GET_STATUS PLAYING <system_id>,<basename>,crc32=<crc>\n
GET_STATUS PAUSED <system_id>,<basename>,crc32=<crc>\n
GET_STATUS CONTENTLESS
```

`<crc>` is printed with `%lx`. It is lowercase, unpadded, and can be `0`.
`<basename>` is the content file name without directory or extension, and it can
contain commas. The parser takes `system_id` up to the first comma and the CRC
after the last `,crc32=`. Everything between them is the basename.
`system_id` comes from the core's info file. When a core has no info file,
RetroArch sends its library name instead, which can contain spaces.

**Labels.** The `system_id` maps to a short bracket label. The default table is
keyed on the ids RetroArch actually sends, which were read from the core info
files: `nes`, `super_nes`, `nintendo_64`, `game_boy`, `game_boy_advance`,
`mega_drive`, `master_system`, `sega_saturn`, `playstation`, `playstation2`,
`dreamcast`, and so on. Shorthand like `snes` or `n64` never arrives.

Some cores report one id for several systems. Gambatte reports Game Boy Color
games as `game_boy`. Genesis Plus GX reports Master System and Game Gear games as
`mega_drive`. So a key may also name a file extension, as in `game_boy.gbc`.
The extension comes from the matched playlist entry's path. Lookup order is
config `id.ext`, default `id.ext`, config `id`, default `id`. An unknown id
falls back to the id itself in upper case, and the daemon logs it at info level.

**Titles.** Resolution stops at the first hit.

1. A pin in config for this exact basename.
2. A playlist entry whose path has the same basename. If several entries
   share it, the one whose CRC matches wins.
3. A playlist entry with the same CRC, when the CRC is not zero.
4. The basename itself.

Playlists are every `*.lpl` file under RetroArch's `playlist_directory`,
including `builtin/`. History entries rank below the system playlists. Entries
with an empty label are skipped. An archive path such as `a.zip#b.sfc` matches
on `b`. The playlist index is rebuilt each time new content loads.

Labels from steps 2 to 4 are cleaned. Every `(...)` and `[...]` group is
removed, which covers region tags like `(USA)` and dump tags like `[!]`. A
trailing article moves to the front, so `Legend of Zelda, The - Ocarina of Time`
becomes `The Legend of Zelda - Ocarina of Time`. Pins are used exactly as
written.

**Presence.** The text is `[LABEL] Title`. While paused, the state line reads
`Paused` and the optional `paused_image` shows as the small image. The session
timer starts when content loads. It resets when the text changes and survives
pause and resume. `CONTENTLESS` clears the presence. A large image key per
system can be set in config, and its hover text is the label.

**Discord.** The daemon speaks the local IPC protocol with your own application
id. There is no bot token, Gateway, or OAuth. It looks for a socket named
`discord-ipc-0` through `discord-ipc-9` in each of these directories, in order.

```
$XDG_RUNTIME_DIR                (else TMPDIR, TMP, TEMP, /tmp)
$XDG_RUNTIME_DIR/app/com.discordapp.Discord     Flatpak Discord
$XDG_RUNTIME_DIR/snap.discord                   Snap Discord
```

A path that exists but is not a socket is skipped. The connection is opened
lazily, the first time there is something to show. When Discord restarts the
connection drops, the daemon notices, and it reconnects and resends on a later
tick. At most one `SET_ACTIVITY` goes out per 5 seconds. Discord allows 5 per
20 seconds, so this leaves headroom. An update that is unchanged since the last
send is never sent. A change that lands inside the 5 second window waits for
the next tick after it, and the latest state wins.

**Logging.** Logs go to stderr through `log/slog`. The level comes from config
and defaults to `error`, which keeps a healthy daemon silent.

## Data flow

```
RetroArch ──UDP GET_STATUS──▶ StatusSource ─Status─▶ UpdatePresence ◀─entries─ GameCatalog (*.lpl)
                                                         │  domain: label, title, session
                                                         ▼
                                              PresencePublisher ──IPC SET_ACTIVITY──▶ Discord
```

`UpdatePresence` is the one use case. Each tick reads a status, resolves the
label and title through pure domain rules, advances the session, and publishes
when the result differs from what Discord is showing.

## Layout

```
cmd/retroarch-presence/            entry point, wiring, signals
internal/domain/                   status, systems, titles, session, presence. No I/O.
internal/application/              UpdatePresence and its ports
internal/infrastructure/retroarch/ UDP status source, GET_STATUS parser, playlists, retroarch.cfg
internal/infrastructure/discord/   IPC framing, socket discovery, activity payload
internal/infrastructure/config/    config.toml loader
internal/architecture/             fitness test for the dependency rule
```

Ports are `StatusSource`, `GameCatalog`, `PresencePublisher`, and `Clock`.
The fitness test fails when `domain` or `application` imports anything under
`infrastructure` or `cmd`. Domain tests use no doubles. Application tests use
fakes for the ports, which stand in for the UDP and IPC sides.

## Config

The config lives at `$XDG_CONFIG_HOME/retroarch-presence/config.toml`, or at
`~/.config/...` when that variable is unset. `config.example.toml` in the repo
documents every key.

```toml
application_id = "000000000000000000"   # required
poll_interval = "5s"
retroarch_address = "127.0.0.1:55355"
retroarch_config_dir = ""               # empty means auto-detect
display = "details"                     # "details" or "name", see below
log_level = "error"                     # debug, info, warn, error
paused_image = ""

[systems.nintendo_64]
label = "N64"
image = "n64"

[systems."game_boy.gbc"]
label = "GBC"

[titles]
"Legend of Zelda, The - Ocarina of Time (USA)" = "Ocarina of Time"
```

Go's standard library has no TOML parser. The loader implements the subset
this file needs. That subset is comments, bare and quoted keys, `[table]` and
`[a."b.c"]` headers, and basic or literal strings. Anything else is a load
error with a line number. A full TOML dependency would replace it if the
format ever grows.

## Open question: the name override

Discord's activity card header reads `Playing <application name>`. An
activity sent over IPC may carry a `name` field. It is unknown whether Discord
honours that field from a local client.

The spike sends `SET_ACTIVITY` with `name` set and checks the header. If the
header changes, `display = "name"` puts the text in `name`. If it does not, the
text goes in `details` with `status_display_type = 2`, so the status line in the
member list shows the details.

**Result, 2026-10-09, partial.** The spike connected to native Discord with
RetroArch's public application id `475456035851599874`. It sent `name`,
`details`, `state`, `status_display_type = 2`, and a start timestamp. Discord
answered with every field echoed back unchanged.

```
"data":{"details":"[SNES] Spike Details","name":"[SNES] Spike Name Override",
"state":"Spike State","status_display_type":2,...,"application_id":"475456035851599874"}
```

So the client stores the `name` override rather than swapping in the
application name. The echo cannot show what the header renders, or what friends
see. That needs a person looking at the activity card. Until someone confirms
it, `display` defaults to `details`, which is known to render. Both modes are
built.

## This machine

RetroArch is the Flathub build, `org.libretro.RetroArch` 1.22.2. Its config dir
is `~/.var/app/org.libretro.RetroArch/config/retroarch/`. Auto-detection reads
`playlist_directory` from `retroarch.cfg` there, expanding `~`. It falls back
to the native `~/.config/retroarch` and then to the Flatpak path. The Flatpak
shares the host network, so the daemon reaches the UDP port from outside the
sandbox.

Discord is the native install launched from `/usr/bin/discord`.

The Flathub manifest grants RetroArch `xdg-run/discord-ipc-0:create`. When
RetroArch starts before Discord, Flatpak creates an empty directory at
`$XDG_RUNTIME_DIR/discord-ipc-0`. Discord then cannot bind its socket, and its
IPC server stays down. This machine was in that state when the project began.
The fix is a user override that drops the grant, since RetroArch's own presence
is off anyway.

```
flatpak override --user --nofilesystem=xdg-run/discord-ipc-0 \
  --nofilesystem=xdg-run/app/com.discordapp.Discord org.libretro.RetroArch
rmdir "$XDG_RUNTIME_DIR/discord-ipc-0"
```

After that, restart Discord. On 2026-10-09 this was applied here, and Discord
bound `discord-ipc-0` on its next start, which confirms the cause.
