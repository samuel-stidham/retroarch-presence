package domain

import "strings"

// System is how one system shows up in Discord.
type System struct {
	Label string
	Image string // Discord asset key for the large image, empty for none
}

// Systems maps RetroArch system ids to their display. A key is either a system
// id such as "game_boy" or an id refined by content extension such as
// "game_boy.gbc", for cores that report one id for several systems.
type Systems struct {
	byKey map[string]System
}

// defaultSystems is keyed on the ids RetroArch actually sends, which come from
// the systemid field of each core's info file.
var defaultSystems = map[string]System{
	"nes":                  {Label: "NES"},
	"super_nes":            {Label: "SNES"},
	"nintendo_64":          {Label: "N64"},
	"gamecube":             {Label: "GC"},
	"gamecube.wbfs":        {Label: "Wii"},
	"gamecube.wad":         {Label: "Wii"},
	"game_boy":             {Label: "GB"},
	"game_boy.gbc":         {Label: "GBC"},
	"game_boy.cgb":         {Label: "GBC"},
	"game_boy_advance":     {Label: "GBA"},
	"game_boy_advance.gb":  {Label: "GB"},
	"game_boy_advance.gbc": {Label: "GBC"},
	"nds":                  {Label: "NDS"},
	"3ds":                  {Label: "3DS"},
	"virtual_boy":          {Label: "VB"},
	"pokemon_mini":         {Label: "PM"},
	"mega_drive":           {Label: "Genesis"},
	"mega_drive.sms":       {Label: "SMS"},
	"mega_drive.gg":        {Label: "GG"},
	"mega_drive.sg":        {Label: "SG-1000"},
	"mega_drive.32x":       {Label: "32X"},
	"mega_drive.cue":       {Label: "Sega CD"},
	"mega_drive.chd":       {Label: "Sega CD"},
	"master_system":        {Label: "SMS"},
	"master_system.gg":     {Label: "GG"},
	"sega_saturn":          {Label: "Saturn"},
	"dreamcast":            {Label: "Dreamcast"},
	"playstation":          {Label: "PS1"},
	"playstation2":         {Label: "PS2"},
	"playstation_portable": {Label: "PSP"},
	"pc_engine":            {Label: "TG-16"},
	"pc_fx":                {Label: "PC-FX"},
	"neo_geo_pocket":       {Label: "NGP"},
	"neo_geo_pocket.ngc":   {Label: "NGPC"},
	"neogeo":               {Label: "Neo Geo"},
	"neo_geo_cd":           {Label: "Neo Geo CD"},
	"wonderswan":           {Label: "WS"},
	"wonderswan.wsc":       {Label: "WSC"},
	"atari_2600":           {Label: "2600"},
	"atari_5200":           {Label: "5200"},
	"atari_7800":           {Label: "7800"},
	"atari_lynx":           {Label: "Lynx"},
	"atari_jaguar":         {Label: "Jaguar"},
	"3do":                  {Label: "3DO"},
	"colecovision":         {Label: "Coleco"},
	"intellivision":        {Label: "Intellivision"},
	"vectrex":              {Label: "Vectrex"},
	"mame":                 {Label: "Arcade"},
	"hbmame":               {Label: "Arcade"},
	"fb_alpha":             {Label: "Arcade"},
	"msx":                  {Label: "MSX"},
	"pc_98":                {Label: "PC-98"},
	"dos":                  {Label: "DOS"},
	"scummvm":              {Label: "ScummVM"},
	"commodore_c64":        {Label: "C64"},
	"commodore_amiga":      {Label: "Amiga"},
	"zx_spectrum":          {Label: "ZX"},
	"xbox":                 {Label: "Xbox"},
	"wiiu":                 {Label: "Wii U"},
}

// DefaultSystems returns the built-in table.
func DefaultSystems() Systems {
	return Systems{byKey: defaultSystems}
}

// WithOverrides returns a copy of s where each override replaces the fields it
// sets. An empty field keeps the existing value.
func (s Systems) WithOverrides(overrides map[string]System) Systems {
	merged := make(map[string]System, len(s.byKey)+len(overrides))
	for key, sys := range s.byKey {
		merged[key] = sys
	}
	for key, override := range overrides {
		sys := merged[key]
		if override.Label != "" {
			sys.Label = override.Label
		}
		if override.Image != "" {
			sys.Image = override.Image
		}
		merged[key] = sys
	}
	return Systems{byKey: merged}
}

// Lookup returns the display for a system id running content with the given
// extension. The extension refinement is tried first, and each field it leaves
// empty falls back to the plain id. known is false when neither key exists, in
// which case the label is the id in upper case.
func (s Systems) Lookup(systemID, ext string) (sys System, known bool) {
	refined, refinedKnown := System{}, false
	if ext != "" {
		refined, refinedKnown = s.byKey[systemID+"."+strings.ToLower(ext)]
	}
	base, baseKnown := s.byKey[systemID]

	sys = System{Label: firstNonEmpty(refined.Label, base.Label), Image: firstNonEmpty(refined.Image, base.Image)}
	if sys.Label == "" {
		sys.Label = strings.ToUpper(systemID)
	}
	return sys, refinedKnown || baseKnown
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
