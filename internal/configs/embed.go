package configs

import "embed"

//go:embed trader-backend/*
var TraderBackendConfigs embed.FS
