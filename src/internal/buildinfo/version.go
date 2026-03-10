package buildinfo

var (
	Version = "dev"
	Commit  = "unknown"
)

func Short() string {
	return Version
}
