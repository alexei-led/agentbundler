// Package write materializes validated build plans as complete output trees.
package write

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	diagnosticWriteFailed = "ARTIFACT_WRITE_FAILED"
	diagnosticExecutable  = "ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED"

	phasePrepared     = "prepared"
	phaseOldMoved     = "old-moved"
	phaseNewInstalled = "new-installed"
)

type stagedFile struct {
	path       string
	bytes      []byte
	executable bool
}

type replacementJournal struct {
	Output  string `json:"output"`
	Staging string `json:"staging"`
	Backup  string `json:"backup"`
	HadOld  bool   `json:"hadOld"`
	Phase   string `json:"phase"`
}

// ReplaceOutput stages plan and replaces outputRoot without exposing a partial output tree.
func ReplaceOutput(plan model.BuildPlan, outputRoot string) []model.Diagnostic {
	if runtime.GOOS == "windows" && hasExecutableFile(plan) {
		return []model.Diagnostic{{
			Code:     diagnosticExecutable,
			Severity: model.SeverityError,
			Message:  "executable file intent is unsupported on Windows",
		}}
	}

	if err := validateOutputRoot(outputRoot); err != nil {
		return writeFailure("inspect output root", err)
	}
	if err := recoverJournal(outputRoot); err != nil {
		return writeFailure("recover output replacement", err)
	}

	files := plannedFiles(plan)
	staging, err := makeStaging(outputRoot)
	if err != nil {
		return writeFailure("create staging tree", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := stageFiles(staging, files); err != nil {
		return writeFailure("stage output", err)
	}
	if err := verifyStaging(staging, files); err != nil {
		return writeFailure("verify staging tree", err)
	}
	if err := syncTree(staging); err != nil {
		return writeFailure("sync staging tree", err)
	}

	if err := replace(staging, outputRoot); err != nil {
		return writeFailure("replace output", err)
	}
	return nil
}

// ReplaceOutputInRoot stages and replaces outputName below a pinned parent
// directory. Every filesystem operation uses the descriptor-relative root.
// This is the production path used by artifact.Write.
func ReplaceOutputInRoot(plan model.BuildPlan, outputName string, parent *os.Root) []model.Diagnostic {
	if parent == nil {
		return writeFailure("replace output", errors.New("pinned output parent is required"))
	}
	if outputName == "" || filepath.Base(outputName) != outputName || outputName == "." || outputName == ".." {
		return writeFailure("replace output", errors.New("output name must be a single relative path component"))
	}
	if runtime.GOOS == "windows" && hasExecutableFile(plan) {
		return []model.Diagnostic{{Code: diagnosticExecutable, Severity: model.SeverityError, Message: "executable file intent is unsupported on Windows"}}
	}

	files := plannedFiles(plan)
	staging, err := pinnedStagingName(parent, outputName)
	if err != nil {
		return writeFailure("create staging tree", err)
	}
	defer func() { _ = parent.RemoveAll(staging) }()
	if err := stageFilesInRoot(parent, staging, files); err != nil {
		return writeFailure("stage output", err)
	}
	if err := verifyStagingInRoot(parent, staging, files); err != nil {
		return writeFailure("verify staging tree", err)
	}
	if err := syncTreeInRoot(parent, staging); err != nil {
		return writeFailure("sync staging tree", err)
	}

	oldInfo, err := parent.Lstat(outputName)
	hadOld := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return writeFailure("inspect output root", err)
	}
	if hadOld && (oldInfo.Mode()&os.ModeSymlink != 0 || !oldInfo.IsDir()) {
		return writeFailure("inspect output root", errors.New("output root is not a directory"))
	}
	backup := "." + outputName + ".agbun-write-backup-" + filepath.Base(staging)
	if hadOld {
		if err := parent.Rename(outputName, backup); err != nil {
			return writeFailure("replace output", err)
		}
	}
	if err := parent.Rename(staging, outputName); err != nil {
		if hadOld {
			_ = parent.Rename(backup, outputName)
		}
		return writeFailure("replace output", err)
	}
	if hadOld {
		_ = parent.RemoveAll(backup)
	}
	if err := syncPinnedDirectory(parent); err != nil {
		return writeFailure("sync output root", err)
	}
	return nil
}

func pinnedStagingName(parent *os.Root, outputName string) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		var random [12]byte
		if _, err := cryptorand.Read(random[:]); err != nil {
			return "", err
		}
		name := "." + outputName + ".agbun-write-staging-" + hex.EncodeToString(random[:])
		if err := parent.Mkdir(name, 0o700); errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return "", err
		}
		return name, nil
	}
	return "", errors.New("could not allocate staging directory")
}

