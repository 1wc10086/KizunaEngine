package core

import "os"

func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func WriteAll(path string, data []byte) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func AtomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := WriteAll(tmp, data); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func PathExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func ReadFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ListFilesTop(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
