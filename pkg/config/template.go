package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// templateBody 是用户首次运行时看到的初始配置。
const templateBody = `# MapleCode configuration
# On first run MapleCode generated this file. Please fill in your api_key
# (and adjust protocol, model, base_url as needed) then re-run.

protocol: anthropic
model: claude-sonnet-4-5
base_url: https://api.anthropic.com
api_key: ""

# Optional: enable Anthropic extended thinking. Only valid when protocol=anthropic.
thinking:
  enabled: false
  budget_tokens: 4096

# Optional: override the default system prompt below.
# Default: a coding assistant that is concise and outputs code in markdown blocks.
system_prompt: ""
`

// WriteTemplate 确保父目录存在，然后将默认配置模板写入指定路径。
// 可以安全地多次调用；不会覆盖已存在的文件。
func WriteTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		// File already exists; do not clobber.
		return nil
	}
	if err := os.WriteFile(path, []byte(templateBody), 0o600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
