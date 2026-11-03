package filesystem

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const sentinelContent = "keep me"

// writeZipArchive creates a zip archive at path using the supplied builder.
func writeZipArchive(t *testing.T, path string, build func(*zip.Writer)) {
	t.Helper()

	archive, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive %q: %v", path, err)
	}

	writer := zip.NewWriter(archive)
	build(writer)

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	if err := archive.Close(); err != nil {
		t.Fatalf("close archive file: %v", err)
	}
}

func zipAddEntry(t *testing.T, writer *zip.Writer, header *zip.FileHeader, content []byte) {
	t.Helper()

	entryWriter, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip entry %q: %v", header.Name, err)
	}

	if _, err := entryWriter.Write(content); err != nil {
		t.Fatalf("write zip entry %q: %v", header.Name, err)
	}
}

func zipAddFile(t *testing.T, writer *zip.Writer, name, content string, method uint16) {
	t.Helper()

	header := &zip.FileHeader{Name: name, Method: method}
	header.SetMode(0o644)
	zipAddEntry(t, writer, header, []byte(content))
}

func zipAddDir(t *testing.T, writer *zip.Writer, name string) {
	t.Helper()

	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(fs.ModeDir | 0o755)
	zipAddEntry(t, writer, header, nil)
}

// mustWriteFile writes content to path and fails the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	return string(content)
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected %q to not exist, got stat err: %v", path, err)
	}
}

func assertIsDir(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %q: %v", path, err)
	}

	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory, mode is %v", path, info.Mode())
	}
}

func assertDirEntries(t *testing.T, dir string, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list %q: %v", dir, err)
	}

	got := make([]string, 0, len(entries))

	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	sort.Strings(got)
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("directory %q listing = %v, want %v", dir, got, want)
	}
}

func withUnzipLimits(t *testing.T, entries int, totalBytes int64) {
	t.Helper()

	previousEntries := maxUnzipEntries
	previousBytes := maxUnzipBytes
	maxUnzipEntries = entries
	maxUnzipBytes = totalBytes

	t.Cleanup(func() {
		maxUnzipEntries = previousEntries
		maxUnzipBytes = previousBytes
	})
}

// TestUnzipNestedDirectories verifies that nested directories and files are
// fully published next to the archive while external files stay untouched.
func TestUnzipNestedDirectories(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "docs.zip")
	destinationPath := filepath.Join(archiveDir, "docs")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddDir(t, writer, "a/")
		zipAddDir(t, writer, "a/b/")
		zipAddFile(t, writer, "root.txt", "root content", zip.Store)
		zipAddFile(t, writer, "a/top.txt", "top content", zip.Deflate)
		zipAddFile(t, writer, "a/b/c/deep.txt", strings.Repeat("nested-", 20), zip.Deflate)
		zipAddDir(t, writer, "a/b/empty/")
	})

	mustWriteFile(t, filepath.Join(archiveDir, "sentinel.txt"), sentinelContent)

	// Run from an unrelated working directory: extraction must happen next
	// to the archive and read from the full archive path.
	workingDir := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir working dir: %v", err)
	}

	previousWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	unzipErr := Unzip(archivePath)

	if err := os.Chdir(previousWorkingDir); err != nil {
		t.Fatalf("restore working directory: %v", err)
	}

	if unzipErr != nil {
		t.Fatalf("Unzip returned error: %v", unzipErr)
	}

	assertNotExists(t, filepath.Join(workingDir, "docs"))

	// Every nested entry, including a directory whose parent was not an
	// explicit zip entry ("a/b/c/"), must be published.
	assertIsDir(t, destinationPath)
	assertIsDir(t, filepath.Join(destinationPath, "a"))
	assertIsDir(t, filepath.Join(destinationPath, "a", "b"))
	assertIsDir(t, filepath.Join(destinationPath, "a", "b", "c"))
	assertIsDir(t, filepath.Join(destinationPath, "a", "b", "empty"))

	if got := mustReadFile(t, filepath.Join(destinationPath, "root.txt")); got != "root content" {
		t.Fatalf("root.txt = %q, want %q", got, "root content")
	}

	if got := mustReadFile(t, filepath.Join(destinationPath, "a", "top.txt")); got != "top content" {
		t.Fatalf("a/top.txt = %q, want %q", got, "top content")
	}

	wantDeep := strings.Repeat("nested-", 20)
	if got := mustReadFile(t, filepath.Join(destinationPath, "a", "b", "c", "deep.txt")); got != wantDeep {
		t.Fatalf("deep.txt = %q, want %q", got, wantDeep)
	}

	// The external sentinel is untouched and no staging directory remains.
	if got := mustReadFile(t, filepath.Join(archiveDir, "sentinel.txt")); got != sentinelContent {
		t.Fatalf("sentinel.txt = %q, want %q", got, sentinelContent)
	}

	assertDirEntries(t, archiveDir, "docs", "docs.zip", "sentinel.txt")
}

