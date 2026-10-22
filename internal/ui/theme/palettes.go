package theme

// The built-in palettes, and the only file in internal/ui that may hold hex
// colour values. Each fills the 24 Base24 slots; slot meanings are in
// palette.go.

import "image/color"

var builtinDark = Palette{
	Name:    "strig-dark",
	Variant: Dark,
	colors: [numSlots]color.Color{
		Base00: hex("#1a1a22"),
		Base01: hex("#2b2b36"),
		Base02: hex("#33364d"),
		Base03: hex("#8b8b9b"),
		Base04: hex("#a8a8b8"),
		Base05: hex("#c9c9d6"),
		Base06: hex("#e0e0ea"),
		Base07: hex("#f4f4fa"),
		Base08: hex("#ff8787"),
		Base09: hex("#ffc078"),
		Base0A: hex("#ffd43b"),
		Base0B: hex("#8ce99a"),
		Base0C: hex("#66d9e8"),
		Base0D: hex("#74c0fc"),
		Base0E: hex("#b197fc"),
		Base0F: hex("#c08552"),
		Base10: hex("#14141a"),
		Base11: hex("#0e0e12"),
		Base12: hex("#ffa8a8"),
		Base13: hex("#ffe066"),
		Base14: hex("#b2f2bb"),
		Base15: hex("#99e9f2"),
		Base16: hex("#a5d8ff"),
		Base17: hex("#da77f2"),
	},
}

var builtinLight = Palette{
	Name:    "strig-light",
	Variant: Light,
	colors: [numSlots]color.Color{
		Base00: hex("#ffffff"),
		Base01: hex("#e4e4ea"),
		Base02: hex("#dde1f5"),
		Base03: hex("#6b6b7b"),
		Base04: hex("#55556a"),
		Base05: hex("#3c3c4a"),
		Base06: hex("#2a2a36"),
		Base07: hex("#14141c"),
		Base08: hex("#c92a2a"),
		Base09: hex("#b8590a"),
		Base0A: hex("#8a5a00"),
		Base0B: hex("#1f7a3d"),
		Base0C: hex("#0a7f8f"),
		Base0D: hex("#0b6e99"),
		Base0E: hex("#5f3dc4"),
		Base0F: hex("#8a5a2b"),
		Base10: hex("#f1f1f5"),
		Base11: hex("#e9e9ef"),
		Base12: hex("#e03131"),
		Base13: hex("#a86b00"),
		Base14: hex("#2b8a3e"),
		Base15: hex("#0c8599"),
		Base16: hex("#1971c2"),
		Base17: hex("#a626a4"),
	},
}
