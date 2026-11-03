// Package filesystem is a collection of various different filesystem
// helper functions.
package filesystem

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Directory shortcuts.
const (
	CurrentDirectory  = "."
	PreviousDirectory = ".."
	HomeDirectory     = "~"
	RootDirectory     = "/"
)

const (
	thousand    = 1000
	ten         = 10
	fivePercent = 0.0499
)

// Different types of listings.
const (
	DirectoriesListingType = "directories"
	FilesListingType       = "files"
)

// RenameDirectoryItem renames a directory or files given a source and destination.
func RenameDirectoryItem(src, dst string) error {
	err := os.Rename(src, dst)

	return errors.Unwrap(err)
}

// CreateDirectory creates a new directory given a name.
func CreateDirectory(name string) error {
	if _, err := os.Stat(name); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(name, os.ModePerm)
		if err != nil {
			return errors.Unwrap(err)
		}
	}

	return nil
}

// GetDirectoryListing returns a list of files and directories within a given directory.
func GetDirectoryListing(dir string, showHidden bool) ([]fs.DirEntry, error) {
	index := 0

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	if !showHidden {
		for _, file := range files {
			// If the file or directory starts with a dot,
			// we know its hidden so dont add it to the array
			// of files to return.
			if !strings.HasPrefix(file.Name(), ".") {
				files[index] = file
				index++
			}
		}

		// Set files to the list that does not include hidden files.
		files = files[:index]
	}

	return files, nil
}

// GetDirectoryListingByType returns a directory listing based on type (directories | files).
func GetDirectoryListingByType(dir, listingType string, showHidden bool) ([]fs.DirEntry, error) {
	index := 0

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	for _, file := range files {
		switch {
		case file.IsDir() && listingType == DirectoriesListingType && !showHidden:
			if !strings.HasPrefix(file.Name(), ".") {
				files[index] = file
				index++
			}
		case file.IsDir() && listingType == DirectoriesListingType && showHidden:
			files[index] = file
			index++
		case !file.IsDir() && listingType == FilesListingType && !showHidden:
			if !strings.HasPrefix(file.Name(), ".") {
				files[index] = file
				index++
			}
		case !file.IsDir() && listingType == FilesListingType && showHidden:
			files[index] = file
			index++
		}
	}

	return files[:index], nil
}

// DeleteDirectory deletes a directory given a name.
func DeleteDirectory(name string) error {
	err := os.RemoveAll(name)

	return errors.Unwrap(err)
}

// GetHomeDirectory returns the users home directory.
func GetHomeDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Unwrap(err)
	}

	return home, nil
}

// GetWorkingDirectory returns the current working directory.
func GetWorkingDirectory() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", errors.Unwrap(err)
	}

	return workingDir, nil
}

// DeleteFile deletes a file given a name.
func DeleteFile(name string) error {
	err := os.Remove(name)

	return errors.Unwrap(err)
}

// MoveDirectoryItem moves a file from one place to another.
func MoveDirectoryItem(src, dst string) error {
	err := os.Rename(src, dst)

	return errors.Unwrap(err)
}

// ReadFileContent returns the contents of a file given a name.
func ReadFileContent(name string) (string, error) {
	fileContent, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		return "", errors.Unwrap(err)
	}

	return string(fileContent), nil
}

// CreateFile creates a file given a name.
func CreateFile(name string) error {
	f, err := os.Create(filepath.Clean(name))
	if err != nil {
		return errors.Unwrap(err)
	}

	if err = f.Close(); err != nil {
		return errors.Unwrap(err)
	}

	return errors.Unwrap(err)
}

