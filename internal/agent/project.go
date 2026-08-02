package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectType 项目类型
type ProjectType string

const (
	ProjectGo         ProjectType = "go"
	ProjectNode       ProjectType = "node"
	ProjectPython     ProjectType = "python"
	ProjectRust       ProjectType = "rust"
	ProjectJava       ProjectType = "java"
	ProjectDotNet     ProjectType = "dotnet"
	ProjectRuby       ProjectType = "ruby"
	ProjectUnknown    ProjectType = "unknown"
)

// ProjectInfo 项目信息
type ProjectInfo struct {
	Type         ProjectType
	Name         string
	RootDir      string
	HasGit       bool
	HasMemory    bool   // TAIJI.md exists
	MemoryPath   string
	MainFiles    []string
	TestCommand  string
	BuildCommand string
	RunCommand   string
}

// DetectProject 检测项目类型和约定
func DetectProject(dir string) *ProjectInfo {
	info := &ProjectInfo{
		Type:    ProjectUnknown,
		RootDir: dir,
	}

	// 检测Git
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		info.HasGit = true
	}

	// 检测TAIJI.md
	memPath := filepath.Join(dir, "TAIJI.md")
	if _, err := os.Stat(memPath); err == nil {
		info.HasMemory = true
		info.MemoryPath = memPath
	}

	// 按优先级检测项目类型
	if info.HasGit {
		// Go
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			info.Type = ProjectGo
			info.Name = extractModuleName(filepath.Join(dir, "go.mod"), "module")
			info.BuildCommand = "go build ./..."
			info.TestCommand = "go test ./..."
			info.RunCommand = "go run ."
			return info
		}

		// Node.js
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			info.Type = ProjectNode
			info.Name = extractModuleName(filepath.Join(dir, "package.json"), "name")
			info.BuildCommand = "npm run build"
			info.TestCommand = "npm test"
			info.RunCommand = "npm start"
			return info
		}

		// Python
		for _, f := range []string{"pyproject.toml", "setup.py", "requirements.txt", "Pipfile"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				info.Type = ProjectPython
				info.Name = filepath.Base(dir)
				info.BuildCommand = "pip install -e ."
				info.TestCommand = "pytest"
				info.RunCommand = "python main.py"
				return info
			}
		}

		// Rust
		if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
			info.Type = ProjectRust
			info.Name = extractModuleName(filepath.Join(dir, "Cargo.toml"), "name")
			info.BuildCommand = "cargo build"
			info.TestCommand = "cargo test"
			info.RunCommand = "cargo run"
			return info
		}

		// Java
		for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				info.Type = ProjectJava
				info.Name = filepath.Base(dir)
				info.BuildCommand = "mvn compile"
				info.TestCommand = "mvn test"
				info.RunCommand = "mvn exec:java"
				return info
			}
		}

		// .NET
		for _, f := range []string{"*.csproj", "*.sln", "*.fsproj"} {
			matches, _ := filepath.Glob(filepath.Join(dir, f))
			if len(matches) > 0 {
				info.Type = ProjectDotNet
				info.Name = filepath.Base(dir)
				info.BuildCommand = "dotnet build"
				info.TestCommand = "dotnet test"
				info.RunCommand = "dotnet run"
				return info
			}
		}

		// Ruby
		if _, err := os.Stat(filepath.Join(dir, "Gemfile")); err == nil {
			info.Type = ProjectRuby
			info.Name = filepath.Base(dir)
			info.BuildCommand = "bundle install"
			info.TestCommand = "bundle exec rspec"
			info.RunCommand = "bundle exec ruby main.rb"
			return info
		}
	}

	info.Name = filepath.Base(dir)
	return info
}

// BuildContextExtension 生成项目上下文扩展（注入system prompt）
func (p *ProjectInfo) BuildContextExtension() string {
	if p.Type == ProjectUnknown {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<project_info>\n")
	sb.WriteString(fmt.Sprintf("项目类型: %s\n", p.Type))
	sb.WriteString(fmt.Sprintf("项目名称: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("Git: %v\n", p.HasGit))

	if p.BuildCommand != "" {
		sb.WriteString(fmt.Sprintf("构建命令: %s\n", p.BuildCommand))
	}
	if p.TestCommand != "" {
		sb.WriteString(fmt.Sprintf("测试命令: %s\n", p.TestCommand))
	}
	if p.RunCommand != "" {
		sb.WriteString(fmt.Sprintf("运行命令: %s\n", p.RunCommand))
	}

	// 项目特定约定
	switch p.Type {
	case ProjectGo:
		sb.WriteString("Go约定: 遵循标准Go项目布局，错误处理用if err != nil，接口小写开头\n")
	case ProjectNode:
		sb.WriteString("Node约定: 使用ESM模块，TypeScript优先，遵循.eslintrc配置\n")
	case ProjectPython:
		sb.WriteString("Python约定: 遵循PEP 8，使用type hints，docstring用Google格式\n")
	case ProjectRust:
		sb.WriteString("Rust约定: 遵循Rust API Guidelines，使用cargo clippy\n")
	}

	sb.WriteString("</project_info>")
	return sb.String()
}

// extractModuleName 从配置文件中提取模块名（简单实现）
func extractModuleName(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(strings.Trim(strings.TrimSpace(parts[1]), `"'`))
			}
			// go.mod format: "module name"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}
