package config

func FromDocument(base Config, document Document) Config {
	cfg := FromTable(base, document.Root)
	if window := tableField(document.Root, "window"); window != nil {
		if document.Has("window.padding_x") {
			cfg.Window.PaddingLeft = cfg.Window.PaddingX
			cfg.Window.PaddingRight = cfg.Window.PaddingX
		}
		if document.Has("window.padding_y") {
			cfg.Window.PaddingTop = cfg.Window.PaddingY
			cfg.Window.PaddingBottom = cfg.Window.PaddingY
		}
		if document.AuthoredVersion >= 2 {
			cfg.Window.PaddingLeft = intField(window, "padding_left", cfg.Window.PaddingLeft)
			cfg.Window.PaddingRight = intField(window, "padding_right", cfg.Window.PaddingRight)
			cfg.Window.PaddingTop = intField(window, "padding_top", cfg.Window.PaddingTop)
			cfg.Window.PaddingBottom = intField(window, "padding_bottom", cfg.Window.PaddingBottom)
			cfg.Window.TextOpacity = numberField(window, "text_opacity", cfg.Window.TextOpacity)
			cfg.Window.BackgroundOpacity = numberField(window, "background_opacity", cfg.Window.BackgroundOpacity)
			cfg.Window.InitialRows = intField(window, "initial_rows", cfg.Window.InitialRows)
			cfg.Window.InitialCols = intField(window, "initial_cols", cfg.Window.InitialCols)
			cfg.Window.Decorations = stringField(window, "decorations", cfg.Window.Decorations)
			cfg.Window.Titlebar = stringField(window, "titlebar", cfg.Window.Titlebar)
		}
	}
	if scrollbar := tableField(document.Root, "scrollbar"); scrollbar != nil {
		if document.AuthoredVersion >= 2 {
			if document.Has("scrollbar.mode") {
				cfg.Scrollbar.Mode = stringField(scrollbar, "mode", cfg.Scrollbar.Mode)
				cfg.Scrollbar.Enabled = cfg.Scrollbar.Mode != "never"
			} else if document.Has("scrollbar.enabled") {
				if cfg.Scrollbar.Enabled {
					cfg.Scrollbar.Mode = "scrolling"
				} else {
					cfg.Scrollbar.Mode = "never"
				}
			}
			cfg.Scrollbar.StableGutter = boolField(scrollbar, "stable_gutter", cfg.Scrollbar.StableGutter)
			cfg.Scrollbar.AnimationFPS = intField(scrollbar, "animation_fps", cfg.Scrollbar.AnimationFPS)
		} else {
			if cfg.Scrollbar.Enabled {
				cfg.Scrollbar.Mode = "scrolling"
			} else {
				cfg.Scrollbar.Mode = "never"
			}
			cfg.Scrollbar.StableGutter = true
			cfg.Scrollbar.AnimationFPS = base.Scrollbar.AnimationFPS
		}
	}
	if document.AuthoredVersion < 2 {
		cfg.TabBar = base.TabBar
	}
	if document.AuthoredVersion < 2 {
		cfg.Window.TextOpacity = base.Window.TextOpacity
		cfg.Window.BackgroundOpacity = base.Window.BackgroundOpacity
		cfg.Render.MaxFPS = base.Render.MaxFPS
		cfg.Window.InitialRows = base.Window.InitialRows
		cfg.Window.InitialCols = base.Window.InitialCols
		cfg.Window.Decorations = base.Window.Decorations
		cfg.Window.Titlebar = base.Window.Titlebar
	}
	if document.AuthoredVersion >= 2 {
		cfg.ColorScheme = stringField(document.Root, "color_scheme", cfg.ColorScheme)
		if background := tableField(document.Root, "background"); background != nil {
			cfg.Background.Layers = backgroundLayerListField(background, "layers", cfg.Background.Layers)
		}
		if font := tableField(document.Root, "font"); font != nil {
			cfg.Font.Descriptors = descriptorListField(font, "descriptors", cfg.Font.Descriptors)
			cfg.Font.Fallback = descriptorListField(font, "fallback", cfg.Font.Fallback)
			cfg.Font.Rules = fontRuleListField(font, "rules", cfg.Font.Rules)
			cfg.Font.Features = integerMapField(font, "features", cfg.Font.Features)
			cfg.Font.LineHeight = numberField(font, "line_height", cfg.Font.LineHeight)
			cfg.Font.CellWidth = numberField(font, "cell_width", cfg.Font.CellWidth)
			cfg.Font.BaselineOffset = numberField(font, "baseline_offset", cfg.Font.BaselineOffset)
			cfg.Font.GlyphOffsetX = numberField(font, "glyph_offset_x", cfg.Font.GlyphOffsetX)
			cfg.Font.GlyphOffsetY = numberField(font, "glyph_offset_y", cfg.Font.GlyphOffsetY)
		}
		decodeV2PlatformConfig(document, &cfg)
		decodeV2GraphicsConfig(document, &cfg)
	}
	if quick := tableField(document.Root, "quick_select"); quick != nil {
		cfg.QuickSelect.Rules = quickSelectRuleListField(quick, "rules", cfg.QuickSelect.Rules)
		cfg.QuickSelect.Compiled, _ = PrepareQuickSelect(cfg.QuickSelect.Rules)
	}
	cfg.LaunchMenu = launchTargetListField(document.Root, "launch_menu", cfg.LaunchMenu)
	return cfg
}
