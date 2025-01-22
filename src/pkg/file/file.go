package file

import (
	"encoding/json"
	"os"
	"path"
)

func StoreJSON(name string, tmpExt string, flag int, perm os.FileMode, data interface{}) error {
	fdir, _ := path.Split(name)
	if _, err := os.Stat(fdir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err = os.MkdirAll(fdir, 0755)
		if err != nil {
			return err
		}
	}
	err := storeJSON(name, tmpExt, flag, perm, data)
	if err != nil {
		os.Remove(name + tmpExt)
		return err
	}
	return os.Rename(name+tmpExt, name)
}

func storeJSON(name string, tmpExt string, flag int, perm os.FileMode, data interface{}) error {
	f, err := os.OpenFile(name+tmpExt, flag, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}
