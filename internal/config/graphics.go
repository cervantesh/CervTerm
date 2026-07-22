package config

import (
	"fmt"

	"cervterm/internal/termimage"
)

// GraphicsConfig contains default-off, restart-scoped terminal graphics policy.
type GraphicsConfig struct {
	Kitty  KittyGraphicsConfig
	Sixel  SixelGraphicsConfig
	ITerm  ITermGraphicsConfig
	Limits GraphicsLimitsConfig
}

type KittyGraphicsConfig struct{ Enabled bool }
type SixelGraphicsConfig struct{ Enabled bool }
type ITermGraphicsConfig struct{ Enabled bool }

type GraphicsLimitsConfig struct {
	EncodedBytesPerPane   uint64
	DecodedBytesPerPane   uint64
	ImageCountPerPane     uint64
	PlacementCountPerPane uint64
	GPUBytesPerContext    uint64
}

func defaultGraphicsConfig() GraphicsConfig {
	return GraphicsConfig{Limits: GraphicsLimitsConfig{
		EncodedBytesPerPane:   termimage.HardEncodedBytesPerPane,
		DecodedBytesPerPane:   termimage.HardDecodedBytesPerPane,
		ImageCountPerPane:     termimage.HardImagesPerPane,
		PlacementCountPerPane: termimage.HardPlacementsPerPane,
		GPUBytesPerContext:    termimage.HardGPUBytesPerContext,
	}}
}

func decodeV2GraphicsConfig(document Document, cfg *Config) {
	graphics := tableField(document.Root, "graphics")
	if graphics == nil {
		return
	}
	if kitty := tableField(graphics, "kitty"); kitty != nil {
		cfg.Graphics.Kitty.Enabled = boolField(kitty, "enabled", cfg.Graphics.Kitty.Enabled)
	}
	if sixel := tableField(graphics, "sixel"); sixel != nil {
		cfg.Graphics.Sixel.Enabled = boolField(sixel, "enabled", cfg.Graphics.Sixel.Enabled)
	}
	if iterm := tableField(graphics, "iterm"); iterm != nil {
		cfg.Graphics.ITerm.Enabled = boolField(iterm, "enabled", cfg.Graphics.ITerm.Enabled)
	}
	limits := tableField(graphics, "limits")
	if limits == nil {
		return
	}
	cfg.Graphics.Limits.EncodedBytesPerPane = uint64(intField(limits, "encoded_bytes_per_pane", int(cfg.Graphics.Limits.EncodedBytesPerPane)))
	cfg.Graphics.Limits.DecodedBytesPerPane = uint64(intField(limits, "decoded_bytes_per_pane", int(cfg.Graphics.Limits.DecodedBytesPerPane)))
	cfg.Graphics.Limits.ImageCountPerPane = uint64(intField(limits, "image_count_per_pane", int(cfg.Graphics.Limits.ImageCountPerPane)))
	cfg.Graphics.Limits.PlacementCountPerPane = uint64(intField(limits, "placement_count_per_pane", int(cfg.Graphics.Limits.PlacementCountPerPane)))
	cfg.Graphics.Limits.GPUBytesPerContext = uint64(intField(limits, "gpu_bytes_per_context", int(cfg.Graphics.Limits.GPUBytesPerContext)))
}

func (config GraphicsLimitsConfig) validate() error {
	limits := []struct {
		name           string
		value, maximum uint64
	}{
		{"encoded_bytes_per_pane", config.EncodedBytesPerPane, termimage.HardEncodedBytesPerPane},
		{"decoded_bytes_per_pane", config.DecodedBytesPerPane, termimage.HardDecodedBytesPerPane},
		{"image_count_per_pane", config.ImageCountPerPane, termimage.HardImagesPerPane},
		{"placement_count_per_pane", config.PlacementCountPerPane, termimage.HardPlacementsPerPane},
		{"gpu_bytes_per_context", config.GPUBytesPerContext, termimage.HardGPUBytesPerContext},
	}
	for _, limit := range limits {
		if limit.value == 0 || limit.value > limit.maximum {
			return fmt.Errorf("graphics.limits.%s must be between 1 and %d", limit.name, limit.maximum)
		}
	}
	return nil
}
