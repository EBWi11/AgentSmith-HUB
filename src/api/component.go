package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"fmt"
	"os"
	"path"
	"strings"
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
	// 检查是否是临时文件（.new）
	if path[len(path)-4:] == ".new" {
		// 获取组件类型和ID
		dir := path[:len(path)-4]
		var componentType, id string

		// 根据路径确定组件类型
		if dir[len(dir)-4:] == ".xml" {
			componentType = "ruleset"
			id = path[len(path)-8-len(id) : len(path)-8]
		} else if dir[len(dir)-3:] == ".go" {
			componentType = "plugin"
			id = path[len(path)-7-len(id) : len(path)-7]
		} else if dir[len(dir)-5:] == ".yaml" {
			// 从路径中提取组件类型
			parts := strings.Split(dir, "/")
			if len(parts) >= 2 {
				componentType = parts[len(parts)-2]
				id = parts[len(parts)-1][:len(parts[len(parts)-1])-5]
			}
		}

		// 如果是临时文件，同时更新内存中的副本
		switch componentType {
		case "input":
			if project.GlobalProject.InputsNew == nil {
				project.GlobalProject.InputsNew = make(map[string]string)
			}
			project.GlobalProject.InputsNew[id] = content
		case "output":
			if project.GlobalProject.OutputsNew == nil {
				project.GlobalProject.OutputsNew = make(map[string]string)
			}
			project.GlobalProject.OutputsNew[id] = content
		case "ruleset":
			if project.GlobalProject.RulesetsNew == nil {
				project.GlobalProject.RulesetsNew = make(map[string]string)
			}
			project.GlobalProject.RulesetsNew[id] = content
		case "project":
			if project.GlobalProject.ProjectsNew == nil {
				project.GlobalProject.ProjectsNew = make(map[string]string)
			}
			project.GlobalProject.ProjectsNew[id] = content
		case "plugin":
			if plugin.PluginsNew == nil {
				plugin.PluginsNew = make(map[string]string)
			}
			plugin.PluginsNew[id] = content
		}
	}

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
