package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const defaultConfigFileName = "config.yaml"

var localConfigFileNames = []string{
	"config.debug.yaml",
	"config.release.yaml",
	"config.test.yaml",
	defaultConfigFileName,
}

type DefaultConfigManager struct {
	defaultConfig []byte
}

func NewDefaultConfigManager(defaultConfig []byte) DefaultConfigManager {
	return DefaultConfigManager{defaultConfig: defaultConfig}
}

func (m DefaultConfigManager) Ensure() error {
	if err := normalizeExplicitConfigEnv(); err != nil {
		return err
	}
	if normalizeConfigArgs(os.Args[1:]) {
		return nil
	}
	if os.Getenv("NAV_CONFIG") != "" || LocalConfigExists() {
		return nil
	}

	configPath, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	if _, err = os.Stat(configPath); err == nil {
		if err = m.EnsurePortableConfig(configPath); err != nil {
			return err
		}
		m.SetDefaultConfigEnv(configPath)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	configBytes, err := m.MaterializeDefaultConfig(filepath.Dir(configPath))
	if err != nil {
		return err
	}
	if err = os.WriteFile(configPath, configBytes, 0600); err != nil {
		return err
	}
	m.SetDefaultConfigEnv(configPath)
	return nil
}

func (m DefaultConfigManager) SetDefaultConfigEnv(configPath string) {
	if os.Getenv("NAV_CONFIG") != "" || HasConfigArg(os.Args[1:]) || LocalConfigExists() {
		return
	}
	_ = os.Setenv("NAV_CONFIG", configPath)
}

func LocalConfigExists() bool {
	for _, name := range localConfigFileNames {
		if _, err := os.Stat(name); err == nil {
			return true
		}
	}
	return false
}

func HasConfigArg(args []string) bool {
	for i, arg := range args {
		if arg == "-c" || arg == "--c" {
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--c=") {
			return true
		}
	}
	return false
}

func DefaultConfigPath() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, defaultConfigFileName), nil
}

func ExecutableDir() (string, error) {
	execPath, err := os.Executable()
	if err == nil {
		if resolvedPath, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
			execPath = resolvedPath
		}
		return filepath.Dir(execPath), nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return dir, nil
}

func (m DefaultConfigManager) MaterializeDefaultConfig(baseDir string) ([]byte, error) {
	cfg, err := decodeYAMLNode(m.defaultConfig)
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(baseDir, "data")
	setConfigPath(cfg, []string{"sqlite", "path"}, dataDir)
	setConfigPath(cfg, []string{"local", "oss-path"}, filepath.Join(dataDir, "oss"))
	setConfigPath(cfg, []string{"local", "cache-path"}, filepath.Join(dataDir, "cache.json"))
	setConfigPath(cfg, []string{"local", "ip2geo-path"}, filepath.Join(dataDir, "ip2geo"))
	setConfigPath(cfg, []string{"zap", "director"}, filepath.Join(baseDir, "logback"))
	setConfigPath(cfg, []string{"navmesh", "data-dir"}, filepath.Join(dataDir, "navmesh"))
	ensurePerformanceConfigDefaults(cfg)
	if value := configStringValue(cfg, []string{"jwt", "signing-key"}); value == "" || value == "navmesh" {
		jwtSecret, err := randomConfigSecret(32)
		if err != nil {
			return nil, err
		}
		setConfigString(cfg, []string{"jwt", "signing-key"}, jwtSecret)
	}
	if value := configStringValue(cfg, []string{"navmesh", "device-register-token"}); value == "" || value == "navfirst@2020" {
		registerToken, err := randomConfigSecret(24)
		if err != nil {
			return nil, err
		}
		setConfigString(cfg, []string{"navmesh", "device-register-token"}, registerToken)
	}

	return encodeYAMLNode(cfg)
}

func (m DefaultConfigManager) EnsurePortableConfig(configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	cfg, err := decodeYAMLNode(raw)
	if err != nil {
		return err
	}

	baseDir := filepath.Dir(configPath)
	dataDir := filepath.Join(baseDir, "data")
	changed := false
	changed = upgradeLegacyConfigPath(cfg, []string{"sqlite", "path"}, dataDir) || changed
	changed = upgradeLegacyConfigPath(cfg, []string{"local", "oss-path"}, filepath.Join(dataDir, "oss")) || changed
	changed = upgradeLegacyConfigPath(cfg, []string{"local", "cache-path"}, filepath.Join(dataDir, "cache.json")) || changed
	changed = upgradeLegacyConfigPath(cfg, []string{"local", "ip2geo-path"}, filepath.Join(dataDir, "ip2geo")) || changed
	changed = upgradeLegacyConfigPath(cfg, []string{"zap", "director"}, filepath.Join(baseDir, "logback")) || changed
	changed = upgradeLegacyConfigPath(cfg, []string{"navmesh", "data-dir"}, filepath.Join(dataDir, "navmesh")) || changed
	changed = ensurePerformanceConfigDefaults(cfg) || changed
	if !changed {
		return nil
	}

	out, err := encodeYAMLNode(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0600)
}

