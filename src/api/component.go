package api

import (
	"AgentSmith-HUB/common"
	"fmt"
	"os"
	"path"
)

const (
	RULESET_EXT     = ".xml"
	RULESET_EXT_NEW = ".xml.new"

	PLUGIN_EXT     = ".go"
	PLUGIN_EXT_NEW = ".go.new"

	EXT     = ".yaml"
	EXT_NEW = ".yaml.new"
)

func GetExt(componentType string, new bool) string {
	if componentType == "ruleset" {
		if new {
			return RULESET_EXT_NEW
		} else {
			return RULESET_EXT
		}
	} else if componentType == "plugin" {
		if new {
			return PLUGIN_EXT_NEW
		} else {
			return PLUGIN_EXT
		}
	} else {
		if new {
			return EXT_NEW
		} else {
			return EXT
		}
	}
}

func GetComponentPath(componentType string, id string, new bool) (string, bool) {
	dirPath := path.Join(common.Config.ConfigRoot, componentType)
	filePath := path.Join(dirPath, id+GetExt(componentType, new))

	//check if dir exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return filePath, false
		}
	}

	_, err := os.Stat(filePath)
	exists := !os.IsNotExist(err)

	return filePath, exists
}

func WriteComponentFile(path string, content string) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

func ReadComponent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	} else {
		return string(data), err
	}
}

// only can edit 'new' file
func EditComponent(componentType string, id string, raw string) error {
	p, _ := GetComponentPath(componentType, id, true)
	return WriteComponentFile(p, raw)
}

func DiffComponent(componentType string, id string) (string, string) {
	newPath, _ := GetComponentPath(componentType, id, true)
	oldPath, _ := GetComponentPath(componentType, id, false)

	dataNew, _ := ReadComponent(newPath)
	dataOld, _ := ReadComponent(oldPath)
	return string(dataOld), string(dataNew)
}

func MergeComponent(componentType string, id string) error {
	newPath, exist := GetComponentPath(componentType, id, true)
	if !exist {
		return fmt.Errorf("File does not exist: %s", newPath)
	}
	oldPath, _ := GetComponentPath(componentType, id, false)

	data, err := ReadComponent(newPath)
	if err != nil {
		return fmt.Errorf("read file error: %s %w", newPath, err)
	}

	_ = os.Remove(oldPath)
	return WriteComponentFile(oldPath, data)
}
