package configs

// Some default values for various things
const (
	// Default file name for main d8x config in default config directory
	DEFAULT_D8X_CONFIG_NAME = "d8x.conf.json"

	// User name which will be created on each server when performing
	// provisioning. Or if performing some configuration/deployment/etc this
	// user will be used to ssh into the server.
	DEFAULT_USER_NAME = "d8xtrader"

	// File where password for DEFAULT_USER_NAME will be stored
	DEFAULT_PASSWORD_FILE = "./password.txt"

	// Default ansible hosts.cfg (ini format) file path
	DEFAULT_HOSTS_FILE = "./hosts.cfg"
)
