package services

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxDeployCommandRunes = 4096

func ValidateDeployFields(runtime, install, build, start string) (normRuntime, normalizedInstall, normalizedBuild, normalizedStart, errCode string) {
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	if runtime == "" {
		runtime = "auto"
	}
	if runtime != "auto" && runtime != "bun" {
		return "", "", "", "", "invalid_deploy_runtime"
	}
	for label, command := range map[string]string{"install_cmd": install, "build_cmd": build, "start_cmd": start} {
		command = strings.TrimSpace(command)
		if strings.ContainsRune(command, '\x00') {
			return "", "", "", "", "deploy_command_invalid"
		}
		if len([]rune(command)) > maxDeployCommandRunes {
			return "", "", "", "", "deploy_command_too_long:" + label
		}
	}
	return runtime, strings.TrimSpace(install), strings.TrimSpace(build), strings.TrimSpace(start), ""
}

func ResolveServiceBuildDirectory(worktree, rootDirectory string) (string, error) {
	rootDirectory = strings.TrimSpace(rootDirectory)
	if rootDirectory == "" || rootDirectory == "." {
		return worktree, nil
	}
	clean := filepath.Clean(rootDirectory)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrCode("invalid_root_directory", fmt.Errorf("root directory must stay within the repository"))
	}
	return filepath.Join(worktree, clean), nil
}
