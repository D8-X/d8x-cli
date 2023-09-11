package configs

import (
	"embed"
)

//go:embed embedded/*
var EmbededConfigs embed.FS