func stageFilesInRoot(parent *os.Root, staging string, files []stagedFile) error {
	for _, file := range files {
		name := filepath.ToSlash(filepath.Join(staging, file.path))
		if err := parent.MkdirAll(filepath.ToSlash(filepath.Dir(filepath.Join(staging, file.path))), 0o755); err != nil {
			return err
		}
		if err := writeRootFile(parent, name, file.bytes, file.executable); err != nil {
			return err
		}
	}
	return nil
}

func writeRootFile(parent *os.Root, name string, data []byte, executable bool) error {
	file, err := parent.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		mode := os.FileMode(0o644)
		if executable {
			mode = 0o755
		}
		return parent.Chmod(name, mode)
	}
	return nil
}

func verifyStagingInRoot(parent *os.Root, staging string, files []stagedFile) error {
	for _, file := range files {
		name := filepath.ToSlash(filepath.Join(staging, file.path))
		info, err := parent.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("staged path %q is not a regular file", file.path)
		}
		data, err := parent.ReadFile(name)
		if err != nil || !bytes.Equal(data, file.bytes) {
			return fmt.Errorf("staged path %q differs from planned bytes", file.path)
		}
		if runtime.GOOS != "windows" && (info.Mode()&0o111 != 0) != file.executable {
			return fmt.Errorf("staged path %q has incorrect executable intent", file.path)
		}
	}
	return nil
}

func syncTreeInRoot(parent *os.Root, staging string) error {
	file, err := parent.Open(staging)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func syncPinnedDirectory(parent *os.Root) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := parent.Open(".")
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func hasExecutableFile(plan model.BuildPlan) bool {
	for _, target := range plan.Targets {
		for _, file := range target.Files {
			if file.Executable {
				return true
			}
		}
	}
	for _, file := range plan.CompilerFiles {
		if file.Executable {
			return true
		}
	}
	return false
}

func validateOutputRoot(outputRoot string) error {
	if filepath.Dir(outputRoot) == outputRoot {
		return errors.New("output root must have a parent directory")
	}
	info, err := os.Lstat(outputRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("output root must not be a symbolic link")
	}
	if !info.IsDir() {
		return errors.New("output root must be a directory")
	}
	return nil
}

func plannedFiles(plan model.BuildPlan) []stagedFile {
	files := make([]stagedFile, 0, len(plan.CompilerFiles))
	for _, file := range plan.CompilerFiles {
		files = append(files, stagedFile{
			path:       filepath.FromSlash(string(file.Path)),
			bytes:      file.Bytes,
			executable: file.Executable,
		})
	}
	for _, target := range plan.Targets {
		for _, file := range target.Files {
			files = append(files, stagedFile{
				path:       filepath.Join(string(target.Target), filepath.FromSlash(string(file.Path))),
				bytes:      file.Bytes,
				executable: file.Executable,
			})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files
}

func makeStaging(outputRoot string) (string, error) {
	parent, base := filepath.Dir(outputRoot), filepath.Base(outputRoot)
	return os.MkdirTemp(parent, "."+base+".agbun-write-staging-")
}

func stageFiles(staging string, files []stagedFile) error {
	for _, file := range files {
		path := filepath.Join(staging, file.path)
		if filepath.Dir(path) == staging {
			// The staging root already exists.
		} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := writeFile(path, file.bytes, file.executable); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, bytes []byte, executable bool) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(bytes); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}
	return os.Chmod(path, mode)
}

func verifyStaging(staging string, files []stagedFile) error {
	for _, file := range files {
		path := filepath.Join(staging, file.path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("staged path %q is not a regular file", file.path)
		}
		stagedBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(stagedBytes, file.bytes) {
			return fmt.Errorf("staged path %q differs from planned bytes", file.path)
		}
		if runtime.GOOS != "windows" && (info.Mode()&0o111 != 0) != file.executable {
			return fmt.Errorf("staged path %q has incorrect executable intent", file.path)
		}
	}
	return nil
}

func replace(staging, outputRoot string) error {
	hadOld, err := outputDirectoryExists(outputRoot)
	if err != nil {
		return err
	}
	if !hadOld {
		return installWithoutOldRoot(staging, outputRoot)
	}
	if exchangeDirectories(staging, outputRoot) {
		if err := syncDirectory(filepath.Dir(outputRoot)); err != nil {
			if exchangeDirectories(staging, outputRoot) {
				_ = syncDirectory(filepath.Dir(outputRoot))
			}
			return err
		}
		_ = os.RemoveAll(staging)
		return nil
	}
	return replaceWithJournal(staging, outputRoot, true)
}

func installWithoutOldRoot(staging, outputRoot string) (err error) {
	journal, err := newReplacementJournal(staging, outputRoot, false)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			return
		}
		if recoveryErr := recoverJournal(outputRoot); recoveryErr != nil {
			err = errors.Join(err, recoveryErr)
		}
	}()
	if err = persistJournal(journalPath(outputRoot), journal); err != nil {
		return err
	}
	if err = os.Rename(staging, outputRoot); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(outputRoot)); err != nil {
		return err
	}
	journal.Phase = phaseNewInstalled
	if err = persistJournal(journalPath(outputRoot), journal); err != nil {
		return err
	}
	_ = removeJournal(journalPath(outputRoot))
	return nil
}

