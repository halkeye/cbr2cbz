package main

import (
	"time"

	"github.com/carlmjohnson/versioninfo"
	"github.com/halkeye/cbr2cbz/cmd"
)

func main() {
	cmd.SetVersionInfo(versioninfo.Version, versioninfo.Revision, versioninfo.LastCommit.Format(time.RFC3339))
	cmd.Execute()
}
