package theme

// This is the built-in palette: the seed for the Base24 palette in #44, and
// the only file in this package allowed to hold hex colour values. Each
// constant is one light-background or dark-background value; identical values
// share a constant. Styles pick a pair with c(xLight, xDark).
const (
	// Status bar surface and text.
	barBgLight = "#e4e4ea"
	barBgDark  = "#2b2b36"
	barFgLight = "#3c3c4a"
	barFgDark  = "#c9c9d6"

	// Text on an accent-coloured background.
	onAccentLight = "#ffffff"
	onAccentDark  = "#1a1a22"

	// De-emphasised text.
	mutedLight = "#6b6b7b"
	mutedDark  = "#8b8b9b"

	// Accent (violet).
	violetLight    = "#5f3dc4"
	violetDark     = "#b197fc"
	violetPaleDark = "#d0bfff"

	// Warm (orange).
	orangeLight      = "#b8590a"
	orangeDark       = "#ffc078"
	orangeStrongDark = "#ffa94d"

	// Success (green).
	greenLight = "#1f7a3d"
	greenDark  = "#8ce99a"

	// Failure (red).
	redLight = "#c92a2a"
	redDark  = "#ff8787"

	// Blue: hunk headers and function names.
	blueLight     = "#0b6e99"
	blueDark      = "#74c0fc"
	blueDeepLight = "#1c62b9"

	// Diff row backgrounds.
	addBgLight          = "#e6ffec"
	addBgDark           = "#12261e"
	addGutterBgLight    = "#ccffd8"
	addGutterBgDark     = "#17382a"
	deleteBgLight       = "#ffebe9"
	deleteBgDark        = "#2d1517"
	deleteGutterBgLight = "#ffd7d5"
	deleteGutterBgDark  = "#42191d"
	cursorBgLight       = "#dde1f5"
	cursorBgDark        = "#33364d"
	hunkBgLight         = "#ddf4ff"
	hunkBgDark          = "#1b2a3a"
	hunkCursorBgLight   = "#c5e4f5"
	hunkCursorBgDark    = "#2c4560"

	// Syntax tokens (foregrounds).
	purpleLight    = "#a626a4" // keywords
	purpleDark     = "#da77f2"
	greenDeepLight = "#2f7d32" // strings
	limeDark       = "#a9e34b"
	tealLight      = "#0a7f8f" // types
	tealDark       = "#66d9e8"
	goldLight      = "#8a5a00" // operators
	goldDark       = "#ffd43b"
)
