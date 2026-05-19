package configs

import (
	"os"
	"path/filepath"
)

const DEFAULT_USER_NAME = "d8xtrader"

var DEFAULT_HOSTS_FILE = filepath.Join(os.TempDir(), "d8x-cli", "hosts.cfg")
