package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	Version   string
	GitCommit string
	BuildDate string
	GoVersion string
}

func Current() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}

func (i Info) String() string {
	return fmt.Sprintf("version: %s\ngit_commit: %s\nbuild_date: %s\ngo_version: %s", i.Version, i.GitCommit, i.BuildDate, i.GoVersion)
}
