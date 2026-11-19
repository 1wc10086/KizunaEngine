package disc

// media.go 把“用户选中的目录/文件”识别成一组光盘镜像，并给游戏库提供
// 不落盘的虚拟文件列表。
//
// 为什么要区分“识别”和“解包”：游戏库扫描只想知道这是哪部作品；如果每扫一次
// 都把 1.5GB 的镜像解到缓存，打开应用会卡几分钟。List 只读 ISO 目录记录，
// 找到 bss.dat / bsxx.dat 之类的锚点即可；真正进入游戏时 Prepare 才流式解包。

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var imageExtensions = map[string]bool{
	".iso": true,
	".bin": true,
	".img": true,
	".mdf": true,
	".nrg": true,
}

var descriptorExtensions = map[string]bool{
	".cue": true,
	".ccd": true,
	".mds": true,
}

// IsContainer 判断路径是不是光盘镜像，或包含光盘镜像的目录。
// 对裸 .bin/.img 会做一次 LBA16 的 CD001 探测，避免把普通数据文件误认成光盘。
func IsContainer(root string) bool {
	if root == "" {
		return false
	}
	info, err := os.Stat(root)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return isImageFile(root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if descriptorExtensions[ext] {
			return true
		}
		if imageExtensions[ext] && hasISOMagic(filepath.Join(root, name)) {
			return true
		}
	}
	return false
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if descriptorExtensions[ext] {
		return true
	}
	return imageExtensions[ext] && hasISOMagic(path)
}

func hasISOMagic(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	size := info.Size()
	for _, layout := range sectorLayouts {
		offset := int64(16*layout.size + layout.userOffset)
		if offset+2048 > size {
			continue
		}
		magic := make([]byte, 6)
		if _, err := file.ReadAt(magic, offset); err == nil && string(magic[1:6]) == isoMagic {
			return true
		}
	}
	return false
}

// containers 返回去重后的镜像数据文件。描述符优先于裸数据文件：
// Disk1.Alpha.img 同时被 .cue/.ccd 引用时，只保留一份。
func containers(root string) []string {
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(root))
		if descriptorExtensions[ext] {
			if data := descriptorDataFile(root); data != "" {
				if absolute, err := filepath.Abs(data); err == nil {
					return []string{absolute}
				}
				return []string{data}
			}
			return nil
		}
		if imageExtensions[ext] {
			return []string{root}
		}
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var descriptors, bare []string
	referenced := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !descriptorExtensions[ext] && !imageExtensions[ext] {
			continue
		}
		if descriptorExtensions[ext] {
			descriptors = append(descriptors, path)
			if data := descriptorDataFile(path); data != "" {
				if absolute, err := filepath.Abs(data); err == nil {
					referenced[strings.ToLower(absolute)] = true
				}
			}
			continue
		}
		if hasISOMagic(path) {
			bare = append(bare, path)
		}
	}
	sort.Strings(descriptors)
	sort.Strings(bare)
	result := make([]string, 0, len(descriptors)+len(bare))
	seen := map[string]bool{}
	// .cue 比 .ccd 更通用地描述 MODE1/2048 与数据轨起点，优先。
	sort.SliceStable(descriptors, func(i, j int) bool {
		rank := func(path string) int {
			switch strings.ToLower(filepath.Ext(path)) {
			case ".cue":
				return 0
			case ".ccd":
				return 1
			default:
				return 2
			}
		}
		return rank(descriptors[i]) < rank(descriptors[j])
	})
	for _, descriptor := range descriptors {
		data := descriptorDataFile(descriptor)
		if data == "" {
			continue
		}
		absolute, err := filepath.Abs(data)
		if err != nil {
			absolute = data
		}
		key := strings.ToLower(absolute)
		if seen[key] {
			continue
		}
		if _, err := os.Stat(absolute); err != nil {
			continue
		}
		seen[key] = true
		result = append(result, absolute)
	}
	for _, path := range bare {
		absolute, err := filepath.Abs(path)
		if err != nil {
			absolute = path
		}
		key := strings.ToLower(absolute)
		if seen[key] || referenced[key] {
			continue
		}
		seen[key] = true
		result = append(result, absolute)
	}
	return result
}

func descriptorDataFile(descriptor string) string {
	switch strings.ToLower(filepath.Ext(descriptor)) {
	case ".cue":
		sheet, _, ok := parseCue(descriptor)
		if !ok {
			return ""
		}
		return sheet.dataFile
	case ".ccd":
		directory := filepath.Dir(descriptor)
		stem := strings.TrimSuffix(filepath.Base(descriptor), filepath.Ext(descriptor))
		for _, ext := range []string{".img", ".bin", ".iso", ".mdf"} {
			if candidates := foldCandidates(directory, stem+ext); len(candidates) > 0 {
				return candidates[0]
			}
		}
		return ""
	case ".mds":
		directory := filepath.Dir(descriptor)
		stem := strings.TrimSuffix(filepath.Base(descriptor), filepath.Ext(descriptor))
		if candidates := foldCandidates(directory, stem+".mdf"); len(candidates) > 0 {
			return candidates[0]
		}
		return ""
	}
	return ""
}

type imageSet struct {
	images []*sectorImage
}