// Zip zips a directory given a name.
func Zip(name string) error {
	var splitName []string
	var output string

	srcFile, err := os.Open(filepath.Clean(name))
	if err != nil {
		return errors.Unwrap(err)
	}

	defer func() {
		err = srcFile.Close()
	}()

	fileExtension := filepath.Ext(name)
	splitFileName := strings.Split(name, "/")
	fileName := splitFileName[len(splitFileName)-1]

	switch {
	case strings.HasPrefix(fileName, ".") && fileExtension != "" && fileExtension == fileName:
		output = fmt.Sprintf("%s_%d.zip", fileName, time.Now().Unix())
	case strings.HasPrefix(fileName, ".") && fileExtension != "" && fileExtension != fileName:
		splitName = strings.Split(fileName, ".")
		output = fmt.Sprintf(".%s_%d.zip", splitName[1], time.Now().Unix())
	case fileExtension != "":
		splitName = strings.Split(fileName, ".")
		output = fmt.Sprintf("%s_%d.zip", splitName[0], time.Now().Unix())
	default:
		output = fmt.Sprintf("%s_%d.zip", fileName, time.Now().Unix())
	}

	newfile, err := os.Create(filepath.Clean(output))
	if err != nil {
		return errors.Unwrap(err)
	}

	defer func() {
		err = newfile.Close()
	}()

	zipWriter := zip.NewWriter(newfile)
	defer func() {
		err = zipWriter.Close()
	}()

	info, err := os.Stat(name)
	if err != nil {
		return errors.Unwrap(err)
	}

	if info.IsDir() {
		err = filepath.Walk(name, func(filePath string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			if err != nil {
				return errors.Unwrap(err)
			}

			relPath := strings.TrimPrefix(filePath, name)
			zipFile, err := zipWriter.Create(relPath)

			if err != nil {
				return errors.Unwrap(err)
			}

			fsFile, err := os.Open(filepath.Clean(filePath))
			if err != nil {
				return errors.Unwrap(err)
			}

			_, err = io.Copy(zipFile, fsFile)
			if err != nil {
				return errors.Unwrap(err)
			}

			return nil
		})
	} else {
		var files []string

		// Walk the directory to get a list of files within it and append it to
		// the array of files.
		err := filepath.Walk(name, func(path string, f fs.FileInfo, err error) error {
			if f.Name() != "." && !f.IsDir() {
				files = append(files, path)
			}

			return nil
		})

		if err != nil {
			return errors.Unwrap(err)
		}

		for _, file := range files {
			zipfile, err := os.Open(filepath.Clean(file))
			if err != nil {
				return errors.Unwrap(err)
			}

			defer func() {
				err = zipfile.Close()
			}()

			info, err := zipfile.Stat()
			if err != nil {
				return errors.Unwrap(err)
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return errors.Unwrap(err)
			}

			header.Method = zip.Deflate
			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return errors.Unwrap(err)
			}

			_, err = io.Copy(writer, zipfile)
			if err != nil {
				return errors.Unwrap(err)
			}
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return errors.Unwrap(err)
	}

	return errors.Unwrap(err)
}

// Extraction limits protect against corrupted or malicious archives such as
// zip bombs. They are variables instead of constants so that tests can use
// smaller values.
var (
	maxUnzipEntries = 10_000
	maxUnzipBytes   = int64(1 << 30) // 1 GiB
)

