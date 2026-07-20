//go:build windows

package layoutstate

import (
	"os"

	"golang.org/x/sys/windows"
)

func fileHasMultipleLinks(path string, _ os.FileInfo) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &data); err != nil {
		return false, err
	}
	return data.NumberOfLinks > 1, nil
}
