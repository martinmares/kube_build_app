package appinfo

const (
	BuildAppName = "kube-build-app"
	EditAppName  = "kube-edit-app"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func For(name string) Info {
	return Info{
		Name:    name,
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}