func replaceWithJournal(staging, outputRoot string, hadOld bool) (err error) {
	journal, err := newReplacementJournal(staging, outputRoot, hadOld)
	if err != nil {
		return err
	}
	path := journalPath(outputRoot)
	defer func() {
		if err == nil {
			return
		}
		if recoveryErr := recoverJournal(outputRoot); recoveryErr != nil {
			err = errors.Join(err, recoveryErr)
		}
	}()
	if err = persistJournal(path, journal); err != nil {
		return err
	}
	if err = os.Rename(outputRoot, journal.Backup); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(outputRoot)); err != nil {
		return err
	}
	journal.Phase = phaseOldMoved
	if err = persistJournal(path, journal); err != nil {
		return err
	}
	if err = os.Rename(staging, outputRoot); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(outputRoot)); err != nil {
		return err
	}
	journal.Phase = phaseNewInstalled
	if err = persistJournal(path, journal); err != nil {
		return err
	}
	_ = os.RemoveAll(journal.Backup)
	_ = removeJournal(path)
	return nil
}

func outputDirectoryExists(outputRoot string) (bool, error) {
	info, err := os.Lstat(outputRoot)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, errors.New("output root is not a directory")
	}
	return true, nil
}

func journalPath(outputRoot string) string {
	return filepath.Join(filepath.Dir(outputRoot), "."+filepath.Base(outputRoot)+".agbun-write-journal.json")
}

func newReplacementJournal(staging, outputRoot string, hadOld bool) (replacementJournal, error) {
	transactionID, err := stagingTransactionID(outputRoot, staging)
	if err != nil {
		return replacementJournal{}, err
	}
	return replacementJournal{
		Output:  outputRoot,
		Staging: staging,
		Backup:  backupPath(outputRoot, transactionID),
		HadOld:  hadOld,
		Phase:   phasePrepared,
	}, nil
}

func stagingTransactionID(outputRoot, staging string) (string, error) {
	parent, base := filepath.Dir(outputRoot), filepath.Base(outputRoot)
	prefix := "." + base + ".agbun-write-staging-"
	if filepath.Dir(staging) != parent || !strings.HasPrefix(filepath.Base(staging), prefix) {
		return "", errors.New("output journal has an invalid staging path")
	}
	transactionID := strings.TrimPrefix(filepath.Base(staging), prefix)
	if transactionID == "" {
		return "", errors.New("output journal staging path has an empty transaction ID")
	}
	return transactionID, nil
}

func backupPath(outputRoot, transactionID string) string {
	return filepath.Join(filepath.Dir(outputRoot), "."+filepath.Base(outputRoot)+".agbun-write-backup-"+transactionID)
}

func persistJournal(path string, journal replacementJournal) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(journal); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func removeStaleJournalTemporary(path string) error {
	temporary := path + ".tmp"
	info, err := os.Lstat(temporary)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("output journal temporary is not a regular file")
	}
	return os.Remove(temporary)
}

