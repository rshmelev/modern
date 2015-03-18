package modern

import (
	"net/http"
	"os"
)

/*
	basically it looks like to be only usefule for FileServer to prevent dirs listening
	so i decided to move such single-use-case code to separate file...
*/

type JustFilesFilesystem struct {
	fs http.FileSystem
}

func (fs JustFilesFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return neuteredReaddirFile{f}, nil
}

type neuteredReaddirFile struct {
	http.File
}

func (f neuteredReaddirFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func MakeJustFilesFs(root string) *JustFilesFilesystem {
	fs := &JustFilesFilesystem{http.Dir(root)}
	return fs
}