func openImageSet(root string) (*imageSet, error) {
	paths := containers(root)
	if len(paths) == 0 {
		return nil, errors.New("disc: no image found")
	}
	set := &imageSet{}
	for _, path := range paths {
		image, err := openSectorImage(path)
		if err != nil {
			set.Close()
			return nil, err
		}
		set.images = append(set.images, image)
	}
	return set, nil
}

func (s *imageSet) Close() {
	for _, image := range s.images {
		_ = image.Close()
	}
	s.images = nil
}

// List 只读目录记录，返回所有盘合并后的文件列表。多张盘按文件名排序，
// 同名路径先出现者优先（Bishop 的 Disc1/Disc2/Disc3 数据目录相同，合并后
// Graphics2.Inx 与 Graphics2.Snn 才会落在同一逻辑目录里）。
func List(root string) ([]File, bool, error) {
	if !IsContainer(root) {
		return nil, false, nil
	}
	set, err := openImageSet(root)
	if err != nil {
		return nil, true, err
	}
	defer set.Close()
	files := map[string]File{}
	for _, image := range set.images {
		volume, err := openISO(image)
		if err != nil {
			return nil, true, err
		}
		for _, file := range append(volume.Files(), volume.installShieldFiles()...) {
			key := strings.ToLower(file.Path)
			if _, exists := files[key]; !exists {
				files[key] = file
			}
		}
	}
	out := make([]File, 0, len(files))
	for _, file := range files {
		out = append(out, file)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, true, nil
}

// Prepare 把镜像流式解包到缓存目录并返回真实目录。解包是按盘合并的：
// 三张盘的 iso/program files/BISHOP/TB 会落在同一个目录，资源索引因此能
// 一次看到 bss.dat、Graphics1/2、Voice、scene.dat 等全部文件。
func Prepare(root, cacheRoot string) (string, bool, error) {
	if !IsContainer(root) {
		return root, false, nil
	}
	paths := containers(root)
	if len(paths) == 0 {
		return root, false, errors.New("disc: no image found")
	}
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = filepath.Join(os.TempDir(), "kizuna-disc")
	}
	mount := filepath.Join(cacheRoot, "disc", mountKey(paths))
	if ready, err := isMountReady(mount); err == nil && ready {
		return mount, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(mount), 0o755); err != nil {
		return root, true, err
	}
	working, err := os.MkdirTemp(filepath.Dir(mount), filepath.Base(mount)+".tmp-")
	if err != nil {
		return root, true, err
	}
	defer os.RemoveAll(working)
	for _, path := range paths {
		if err := extractImage(path, working); err != nil {
			return root, true, fmt.Errorf("disc: extract %s: %w", filepath.Base(path), err)
		}
	}
	unpackInstallShield(working)
	exportDiscAudio(working, paths)
	if err := writeMountManifest(working, paths); err != nil {
		return root, true, err
	}
	// rename 是原子的；并发打开同一镜像时后到者直接复用先到者的结果。
	if err := os.Rename(working, mount); err != nil {
		if ready, checkErr := isMountReady(mount); checkErr == nil && ready {
			return mount, true, nil
		}
		return root, true, err
	}
	return mount, true, nil
}

func extractImage(path, destination string) error {
	image, err := openSectorImage(path)
	if err != nil {
		return err
	}
	defer image.Close()
	volume, err := openISO(image)
	if err != nil {
		return err
	}
	for _, file := range volume.Files() {
		relative, ok := sanitizeRelative(file.Path)
		if !ok {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if info, err := os.Stat(target); err == nil && info.Size() == file.Size {
			// 多盘合并时同名同大小文件直接跳过，避免重复写 GB 级资源。
			continue
		}
		if info, err := os.Stat(target); err == nil && info.Size() != file.Size {
			// 真正的同名冲突极罕见；保留先到的盘，日志里能看出是哪张盘。
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source := volume.files[file.Path]
		if source == nil {
			continue
		}
		writer, err := os.Create(target)
		if err != nil {
			return err
		}
		streamErr := volume.streamFile(source, writer)
		closeErr := writer.Close()
		if streamErr != nil {
			return streamErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func mountKey(paths []string) string {
	hash := sha1.New()
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			absolute = path
		}
		_, _ = io.WriteString(hash, absolute)
		if info, err := os.Stat(absolute); err == nil {
			fmt.Fprintf(hash, "|%d|%d", info.Size(), info.ModTime().UnixNano())
		}
		_, _ = io.WriteString(hash, "\n")
	}
	return hex.EncodeToString(hash.Sum(nil))[:20]
}

const mountMarker = ".kizuna-ready"
const manifestFile = ".kizuna-disc.json"

func isMountReady(mount string) (bool, error) {
	info, err := os.Stat(filepath.Join(mount, mountMarker))
	return err == nil && !info.IsDir(), nil
}

func writeMountManifest(destination string, paths []string) error {
	manifest := make([]string, 0, len(paths))
	for _, path := range paths {
		if absolute, err := filepath.Abs(path); err == nil {
			manifest = append(manifest, absolute)
		} else {
			manifest = append(manifest, path)
		}
	}
	data := strings.Join(manifest, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(destination, manifestFile), []byte(data), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destination, mountMarker), []byte("ok\n"), 0o644)
}
