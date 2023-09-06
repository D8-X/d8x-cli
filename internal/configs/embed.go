package configs

import (
	"embed"
)

//go:generate mockgen -p mocks

//go:embed trader-backend/*
var TraderBackendConfigs embed.FS

//go:embed playbooks/*
var AnsiblePlaybooks embed.FS

//go:embed broker-server/*
var BrokerServerConfigs embed.FS

//go:embed nginx/*
var NginxConfigs embed.FS
