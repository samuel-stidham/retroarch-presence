# retroarch-presence

A small Linux daemon that shows what RetroArch is running as your Discord
activity, written as `[N64] Ocarina of Time` or `[NES] Dragon Warrior IV`. It
replaces RetroArch's built-in Discord presence.

It polls RetroArch's network command interface over UDP and names the game from
your RetroArch playlists. It then talks to the Discord desktop client over its
local IPC socket with your own application id. There is no bot token, no
Gateway connection, and no login. It uses only the Go standard library.

`docs/spec.md` describes the exact behavior and the reasoning behind it.

## Setup

1. **Create a Discord application.** Go to
   <https://discord.com/developers/applications> and select New Application.
   Its name heads your activity card as "Playing <name>", so call it something
   like `RetroArch`. Copy the Application ID from General Information. To show
   console art, upload images under Rich Presence, Art Assets. Each image's key
   is what you put in `image =` later.

2. **Configure RetroArch.**
   - Settings > User Interface > Show Advanced Settings: **On**. Network
     Commands is an advanced setting and stays hidden without it.
   - Settings > Network > Network Commands: **On**. Leave the port at 55355.
   - Settings > User > Privacy > Discord Rich Presence: **Off**, so
     RetroArch's own presence doesn't compete.
   - Restart RetroArch. The network command interface only starts at launch.

3. **Configure Discord.** In User Settings > Activity Privacy, turn on the
   switch that shares your detected activity. Older builds call it "Display
   current activity as a status message". Without it nobody sees the activity,
   including you.

4. **Install the daemon.**

   ```sh
   go install github.com/samuel-stidham/retroarch-presence/cmd/retroarch-presence@latest
   # or, from a checkout:
   go install ./cmd/retroarch-presence
   ```

   This puts the binary at `~/go/bin/retroarch-presence`.

5. **Write the config.**

   ```sh
   mkdir -p ~/.config/retroarch-presence
   cp config.example.toml ~/.config/retroarch-presence/config.toml
   $EDITOR ~/.config/retroarch-presence/config.toml   # set application_id
   ```

6. **Run it as a user service.**

   ```sh
   mkdir -p ~/.config/systemd/user
   cp contrib/systemd/retroarch-presence.service ~/.config/systemd/user/
   systemctl --user daemon-reload
   systemctl --user enable --now retroarch-presence
   ```

Load a game. Your Discord activity changes within a poll or two, about ten
seconds at most.

## Configuration

Every key, with its default, is documented in `config.example.toml`. Only
`application_id` is required. The ones you are most likely to touch are below.

- `display` chooses where the `[LABEL] Title` text goes. `"details"` puts it on
  the line under the header, which always renders. `"name"` puts it in the
  header itself, as in "Playing [N64] Ocarina of Time". Discord accepts the
  `name` field from local clients, but whether every view honours it is still
  being confirmed. See the open question in `docs/spec.md`.
- `[systems.<id>]` sets a `label` or a large `image` per system. The id is what
  RetroArch reports, such as `nintendo_64` or `super_nes`, never `n64`. Run with
  `log_level = "info"` to see the id of whatever you load.
- `[titles]` pins an exact title per file, keyed by file name without its
  extension.

## Gotchas

- **RetroArch's own Discord toggle must be off.** Otherwise both programs set
  your activity and you see whichever wrote last.
- **Network Commands must be on.** Otherwise nothing answers on UDP 55355, and
  the daemon waits quietly as if RetroArch were closed. The option is hidden
  until Show Advanced Settings is on, and it takes effect after a restart.
- **Discord's activity sharing must be on.** Otherwise Discord accepts every
  update and shows none of them.
- **The Flatpak RetroArch can break Discord's IPC socket.** The Flathub build
  grants itself `xdg-run/discord-ipc-0:create`. If RetroArch starts before
  Discord, Flatpak creates an empty *directory* at
  `$XDG_RUNTIME_DIR/discord-ipc-0`. Discord then cannot create its socket, and
  no local app can reach it, this daemon included. Remove the grant once, since
  this daemon replaces the feature it was for:

  ```sh
  flatpak override --user --nofilesystem=xdg-run/discord-ipc-0 \
    --nofilesystem=xdg-run/app/com.discordapp.Discord org.libretro.RetroArch
  rmdir "$XDG_RUNTIME_DIR/discord-ipc-0" 2>/dev/null   # if the directory exists
  ```

  Then restart Discord. The daemon also skips non-socket paths when it looks
  for Discord, so a stray directory never hides a socket at a later index.
- **Some cores report one system id for several systems.** Gambatte says
  `game_boy` for Game Boy Color games, and Genesis Plus GX says `mega_drive` for
  Master System and Game Gear. The daemon tells them apart by the file extension
  of the matching playlist entry. Content that is in no playlist gets the base
  label, such as `GB` for a `.gbc` file.
- **Disc images often report a CRC of zero.** Title matching then relies on the
  file name, so keep the file name the same as the one in the playlist.
- **Updates are rate limited** to one every five seconds, which keeps clear of
  Discord's limit. A quick pause and resume may never show.

## Troubleshooting

```sh
systemctl --user status retroarch-presence
journalctl --user -u retroarch-presence -f
```

The default log level is `error`, so a healthy daemon prints nothing. Set
`log_level = "debug"` and restart the service to watch every poll and publish.
Check Discord's socket with `ss -xl | grep discord-ipc`. Check RetroArch's port
with `ss -ulpn | grep 55355`.

## Development

```sh
go test -race ./...
```

The tests are the behavioral spec, written given/then style with the standard
library. `internal/domain` holds the pure rules. `internal/application` holds the
one use case and its ports. `internal/infrastructure` holds the UDP, playlist,
IPC, and config adapters. A fitness test in `internal/architecture` fails if the
inner layers import outward.
