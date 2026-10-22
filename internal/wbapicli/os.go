package wbapicli

import "os"

func osIsNotExist(err error) bool {
	return os.IsNotExist(err)
}
