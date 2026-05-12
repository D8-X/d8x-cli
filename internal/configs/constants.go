package configs

// Some default values for various things
const (
	// User name which will be created on each server when performing
	// provisioning. Or if performing some configuration/deployment/etc this
	// user will be used to ssh into the server.
	DEFAULT_USER_NAME = "d8xtrader"

	// Default ansible hosts.cfg (ini format) file path
	DEFAULT_HOSTS_FILE = "./hosts.cfg"
)
