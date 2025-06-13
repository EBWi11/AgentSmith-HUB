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

const NewPluginData = `package plugin

import (
	"errors"
	"strings"
)

func Eval(data string) (bool, error) {
	if data == "" {
		return false, errors.New("")
	}

	if strings.HasSuffix(data, "something") {
		return true, nil
	} else {
		return false, nil
	}
}`
const NewInputData = `type: kafka
kafka:
  brokers:
    - 127.0.0.1:9092
  topic: test-topic
  group: test
  
#type: aliyun_sls
#aliyun_sls:
#  endpoint: "cn-beijing.log.aliyuncs.com"
#  access_key_id: "xx"
#  access_key_secret: "xx"
#  project: "xx"
#  logstore: "xx"
#  consumer_group_name: "xx"
#  consumer_name: "xx"
#  cursor_position: "BEGIN_CURSOR"
#  query: "xx"`

const NewOutputData = `name: kafka_output_demo
type: kafka
kafka:
  brokers:
    - "192.168.27.130:9092"
  topic: "kafka_output_demo"`

const NewRulesetData = `<root name="test2" type="DETECTION">
    <rule id="reverse_shell_01" name="测试" author="test">
        <filter field="data_type">_$data_type</filter>
        <checklist condition="a and c and d and e">
            <node id="a" type="REGEX" field="exe">testcases</node>
            <node id="c" type="INCL" field="exe" logic="OR" delimiter="|">abc|edf</node>
            <node id="d" type="EQU" field="sessionid">_$sessionid</node>
        </checklist>
        <append field_name="abc">123</append>
        <del>exe,argv</del>
    </rule>
</root>`

const NewProjectData = `content: |
  INPUT.demo -> OUTPUT.demo`

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
