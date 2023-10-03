package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v2"
)

var defaultHomeDir = AppDataDir("brbot", false)

type config struct {
	DataDir string

	URL            string
	ServerCertPath string
	ClientCertPath string
	ClientKeyPath  string

	MatrixUser  string
	MatrixPass  string
	MatrixToken string
	MatrixProxy string

	// TODO - add network support
	Bridges [][2]string
	bridges map[string]string
}

func loadConfig() (*config, error) {
	const funcName = "loadConfig"

	configPath := filepath.Join(defaultHomeDir, "brbot.conf")
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		str := "%s: %w"
		err := fmt.Errorf(str, funcName, err)
		return nil, err
	}
	cfg := config{
		DataDir: defaultHomeDir,
	}
	if err = yaml.UnmarshalStrict(configFile, &cfg); err != nil {
		str := "%s: failed to parse config file: %w"
		err := fmt.Errorf(str, funcName, err)
		return nil, err
	}

	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.ServerCertPath = cleanAndExpandPath(cfg.ServerCertPath)
	cfg.ClientCertPath = cleanAndExpandPath(cfg.ClientCertPath)
	cfg.ClientKeyPath = cleanAndExpandPath(cfg.ClientKeyPath)

	if cfg.DataDir == "" {
		cfg.DataDir = defaultHomeDir
	}
	if cfg.MatrixPass == "" {
		str := "%s: matrix password is required"
		err = fmt.Errorf(str, funcName)
		return nil, err
	}
	if cfg.MatrixUser == "" {
		str := "%s: matrix user is required"
		err = fmt.Errorf(str, funcName)
		return nil, err
	}
	if !strings.HasSuffix(cfg.MatrixUser, ":decred.org") || cfg.MatrixUser[0] != '@' {
		str := "%s: matrix username invalid: must be in the form @username:decred.org"
		err = fmt.Errorf(str, funcName)
		return nil, err
	}
	if cfg.MatrixToken == "" {
		str := "%s: matrix token is required"
		err = fmt.Errorf(str, funcName)
		return nil, err
	}

	if len(cfg.Bridges) == 0 {
		str := "%s: no bridges defined"
		err = fmt.Errorf(str, funcName)
		return nil, err
	}

	bridges := make(map[string]string)
	for _, bridge := range cfg.Bridges {
		if len(bridge) != 2 {
			str := "%s: invalid bridge specified"
			err = fmt.Errorf(str, funcName)
			return nil, err
		}
		for _, room := range bridge {
			if _, exists := bridges[room]; exists {
				str := "%s: room %v is already bridged elsewhere"
				err = fmt.Errorf(str, funcName, room)
				return nil, err
			}
		}
		bridges[bridge[0]] = bridge[1]
		bridges[bridge[1]] = bridge[0]
	}
	cfg.bridges = bridges
	return &cfg, nil
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Nothing to do when no path is given.
	if path == "" {
		return path
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but the variables can still be expanded via POSIX-style
	// $VARIABLE.
	path = os.ExpandEnv(path)

	if !strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	// Expand initial ~ to the current user's home directory, or ~otheruser
	// to otheruser's home directory.  On Windows, both forward and backward
	// slashes can be used.
	path = path[1:]

	var pathSeparators string
	if runtime.GOOS == "windows" {
		pathSeparators = string(os.PathSeparator) + "/"
	} else {
		pathSeparators = string(os.PathSeparator)
	}

	userName := ""
	if i := strings.IndexAny(path, pathSeparators); i != -1 {
		userName = path[:i]
		path = path[i:]
	}

	homeDir := ""
	var u *user.User
	var err error
	if userName == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(userName)
	}
	if err == nil {
		homeDir = u.HomeDir
	}
	// Fallback to CWD if user lookup fails or user has no home directory.
	if homeDir == "" {
		homeDir = "."
	}

	return filepath.Join(homeDir, path)
}
