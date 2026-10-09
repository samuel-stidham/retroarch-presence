// Command retroarch-presence shows the game RetroArch is running as a Discord
// presence such as "[N64] Ocarina of Time". See docs/spec.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/application"
	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/config"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/discord"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/retroarch"
)

const (
	// udpTimeout bounds each GET_STATUS. The minimum poll interval is one
	// second, so a poll always finishes before the next is due.
	udpTimeout = time.Second
	// exitConfig is sysexits' EX_CONFIG. The systemd unit lists it in
	// RestartPreventExitStatus, so a broken config does not restart in a loop.
	exitConfig = 78
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", config.DefaultPath(os.Getenv), "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "retroarch-presence:", err)
		return exitConfig
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "retroarch-presence:", err)
		return 1
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}

	publisher := discord.NewClient(cfg.ApplicationID, cfg.Display, discord.SocketDialer(os.Getenv), log)
	defer publisher.Close()

	update := application.NewUpdatePresence(
		retroarch.UDPSource{Address: cfg.RetroArchAddress, Timeout: udpTimeout},
		retroarch.PlaylistCatalog{ConfigDir: cfg.RetroArchConfigDir, ConfigHome: configHome, Home: home},
		publisher,
		systemClock{},
		application.Settings{
			Systems:     domain.DefaultSystems().WithOverrides(cfg.Systems),
			Pins:        cfg.Titles,
			PausedImage: cfg.PausedImage,
		},
		log,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Info("started", "config", *configPath, "poll_interval", cfg.PollInterval, "retroarch", cfg.RetroArchAddress)
	update.Run(ctx, cfg.PollInterval)
	log.Info("stopped")
	return 0
}
