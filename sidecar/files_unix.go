package sidecar

import "os"

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func renameFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