// Unzip extracts a zip archive into a new directory created next to the
// archive. The selected archive is read from its full path, fully extracted
// into a private staging directory inside the archive's own directory, and
// only published with an atomic rename once every entry has been validated
// and written. Entries containing path traversal, symbolic links or special
// file types are rejected, extraction is limited by entry count and total
// uncompressed size, and any failure removes the staging directory without
// modifying anything else on disk.
func Unzip(name string) error {
	archivePath, err := filepath.Abs(name)
	if err != nil {
		return fmt.Errorf("unzip %q: cannot resolve archive path: %w", name, err)
	}

	// Resolve symlinks so the archive and every path derived from it points
	// at the real on-disk location rather than writing through a link.
	archivePath, err = filepath.EvalSymlinks(archivePath)
	if err != nil {
		return fmt.Errorf("unzip %q: cannot access archive: %w", name, err)
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("unzip %q: cannot stat archive: %w", name, err)
	}

	if info.IsDir() {
		return fmt.Errorf("unzip %q: %s is a directory, not a zip archive", name, archivePath)
	}

	archiveDir := filepath.Dir(archivePath)
	archiveBase := filepath.Base(archivePath)
	destinationBase := unzipDestinationName(archiveBase)

	if destinationBase == "" {
		return fmt.Errorf("unzip %q: cannot derive an output directory name from %q", name, archiveBase)
	}

	destinationPath := filepath.Join(archiveDir, destinationBase)

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("unzip %q: cannot open archive: %w", name, err)
	}

	defer func() {
		_ = reader.Close()
	}()

	// Fail fast instead of extracting when the output directory already
	// exists. Lstat is used on purpose so that a symbolic link at the
	// destination is detected instead of being followed.
	if err := rejectIfDestinationExists(destinationPath); err != nil {
		return fmt.Errorf("unzip %q: %w", name, err)
	}

	// Validate every entry before anything is written to disk and total the
	// declared uncompressed size for an early zip-bomb check.
	var declaredBytes uint64

	for _, file := range reader.File {
		mode := file.Mode()

		if !mode.IsDir() && !mode.IsRegular() {
			return fmt.Errorf(
				"unzip %q: entry %q is a %s, only regular files and directories are allowed",
				name, file.Name, unsupportedEntryKind(mode),
			)
		}

		if _, err := safeZipEntryPath(file.Name); err != nil {
			return fmt.Errorf("unzip %q: %w", name, err)
		}

		declaredBytes += file.UncompressedSize64
	}

	if len(reader.File) > maxUnzipEntries {
		return fmt.Errorf(
			"unzip %q: archive contains %d entries, extraction limit is %d entries",
			name, len(reader.File), maxUnzipEntries,
		)
	}

	if declaredBytes > uint64(maxUnzipBytes) {
		return fmt.Errorf(
			"unzip %q: archive declares %d uncompressed bytes, extraction limit is %d bytes",
			name, declaredBytes, maxUnzipBytes,
		)
	}

	stagingPath, err := os.MkdirTemp(archiveDir, "."+archiveBase+".unzip-staging-*")
	if err != nil {
		return fmt.Errorf("unzip %q: cannot create staging directory in %q: %w",
			name, archiveDir, err)
	}

	// Any return from this point on removes the staging directory so a
	// corrupt archive, an exceeded limit or a write failure can never leave
	// a half-extracted result behind.
	published := false

	defer func() {
		if !published {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	var totalBytes int64

	for _, file := range reader.File {
		entryPath, err := safeZipEntryPath(file.Name)
		if err != nil {
			return fmt.Errorf("unzip %q: %w", name, err)
		}

		targetPath := filepath.Join(stagingPath, entryPath)
		if !withinDirectory(stagingPath, targetPath) {
			return fmt.Errorf("unzip %q: entry %q escapes the staging directory", name, file.Name)
		}

		if file.Mode().IsDir() {
			if err := os.MkdirAll(targetPath, file.Mode().Perm()|0o700); err != nil {
				return fmt.Errorf("unzip %q: cannot create directory %q: %w", name, file.Name, err)
			}

			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("unzip %q: cannot create parent directory for %q: %w", name, file.Name, err)
		}

		// O_EXCL guarantees a fresh file inside the brand-new staging
		// directory: an existing entry can never be truncated or followed.
		outFile, err := os.OpenFile(
			targetPath,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			file.Mode().Perm()|0o600,
		)
		if err != nil {
			return fmt.Errorf("unzip %q: cannot create file %q: %w", name, file.Name, err)
		}

		archiveFile, err := file.Open()
		if err != nil {
			_ = outFile.Close()
			return fmt.Errorf("unzip %q: cannot open zip entry %q: %w", name, file.Name, err)
		}

		remaining := maxUnzipBytes - totalBytes

		// Read at most one byte beyond the remaining budget so an entry that
		// lies about its declared size is still detected from the bytes it
		// actually streams. CRC or compression errors surface here too.
		limited := &io.LimitedReader{R: archiveFile, N: remaining + 1}

		copied, copyErr := io.Copy(outFile, limited)

		readCloseErr := archiveFile.Close()
		writeCloseErr := outFile.Close()

		if copyErr != nil {
			return fmt.Errorf("unzip %q: failed to extract %q, the archive may be corrupt: %w",
				name, file.Name, copyErr)
		}

		if readCloseErr != nil {
			return fmt.Errorf("unzip %q: cannot close zip entry %q: %w", name, file.Name, readCloseErr)
		}

		if writeCloseErr != nil {
			return fmt.Errorf("unzip %q: cannot write file %q: %w", name, file.Name, writeCloseErr)
		}

		if copied > remaining {
			return fmt.Errorf(
				"unzip %q: entry %q exceeds the remaining extraction budget, total limit is %d bytes",
				name, file.Name, maxUnzipBytes,
			)
		}

		totalBytes += copied
	}

	// Authoritative collision check immediately before publishing: never
	// replace or follow an existing file, directory or symbolic link.
	if err := rejectIfDestinationExists(destinationPath); err != nil {
		return fmt.Errorf("unzip %q: %w", name, err)
	}

	if err := os.Chmod(stagingPath, 0o755); err != nil {
		return fmt.Errorf("unzip %q: cannot finalize staging directory: %w", name, err)
	}

	if err := os.Rename(stagingPath, destinationPath); err != nil {
		return fmt.Errorf("unzip %q: cannot publish extracted directory: %w", name, err)
	}

	published = true

	return nil
}

// unzipDestinationName derives the extracted directory name from an archive
// file name: a trailing ".zip" extension is removed case-insensitively,
// otherwise "_unzipped" is appended.
func unzipDestinationName(archiveBase string) string {
	if ext := filepath.Ext(archiveBase); strings.EqualFold(ext, ".zip") {
		return archiveBase[:len(archiveBase)-len(ext)]
	}

	return archiveBase + "_unzipped"
}

// safeZipEntryPath validates an entry name stored inside a zip archive and
// returns the cleaned relative path that may be joined with a destination
// directory. Absolute paths and any ".." traversal are rejected.
func safeZipEntryPath(name string) (string, error) {
	if name == "" {
		return "", errors.New("zip entry has an empty name")
	}

	cleaned := filepath.Clean(filepath.FromSlash(name))

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("zip entry %q uses an absolute path", name)
	}

	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == ".." {
			return "", fmt.Errorf("zip entry %q contains path traversal", name)
		}
	}

	return cleaned, nil
}