func decodeYAMLNode(data []byte) (*yaml.Node, error) {
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	if node.Kind == 0 {
		node.Kind = yaml.DocumentNode
		node.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	return &node, nil
}

func encodeYAMLNode(node *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func setConfigPath(cfg *yaml.Node, keys []string, value string) {
	setConfigString(cfg, keys, filepath.Clean(value))
}

func setConfigString(cfg *yaml.Node, keys []string, value string) {
	setConfigScalar(cfg, keys, "!!str", value)
}

func setConfigScalar(cfg *yaml.Node, keys []string, tag, value string) {
	if len(keys) == 0 {
		return
	}
	current := rootMappingNode(cfg, true)
	for _, key := range keys[:len(keys)-1] {
		next := mappingValueNode(current, key)
		if next == nil || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode}
			setMappingValueNode(current, key, next)
		}
		current = next
	}
	setMappingValueNode(current, keys[len(keys)-1], &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
}

func ensurePerformanceConfigDefaults(cfg *yaml.Node) bool {
	changed := false
	changed = ensureConfigDefault(cfg, []string{"sqlite", "log-mode"}, "!!str", "warn") || changed
	changed = ensureConfigDefault(cfg, []string{"sqlite", "max-idle-conns"}, "!!int", "8") || changed
	changed = ensureConfigDefault(cfg, []string{"sqlite", "max-open-conns"}, "!!int", "8") || changed
	changed = ensureConfigDefault(cfg, []string{"navmesh", "access-log-queue-size"}, "!!int", "8192") || changed
	changed = ensureConfigDefault(cfg, []string{"navmesh", "access-log-batch-size"}, "!!int", "200") || changed
	changed = ensureConfigDefault(cfg, []string{"navmesh", "access-log-flush-interval"}, "!!str", "100ms") || changed
	changed = ensureConfigDefault(cfg, []string{"navmesh", "database-slow-threshold"}, "!!str", "200ms") || changed
	return changed
}

func ensureConfigDefault(cfg *yaml.Node, keys []string, tag, value string) bool {
	if len(keys) == 0 || configStringValue(cfg, keys) != "" {
		return false
	}
	setConfigScalar(cfg, keys, tag, value)
	return true
}

func configStringValue(cfg *yaml.Node, keys []string) string {
	current := rootMappingNode(cfg, false)
	if current == nil {
		return ""
	}
	for _, key := range keys {
		current = mappingValueNode(current, key)
		if current == nil {
			return ""
		}
	}
	return strings.TrimSpace(current.Value)
}

func randomConfigSecret(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func upgradeLegacyConfigPath(cfg *yaml.Node, keys []string, value string) bool {
	current := rootMappingNode(cfg, false)
	if current == nil {
		return false
	}
	for _, key := range keys[:len(keys)-1] {
		next := mappingValueNode(current, key)
		if next == nil || next.Kind != yaml.MappingNode {
			return false
		}
		current = next
	}

	key := keys[len(keys)-1]
	valueNode := mappingValueNode(current, key)
	if valueNode == nil || valueNode.Kind != yaml.ScalarNode || !isLegacyRelativePath(valueNode.Value) {
		return false
	}
	valueNode.Value = filepath.Clean(value)
	valueNode.Tag = "!!str"
	return true
}

func rootMappingNode(node *yaml.Node, create bool) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			if !create {
				return nil
			}
			node.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
		}
		if node.Content[0].Kind != yaml.MappingNode {
			if !create {
				return nil
			}
			node.Content[0] = &yaml.Node{Kind: yaml.MappingNode}
		}
		return node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		return node
	}
	if !create {
		return nil
	}
	node.Kind = yaml.MappingNode
	node.Content = nil
	return node
}

func mappingValueNode(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setMappingValueNode(mapping *yaml.Node, key string, value *yaml.Node) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, scalarStringNode(key), value)
}

func scalarStringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func isLegacyRelativePath(path string) bool {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimSuffix(path, "/")
	switch path {
	case "./data", "data", "./data/oss", "data/oss", "./data/cache.json", "data/cache.json", "./data/ip2geo", "data/ip2geo", "./data/navmesh", "data/navmesh", "logback", "./logback":
		return true
	default:
		return false
	}
}

func normalizeExplicitConfigEnv() error {
	configPath := strings.TrimSpace(os.Getenv("NAV_CONFIG"))
	if configPath == "" || filepath.IsAbs(configPath) {
		return nil
	}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	return os.Setenv("NAV_CONFIG", abs)
}

func normalizeConfigArgs(args []string) bool {
	for i, arg := range args {
		if (arg == "-c" || arg == "--c") && i+1 < len(args) {
			os.Args[i+2] = absPathOrOriginal(args[i+1])
			return true
		}
		if strings.HasPrefix(arg, "-c=") {
			os.Args[i+1] = "-c=" + absPathOrOriginal(strings.TrimPrefix(arg, "-c="))
			return true
		}
		if strings.HasPrefix(arg, "--c=") {
			os.Args[i+1] = "--c=" + absPathOrOriginal(strings.TrimPrefix(arg, "--c="))
			return true
		}
	}
	return false
}

func absPathOrOriginal(path string) string {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
