package webhooks

import (
	"goklipper/common/logger"
	"goklipper/common/utils/file"
	"os"
)

// RemoveSocketFile removes the Unix domain socket file at filePath,
// tolerating the case where the file does not exist.
func RemoveSocketFile(filePath string) error {
	err := os.Remove(filePath)
	if err != nil {
		exist, err1 := file.PathExists(filePath)
		if err1 != nil {
			logger.Error(err1.Error())
		} else if exist {
			logger.Error("webhooks: Unable to delete socket file ", filePath)
			return err
		}
	}
	return nil
}
