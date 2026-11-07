package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version = "0.1.0-rc.1"
	Commit  = "unknown"
	Date    = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Modified  bool   `json:"modified,omitempty"`
}

func Current() Info {
	info := Info{
		Version: strings.TrimSpace(Version),
		Commit: strings.TrimSpace(Commit),
		Date: strings.TrimSpace(Date),
		GoVersion: runtime.Version(),
		OS: runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" || info.Commit == "unknown" {
					info.Commit = setting.Value
				}
			case "vcs.time":
				if info.Date == "" || info.Date == "unknown" {
					info.Date = setting.Value
				}
			case "vcs.modified":
				info.Modified = setting.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.Date == "" {
		info.Date = "unknown"
	}
	return info
}
