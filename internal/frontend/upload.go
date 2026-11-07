package frontend

import (
	"archive/zip"
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// 上传限制：压缩包 50MB，解压后累计 200MB。变量便于测试下调。
var (
	uploadMaxZipBytes   int64 = 50 << 20
	uploadMaxTotalBytes int64 = 200 << 20
)

// Upload 接收 zip：校验（大小、根目录 index.html、路径穿越）后
// 解压到临时目录再原子改名为版本目录。任何失败都不留文件。
func (m *Manager) Upload(app string, rd io.Reader) (VersionInfo, error) {
	data, err := io.ReadAll(io.LimitReader(rd, uploadMaxZipBytes+1))
	if err != nil {
		return VersionInfo{}, fmt.Errorf("读取上传内容: %w", err)
	}
	if int64(len(data)) > uploadMaxZipBytes {
		return VersionInfo{}, errors.New("压缩包超过 50MB 限制")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return VersionInfo{}, errors.New("不是合法的 zip 文件")
	}

	// 先整体校验条目，再落盘。
	hasIndex := false
	var declared int64
	type entry struct {
		rel  string
		file *zip.File
	}
	var files []entry
	for _, f := range zr.File {
		rel, err := safeRel(f.Name)
		if err != nil {
			return VersionInfo{}, err
		}
		if rel == "" {
			continue // 目录条目
		}
		if rel == "index.html" {
			hasIndex = true
		}
		declared += int64(f.UncompressedSize64)
		files = append(files, entry{rel: rel, file: f})
	}
	if declared > uploadMaxTotalBytes {
		return VersionInfo{}, errors.New("解压后超过 200MB 限制")
	}
	if !hasIndex {
		return VersionInfo{}, errors.New("zip 根目录缺少 index.html")
	}

	version := newVersionName()
	finalDir := filepath.Join(m.base, app, version)
	tmpDir := finalDir + ".tmp"
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return VersionInfo{}, err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(tmpDir)
		}
	}()

	var total int64
	var count int
	counting := func(n int64) error {
		total += n
		if total > uploadMaxTotalBytes {
			return errors.New("解压后超过 200MB 限制")
		}
		return nil
	}
	for _, e := range files {
		full := filepath.Join(tmpDir, filepath.FromSlash(e.rel))
		if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(tmpDir)+string(os.PathSeparator)) {
			return VersionInfo{}, fmt.Errorf("路径穿越: %q", e.file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return VersionInfo{}, err
		}
		src, err := e.file.Open()
		if err != nil {
			return VersionInfo{}, err
		}
		dst, oerr := os.Create(full)
		if oerr != nil {
			src.Close()
			return VersionInfo{}, oerr
		}
		err = copyLimited(dst, src, counting)
		src.Close()
		dst.Close()
		if err != nil {
			return VersionInfo{}, err
		}
		count++
	}

	if err := os.Rename(tmpDir, finalDir); err != nil {
		return VersionInfo{}, err
	}
	success = true
	return VersionInfo{
		Version:   version,
		Size:      total,
		Files:     count,
		CreatedAt: time.Now().Unix(),
	}, nil
}

// copyLimited 复制并累计字节数，超限即中止。
func copyLimited(dst io.Writer, src io.Reader, count func(int64) error) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if cerr := count(int64(n)); cerr != nil {
				return cerr
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// safeRel 清洗 zip 条目名：拒绝绝对路径与含 .. 段的路径穿越，
// 目录条目（以 / 结尾或本身为 .）返回空串。
func safeRel(name string) (string, error) {
	n := filepath.ToSlash(name)
	if strings.HasPrefix(n, "/") {
		return "", fmt.Errorf("zip 内含绝对路径: %q", name)
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return "", fmt.Errorf("zip 内含路径穿越: %q", name)
		}
	}
	rel := path.Clean(n)
	if rel == "." || rel == "/" {
		return "", nil
	}
	return rel, nil
}

// newVersionName 生成版本名：时间戳加 4 位随机后缀，同秒上传不冲突。
func newVersionName() string {
	b := make([]byte, 2)
	if _, err := crand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	return time.Now().Format("20060102150405") + "-" + hex.EncodeToString(b)
}