func TestUnzipRejectsPathTraversal(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "evil.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, "../../../unzip-escape-sentinel.txt", "pwned", zip.Store)
		zipAddFile(t, writer, "sub/../../escape.txt", "pwned", zip.Store)
	})

	mustWriteFile(t, filepath.Join(archiveDir, "sentinel.txt"), sentinelContent)

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("Unzip error = %v, want a path traversal error", err)
	}

	escapedPath := filepath.Clean(filepath.Join(archiveDir, "..", "..", "..", "unzip-escape-sentinel.txt"))
	assertNotExists(t, escapedPath)
	assertNotExists(t, filepath.Join(archiveDir, "evil"))
	if got := mustReadFile(t, filepath.Join(archiveDir, "sentinel.txt")); got != sentinelContent {
		t.Fatalf("sentinel.txt = %q, want %q", got, sentinelContent)
	}

	assertDirEntries(t, archiveDir, "evil.zip", "sentinel.txt")
}

func TestUnzipRejectsAbsolutePathEntry(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "evil.zip")
	outsideTarget := filepath.Join(t.TempDir(), "absolute-sentinel.txt")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, outsideTarget, "pwned", zip.Store)
	})

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("Unzip error = %v, want an absolute path error", err)
	}

	assertNotExists(t, outsideTarget)
	assertNotExists(t, filepath.Join(archiveDir, "evil"))
	assertDirEntries(t, archiveDir, "evil.zip")
}

func TestUnzipRejectsSymlinkEntry(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "evil.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: "evil_link", Method: zip.Store}
		header.SetMode(fs.ModeSymlink | 0o777)
		zipAddEntry(t, writer, header, []byte("../sentinel.txt"))
	})

	mustWriteFile(t, filepath.Join(archiveDir, "sentinel.txt"), sentinelContent)

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Unzip error = %v, want a symbolic link error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "evil"))
	if got := mustReadFile(t, filepath.Join(archiveDir, "sentinel.txt")); got != sentinelContent {
		t.Fatalf("sentinel.txt = %q, want %q", got, sentinelContent)
	}

	assertDirEntries(t, archiveDir, "evil.zip", "sentinel.txt")
}

func TestUnzipRejectsSpecialEntry(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "evil.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: "fifo", Method: zip.Store}
		header.SetMode(fs.ModeNamedPipe | 0o600)
		zipAddEntry(t, writer, header, nil)
	})

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "named pipe") {
		t.Fatalf("Unzip error = %v, want a named pipe error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "evil"))
	assertDirEntries(t, archiveDir, "evil.zip")
}

func TestUnzipDoesNotOverwriteExistingDestination(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "docs.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, "root.txt", "from archive", zip.Store)
	})

	if err := os.MkdirAll(filepath.Join(archiveDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir destination: %v", err)
	}

	mustWriteFile(t, filepath.Join(archiveDir, "docs", "original.txt"), "original")

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Unzip error = %v, want an already exists error", err)
	}

	if got := mustReadFile(t, filepath.Join(archiveDir, "docs", "original.txt")); got != "original" {
		t.Fatalf("original.txt = %q, want %q", got, "original")
	}

	assertNotExists(t, filepath.Join(archiveDir, "docs", "root.txt"))
	assertDirEntries(t, archiveDir, "docs", "docs.zip")
}

