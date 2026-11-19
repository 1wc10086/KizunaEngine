package resource

import "strings"

func pathExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		ch := path[i]
		if ch == '.' {
			if i != len(path)-1 {
				return path[i:]
			}
			return ""
		}
		if ch == '/' {
			break
		}
	}
	return ""
}

func pathFileName(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return path
	}
	return path[index+1:]
}

func pathFileNameWithoutExtension(path string) string {
	name := pathFileName(path)
	extension := pathExtension(name)
	return name[:len(name)-len(extension)]
}

func pathRootLength(path string) int {
	if strings.HasPrefix(path, "/") {
		return 1
	}
	return 0
}

func pathDirectoryName(path string) string {
	if path == "" {
		return ""
	}

	rootLength := pathRootLength(path)
	end := len(path)
	if end <= rootLength {
		return ""
	}

	for end > rootLength {
		end--
		if path[end] == '/' {
			break
		}
	}

	for end > rootLength && path[end-1] == '/' {
		end--
	}

	return path[:end]
}
