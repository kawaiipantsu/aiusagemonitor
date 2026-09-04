package theme

// Built-in themes. Register in a stable order; Names() sorts for display.
func init() {
	register(Theme{
		Name: "midnight", Display: "Midnight", Dark: true,
		Bg: "#0b0f1a", Surface: "#131a2b", Overlay: "#1d2740",
		Text: "#e6ecff", Muted: "#93a1c9", Subtle: "#5b6790", Border: "#2a3552",
		Primary: "#6ea8fe", Secondary: "#b78dff", Accent: "#37d3c3",
		Success: "#4ade80", Warning: "#fbbf24", Danger: "#f87171",
		Gradient: []string{"#1e3a8a", "#2563eb", "#38bdf8", "#5eead4", "#a7f3d0"},
		Providers: map[string]string{
			"openai": "#10a37f", "anthropic": "#d97757", "google": "#4285f4", "xai": "#e6ecff",
		},
	})
	register(Theme{
		Name: "dracula", Display: "Dracula", Dark: true,
		Bg: "#282a36", Surface: "#2f3242", Overlay: "#3b3f52",
		Text: "#f8f8f2", Muted: "#bfc7d5", Subtle: "#6272a4", Border: "#44475a",
		Primary: "#bd93f9", Secondary: "#ff79c6", Accent: "#8be9fd",
		Success: "#50fa7b", Warning: "#f1fa8c", Danger: "#ff5555",
		Gradient: []string{"#44475a", "#6272a4", "#8be9fd", "#50fa7b", "#f1fa8c"},
		Providers: map[string]string{
			"openai": "#50fa7b", "anthropic": "#ffb86c", "google": "#8be9fd", "xai": "#f8f8f2",
		},
	})
	register(Theme{
		Name: "nord", Display: "Nord", Dark: true,
		Bg: "#2e3440", Surface: "#3b4252", Overlay: "#434c5e",
		Text: "#eceff4", Muted: "#d8dee9", Subtle: "#7b88a1", Border: "#4c566a",
		Primary: "#88c0d0", Secondary: "#b48ead", Accent: "#8fbcbb",
		Success: "#a3be8c", Warning: "#ebcb8b", Danger: "#bf616a",
		Gradient: []string{"#4c566a", "#5e81ac", "#81a1c1", "#88c0d0", "#8fbcbb"},
		Providers: map[string]string{
			"openai": "#a3be8c", "anthropic": "#d08770", "google": "#5e81ac", "xai": "#eceff4",
		},
	})
	register(Theme{
		Name: "gruvbox", Display: "Gruvbox", Dark: true,
		Bg: "#1d2021", Surface: "#282828", Overlay: "#3c3836",
		Text: "#ebdbb2", Muted: "#bdae93", Subtle: "#928374", Border: "#504945",
		Primary: "#83a598", Secondary: "#d3869b", Accent: "#8ec07c",
		Success: "#b8bb26", Warning: "#fabd2f", Danger: "#fb4934",
		Gradient: []string{"#504945", "#665c54", "#83a598", "#8ec07c", "#b8bb26"},
		Providers: map[string]string{
			"openai": "#b8bb26", "anthropic": "#fe8019", "google": "#83a598", "xai": "#ebdbb2",
		},
	})
	register(Theme{
		Name: "tokyonight", Display: "Tokyo Night", Dark: true,
		Bg: "#1a1b26", Surface: "#1f2335", Overlay: "#292e42",
		Text: "#c0caf5", Muted: "#9aa5ce", Subtle: "#565f89", Border: "#3b4261",
		Primary: "#7aa2f7", Secondary: "#bb9af7", Accent: "#2ac3de",
		Success: "#9ece6a", Warning: "#e0af68", Danger: "#f7768e",
		Gradient: []string{"#3b4261", "#565f89", "#7aa2f7", "#2ac3de", "#9ece6a"},
		Providers: map[string]string{
			"openai": "#9ece6a", "anthropic": "#ff9e64", "google": "#7aa2f7", "xai": "#c0caf5",
		},
	})
	register(Theme{
		Name: "solarized", Display: "Solarized Dark", Dark: true,
		Bg: "#002b36", Surface: "#073642", Overlay: "#0b4553",
		Text: "#eee8d5", Muted: "#93a1a1", Subtle: "#586e75", Border: "#0b4a5a",
		Primary: "#268bd2", Secondary: "#6c71c4", Accent: "#2aa198",
		Success: "#859900", Warning: "#b58900", Danger: "#dc322f",
		Gradient: []string{"#073642", "#586e75", "#268bd2", "#2aa198", "#859900"},
		Providers: map[string]string{
			"openai": "#859900", "anthropic": "#cb4b16", "google": "#268bd2", "xai": "#eee8d5",
		},
	})
	register(Theme{
		Name: "matrix", Display: "Matrix", Dark: true,
		Bg: "#000000", Surface: "#001a00", Overlay: "#003300",
		Text: "#c8ffc8", Muted: "#4fdd4f", Subtle: "#1f7a1f", Border: "#0a4a0a",
		Primary: "#00ff41", Secondary: "#7dff9a", Accent: "#39ff14",
		Success: "#00ff41", Warning: "#d7ff00", Danger: "#ff3860",
		Gradient: []string{"#003b00", "#006400", "#00a300", "#00ff41", "#9dff9d"},
		Providers: map[string]string{
			"openai": "#00ff41", "anthropic": "#7dff9a", "google": "#39ff14", "xai": "#c8ffc8",
		},
	})
	register(Theme{
		Name: "ember", Display: "Ember", Dark: true,
		Bg: "#170f0b", Surface: "#241611", Overlay: "#33201a",
		Text: "#ffece0", Muted: "#e0b39a", Subtle: "#a5715a", Border: "#4a2b20",
		Primary: "#ff8c42", Secondary: "#ffb347", Accent: "#ff5e5b",
		Success: "#9bcf53", Warning: "#ffd166", Danger: "#ef476f",
		Gradient: []string{"#4a2b20", "#8a3b12", "#d9531e", "#ff8c42", "#ffd166"},
		Providers: map[string]string{
			"openai": "#9bcf53", "anthropic": "#ff8c42", "google": "#ffd166", "xai": "#ffece0",
		},
	})
	register(Theme{
		Name: "paper", Display: "Paper (light)", Dark: false,
		Bg: "#fbfbf7", Surface: "#ffffff", Overlay: "#eef0f4",
		Text: "#1f2430", Muted: "#55607a", Subtle: "#8a93a8", Border: "#d7dbe4",
		Primary: "#2563eb", Secondary: "#7c3aed", Accent: "#0891b2",
		Success: "#15803d", Warning: "#b45309", Danger: "#b91c1c",
		Gradient: []string{"#dbeafe", "#93c5fd", "#3b82f6", "#1d4ed8", "#1e3a8a"},
		Providers: map[string]string{
			"openai": "#10a37f", "anthropic": "#d97757", "google": "#4285f4", "xai": "#111111",
		},
	})
	register(Theme{
		Name: "contrast", Display: "High Contrast", Dark: true,
		Bg: "#000000", Surface: "#0a0a0a", Overlay: "#1a1a1a",
		Text: "#ffffff", Muted: "#d0d0d0", Subtle: "#9a9a9a", Border: "#5a5a5a",
		Primary: "#00e0ff", Secondary: "#ff00ff", Accent: "#ffff00",
		Success: "#00ff00", Warning: "#ffaa00", Danger: "#ff0033",
		Gradient: []string{"#333333", "#0066ff", "#00e0ff", "#00ff88", "#ffff00"},
		Providers: map[string]string{
			"openai": "#00ff88", "anthropic": "#ffaa00", "google": "#00e0ff", "xai": "#ffffff",
		},
	})
}