func TestUnzipDoesNotFollowDestinationSymlink(t *testing.T) {
	archiveDir := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "target")
	archivePath := filepath.Join(archiveDir, "docs.zip")

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	mustWriteFile(t, filepath.Join(targetDir, "secret.txt"), "secret")

	if err := os.Symlink(targetDir, filepath.Join(archiveDir, "docs")); err != nil {
		t.Fatalf("create destination symlink: %v", err)
	}

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, "planted.txt", "from archive", zip.Store)
	})

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Unzip error = %v, want an already exists error", err)
	}

	linkInfo, err := os.Lstat(filepath.Join(archiveDir, "docs"))
	if err != nil {
		t.Fatalf("lstat destination link: %v", err)
	}

	if linkInfo.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("destination symlink was replaced with mode %v", linkInfo.Mode())
	}

	if got := mustReadFile(t, filepath.Join(targetDir, "secret.txt")); got != "secret" {
		t.Fatalf("target secret.txt = %q, want %q", got, "secret")
	}

	assertNotExists(t, filepath.Join(targetDir, "planted.txt"))
}

func TestUnzipRejectsTooManyEntries(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "docs.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		for i := 0; i < 5; i++ {
			zipAddFile(t, writer, "file"+strings.Repeat("x", i)+".txt", "x", zip.Store)
		}
	})

	withUnzipLimits(t, 3, maxUnzipBytes)

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("Unzip error = %v, want an entry limit error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "docs"))
	assertDirEntries(t, archiveDir, "docs.zip")
}

func TestUnzipRejectsTooManyBytes(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "docs.zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, "big.txt", strings.Repeat("z", 16), zip.Store)
	})

	withUnzipLimits(t, maxUnzipEntries, 8)

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("Unzip error = %v, want a size limit error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "docs"))
	assertDirEntries(t, archiveDir, "docs.zip")
}

func TestUnzipRejectsCorruptArchive(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "broken.zip")

	mustWriteFile(t, archivePath, "this is definitely not a zip file")

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "cannot open archive") {
		t.Fatalf("Unzip error = %v, want a cannot open archive error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "broken"))
	assertDirEntries(t, archiveDir, "broken.zip")
}

func TestUnzipRejectsChecksumMismatchAndCleansUp(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "brokencrc.zip")

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	zipAddFile(t, writer, "f.txt", "hello world", zip.Store)

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	raw := buffer.Bytes()

	index := bytes.Index(raw, []byte("hello world"))
	if index < 0 {
		t.Fatalf("stored payload not found inside archive bytes")
	}

	raw[index] = 'X'

	if err := os.WriteFile(archivePath, raw, 0o644); err != nil {
		t.Fatalf("write corrupt archive: %v", err)
	}

	err := Unzip(archivePath)
	if err == nil {
		t.Fatalf("Unzip error = nil, want a corrupt archive error")
	}

	if !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("Unzip error = %v, want a corrupt archive error", err)
	}

	assertNotExists(t, filepath.Join(archiveDir, "brokencrc"))
	assertDirEntries(t, archiveDir, "brokencrc.zip")
}

func TestUnzipRejectsDirectoryAsArchive(t *testing.T) {
	archiveDir := t.TempDir()

	err := Unzip(archiveDir)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("Unzip error = %v, want an is a directory error", err)
	}
}

func TestUnzipRejectsEmptyDerivedName(t *testing.T) {
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, ".zip")

	writeZipArchive(t, archivePath, func(writer *zip.Writer) {
		zipAddFile(t, writer, "f.txt", "x", zip.Store)
	})

	err := Unzip(archivePath)
	if err == nil || !strings.Contains(err.Error(), "output directory name") {
		t.Fatalf("Unzip error = %v, want an output directory name error", err)
	}

	assertDirEntries(t, archiveDir, ".zip")
}

func TestUnzipDestinationNameDerivation(t *testing.T) {
	cases := map[string]string{
		"docs.zip":       "docs",
		"Docs.ZIP":       "Docs",
		"a.b.zip":        "a.b",
		".hidden.zip":    ".hidden",
		"archive":        "archive_unzipped",
		"archive.tar":    "archive.tar_unzipped",
		"backup.zip.bak": "backup.zip.bak_unzipped",
	}

	for input, want := range cases {
		if got := unzipDestinationName(input); got != want {
			t.Fatalf("unzipDestinationName(%q) = %q, want %q", input, got, want)
		}
	}
}
