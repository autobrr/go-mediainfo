package mediainfo_test

import (
	"fmt"

	mediainfo "github.com/autobrr/go-mediainfo"
)

func ExampleAnalyzeFile() {
	report, err := mediainfo.AnalyzeFile("samples/sample.mp4")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(report.General.Kind)
	// Output: General
}