// withinDirectory reports whether target is root itself or located inside it.
// Both paths must be cleaned absolute paths.
func withinDirectory(root, target string) bool {
	if target == root {
		return true
	}

	return strings.HasPrefix(target, root+string(os.PathSeparator))
}

// rejectIfDestinationExists returns an error when anything already exists at
// path, including a dangling symbolic link, so extraction never overwrites or
// follows an existing destination.
func rejectIfDestinationExists(path string) error {
	_, err := os.Lstat(path)

	switch {
	case err == nil:
		return fmt.Errorf("destination %q already exists, refusing to overwrite it", path)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("cannot inspect destination %q: %w", path, err)
	}
}

// unsupportedEntryKind describes a rejected zip entry type for error messages.
func unsupportedEntryKind(mode os.FileMode) string {
	switch {
	case mode&os.ModeSymlink != 0:
		return "symbolic link"
	case mode&os.ModeNamedPipe != 0:
		return "named pipe"
	case mode&os.ModeSocket != 0:
		return "socket"
	case mode&os.ModeDevice != 0 && mode&os.ModeCharDevice != 0:
		return "character device"
	case mode&os.ModeDevice != 0:
		return "block device"
	case mode&os.ModeSetuid != 0 || mode&os.ModeSetgid != 0:
		return "setuid/setgid file"
	default:
		return "special file"
	}
}

