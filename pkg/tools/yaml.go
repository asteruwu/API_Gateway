package tools

import (
	"os"

	"gopkg.in/yaml.v3"
)

// LoadYAML 从文件读取 YAML 并反序列化到 out 指向的结构体
func LoadYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
