package version

var version string

func Get() string {
	if version == "" {
		return "dev"
	}
	return version
}