// CopyFile copies a file given a name.
func CopyFile(name string) error {
	var splitName []string
	var output string

	srcFile, err := os.Open(filepath.Clean(name))
	if err != nil {
		return errors.Unwrap(err)
	}

	defer func() {
		err = srcFile.Close()
	}()

	fileExtension := filepath.Ext(name)
	splitFileName := strings.Split(name, "/")
	fileName := splitFileName[len(splitFileName)-1]

	switch {
	case strings.HasPrefix(fileName, ".") && fileExtension != "" && fileExtension == fileName:
		output = fmt.Sprintf("%s_%d", fileName, time.Now().Unix())
	case strings.HasPrefix(fileName, ".") && fileExtension != "" && fileExtension != fileName:
		splitName = strings.Split(fileName, ".")
		output = fmt.Sprintf(".%s_%d.%s", splitName[1], time.Now().Unix(), splitName[2])
	case fileExtension != "":
		splitName = strings.Split(fileName, ".")
		output = fmt.Sprintf("%s_%d.%s", splitName[0], time.Now().Unix(), splitName[1])
	default:
		output = fmt.Sprintf("%s_%d", fileName, time.Now().Unix())
	}

	destFile, err := os.Create(filepath.Clean(output))
	if err != nil {
		return errors.Unwrap(err)
	}

	defer func() {
		err = destFile.Close()
	}()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return errors.Unwrap(err)
	}

	err = destFile.Sync()
	if err != nil {
		return errors.Unwrap(err)
	}

	return errors.Unwrap(err)
}

// CopyDirectory copies a directory given a path.
func CopyDirectory(pathname string) error {
	name := filepath.Base(pathname)
	output := fmt.Sprintf("%s_%d", name, time.Now().Unix())

	err := filepath.Walk(pathname, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err // Return early if there's an error walking the path
		}

		relPath, err := filepath.Rel(pathname, path)
		if err != nil {
			return err // Return if there's an error getting the relative path
		}

		targetPath := filepath.Join(output, relPath)

		if info.IsDir() {
			return os.Mkdir(targetPath, os.ModePerm)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err // Return if there's an error reading the file
		}

		err = os.WriteFile(targetPath, data, os.ModePerm)
		if err != nil {
			return err // Return if there's an error writing the file
		}

		return nil
	})

	if err != nil {
		return err // Return the final error, if any
	}

	return nil
}

// GetDirectoryItemSize calculates the size of a directory or file.
func GetDirectoryItemSize(path string) (int64, error) {
	var size int64

	curFile, err := os.Stat(path)
	if err != nil {
		return 0, errors.Unwrap(err)
	}

	if !curFile.IsDir() {
		return curFile.Size(), nil
	}

	err = filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return errors.Unwrap(err)
		}

		fileInfo, err := entry.Info()
		if err != nil {
			return errors.Unwrap(err)
		}

		if !entry.IsDir() {
			size += fileInfo.Size()
		}

		return errors.Unwrap(err)
	})

	return size, errors.Unwrap(err)
}

// FindFilesByName returns files found based on a name.
func FindFilesByName(name, dir string) ([]string, []fs.DirEntry, error) {
	var paths []string
	var entries []fs.DirEntry

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		if strings.Contains(entry.Name(), name) {
			paths = append(paths, path)
			entries = append(entries, entry)
		}

		return errors.Unwrap(err)
	})

	return paths, entries, errors.Unwrap(err)
}

// WriteToFile writes content to a file, overwriting content if it exists.
func WriteToFile(path, content string) error {
	file, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return errors.Unwrap(err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return errors.Unwrap(err)
	}

	_, err = fmt.Fprintf(file, "%s\n", filepath.Join(workingDir, content))
	if err != nil {
		err = file.Close()
		if err != nil {
			return errors.Unwrap(err)
		}

		return errors.Unwrap(err)
	}

	err = file.Close()
	if err != nil {
		return errors.Unwrap(err)
	}

	return errors.Unwrap(err)
}

// ConvertBytesToSizeString converts a byte count to a human readable string.
func ConvertBytesToSizeString(size int64) string {
	if size < thousand {
		return fmt.Sprintf("%dB", size)
	}

	suffix := []string{
		"K", // kilo
		"M", // mega
		"G", // giga
		"T", // tera
		"P", // peta
		"E", // exa
		"Z", // zeta
		"Y", // yotta
	}

	curr := float64(size) / thousand
	for _, s := range suffix {
		if curr < ten {
			return fmt.Sprintf("%.1f%s", curr-fivePercent, s)
		} else if curr < thousand {
			return fmt.Sprintf("%d%s", int(curr), s)
		}
		curr /= thousand
	}

	return ""
}

func IsBinary(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buffer := make([]byte, 512)
	n, _ := f.Read(buffer)

	return bytes.Contains(buffer[:n], []byte{0}), nil
}