func removeJournal(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func recoverJournal(outputRoot string) error {
	path := journalPath(outputRoot)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return removeStaleJournalTemporary(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("output journal is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var journal replacementJournal
	err = decoder.Decode(&journal)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("output journal has trailing data")
		}
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := validateJournal(outputRoot, journal); err != nil {
		return err
	}
	if err := removeStaleJournalTemporary(path); err != nil {
		return err
	}

	switch journal.Phase {
	case phasePrepared:
		outputExists, err := outputDirectoryExists(outputRoot)
		if err != nil {
			return err
		}
		stagingExists, err := pathPresent(journal.Staging)
		if err != nil {
			return err
		}
		if journal.HadOld {
			if !outputExists {
				backupExists, err := outputDirectoryExists(journal.Backup)
				if err != nil {
					return err
				}
				if !backupExists {
					return errors.New("prepared journal is missing its prior output root")
				}
				if err := os.Rename(journal.Backup, outputRoot); err != nil {
					return err
				}
				if err := syncDirectory(filepath.Dir(outputRoot)); err != nil {
					return err
				}
			} else if !stagingExists {
				return errors.New("prepared journal found an unexpected output root")
			}
		} else if outputExists && stagingExists {
			return errors.New("prepared journal found an unexpected output root")
		}
		if exists, err := pathPresent(journal.Backup); err != nil || exists {
			if err != nil {
				return err
			}
			return errors.New("prepared journal has an unexpected backup")
		}
		if stagingExists {
			if err := removePathIfPresent(journal.Staging); err != nil {
				return err
			}
		}
	case phaseOldMoved:
		backupExists, err := outputDirectoryExists(journal.Backup)
		if err != nil {
			return err
		}
		if !backupExists {
			return errors.New("old-moved journal is missing its backup")
		}
		outputExists, err := outputDirectoryExists(outputRoot)
		if err != nil {
			return err
		}
		stagingExists, err := pathPresent(journal.Staging)
		if err != nil {
			return err
		}
		if outputExists {
			if stagingExists {
				return errors.New("old-moved journal found an unexpected output root")
			}
			if err := removePathIfPresent(journal.Backup); err != nil {
				return err
			}
			break
		}
		if err := os.Rename(journal.Backup, outputRoot); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(outputRoot)); err != nil {
			return err
		}
		if stagingExists {
			if err := removePathIfPresent(journal.Staging); err != nil {
				return err
			}
		}
	case phaseNewInstalled:
		exists, err := outputDirectoryExists(outputRoot)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("new-installed journal is missing its replacement output root")
		}
		if err := removePathIfPresent(journal.Backup); err != nil {
			return err
		}
		if err := removePathIfPresent(journal.Staging); err != nil {
			return err
		}
	default:
		return errors.New("output journal has an unknown phase")
	}
	return removeJournal(path)
}

func validateJournal(outputRoot string, journal replacementJournal) error {
	if journal.Output != outputRoot {
		return errors.New("output journal belongs to a different output root")
	}
	transactionID, err := stagingTransactionID(outputRoot, journal.Staging)
	if err != nil {
		return err
	}
	if journal.Backup != backupPath(outputRoot, transactionID) {
		return errors.New("output journal has an invalid backup path")
	}
	if journal.Phase == phaseOldMoved && !journal.HadOld {
		return errors.New("old-moved journal requires a prior output root")
	}
	return nil
}

func pathPresent(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func removePathIfPresent(path string) error {
	exists, err := pathPresent(path)
	if err != nil || !exists {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("staging tree contains symbolic link %q", filepath.Base(path))
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func writeFailure(operation string, err error) []model.Diagnostic {
	return []model.Diagnostic{{
		Code:     diagnosticWriteFailed,
		Severity: model.SeverityError,
		Message:  operation + ": " + osErrorText(err),
	}}
}

func osErrorText(err error) string {
	switch typed := err.(type) {
	case *os.PathError:
		return osErrorText(typed.Err)
	case *os.LinkError:
		return osErrorText(typed.Err)
	case interface{ Unwrap() []error }:
		errors := typed.Unwrap()
		messages := make([]string, 0, len(errors))
		for _, nested := range errors {
			messages = append(messages, osErrorText(nested))
		}
		return strings.Join(messages, "; ")
	case interface{ Unwrap() error }:
		if nested := typed.Unwrap(); nested != nil {
			return osErrorText(nested)
		}
	}
	return err.Error()
}
