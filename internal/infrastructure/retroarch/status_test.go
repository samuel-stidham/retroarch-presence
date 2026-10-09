package retroarch_test

import (
	"testing"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/retroarch"
)

func TestParseStatus(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  domain.Status
	}{
		{
			name:  "given a playing reply whose basename has commas, it splits on the first comma and the last crc32",
			reply: "GET_STATUS PLAYING nintendo_64,Legend of Zelda, The - Ocarina of Time (USA),crc32=cd16c529\n",
			want:  domain.Status{State: domain.Playing, SystemID: "nintendo_64", Basename: "Legend of Zelda, The - Ocarina of Time (USA)", CRC32: 0xcd16c529},
		},
		{
			name:  "given a paused reply, the state is paused",
			reply: "GET_STATUS PAUSED nes,Dragon Warrior IV (USA),crc32=1f2b2c3d\n",
			want:  domain.Status{State: domain.Paused, SystemID: "nes", Basename: "Dragon Warrior IV (USA)", CRC32: 0x1f2b2c3d},
		},
		{
			name:  "given the menu with no content, the state is contentless",
			reply: "GET_STATUS CONTENTLESS",
			want:  domain.Status{State: domain.Contentless},
		},
		{
			name:  "given an unpadded crc, it parses as hex",
			reply: "GET_STATUS PLAYING super_nes,Tetris,crc32=1a\n",
			want:  domain.Status{State: domain.Playing, SystemID: "super_nes", Basename: "Tetris", CRC32: 0x1a},
		},
		{
			name:  "given a crc of zero, the crc is zero",
			reply: "GET_STATUS PLAYING playstation,Final Fantasy VII (USA) (Disc 1),crc32=0\n",
			want:  domain.Status{State: domain.Playing, SystemID: "playstation", Basename: "Final Fantasy VII (USA) (Disc 1)", CRC32: 0},
		},
		{
			name:  "given a core without an info file, the library name with spaces becomes the system id",
			reply: "GET_STATUS PLAYING Beetle PSX HW,Crash Bandicoot (USA),crc32=0\n",
			want:  domain.Status{State: domain.Playing, SystemID: "Beetle PSX HW", Basename: "Crash Bandicoot (USA)", CRC32: 0},
		},
		{
			name:  "given a CRLF line ending, it is trimmed",
			reply: "GET_STATUS PLAYING nes,Tetris,crc32=ff\r\n",
			want:  domain.Status{State: domain.Playing, SystemID: "nes", Basename: "Tetris", CRC32: 0xff},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := retroarch.ParseStatus(tc.reply)
			if err != nil {
				t.Fatalf("ParseStatus: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseStatusRejects(t *testing.T) {
	cases := []struct {
		name  string
		reply string
	}{
		{"given an empty reply", ""},
		{"given a reply to another command", "PONG"},
		{"given an unknown state", "GET_STATUS RUNNING nes,Tetris,crc32=1"},
		{"given no content fields", "GET_STATUS PLAYING"},
		{"given no comma after the system id", "GET_STATUS PLAYING nes"},
		{"given no crc field", "GET_STATUS PLAYING nes,Tetris"},
		{"given a crc that is not hex", "GET_STATUS PLAYING nes,Tetris,crc32=zz"},
		{"given a crc wider than 32 bits", "GET_STATUS PLAYING nes,Tetris,crc32=1ffffffff"},
	}

	for _, tc := range cases {
		t.Run(tc.name+", it returns an error", func(t *testing.T) {
			if got, err := retroarch.ParseStatus(tc.reply); err == nil {
				t.Errorf("ParseStatus(%q) = %+v, want an error", tc.reply, got)
			}
		})
	}
}
