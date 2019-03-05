package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/pathutil"
)

var testGitRepoPath = ""

func createDummyFiles(destDir string, filePaths []string) error {
	for _, filePath := range filePaths {
		pth := filepath.Join(destDir, filePath)
		if err := os.MkdirAll(filepath.Dir(pth), 0777); err != nil {
			return err
		}
		if err := ioutil.WriteFile(pth, []byte("test"), 0777); err != nil {
			return err
		}
	}
	return nil
}

func TestMain(m *testing.M) {
	//create git stuff here
	var err error
	testGitRepoPath, err = pathutil.NormalizedOSTempDirPath("test-git-repo")
	if err != nil {
		failf("failed to create temp dir, error: %s", err)
	}

	if err := command.New("git", "init").SetDir(testGitRepoPath).Run(); err != nil {
		failf("failed to git init, error: %s", err)
	}

	androidConfigPath := filepath.Join(testGitRepoPath, "dummy-file")

	for i, tag := range []string{"0.1.0", "0.1.1", "pre-release", "0.2.0", "0.2.1", "1.0.0", "1.0.1"} {
		if err := ioutil.WriteFile(androidConfigPath, []byte(fmt.Sprintf("%d", i)), 0644); err != nil {
			failf("failed to write file, error: %s", err)
		}

		if err := command.New("git", "add", ".").SetDir(testGitRepoPath).Run(); err != nil {
			failf("failed to git init, error: %s", err)
		}

		if err := command.New("git", "commit", "-m", "commit message").SetDir(testGitRepoPath).Run(); err != nil {
			failf("failed to git init, error: %s", err)
		}

		if err := command.New("git", "tag", tag).SetDir(testGitRepoPath).Run(); err != nil {
			failf("failed to git init, error: %s", err)
		}
	}

	os.Exit(m.Run())
}

func Test_detectPlatform(t *testing.T) {
	tempDir, err := pathutil.NormalizedOSTempDirPath("test")
	if err != nil {
		t.Fatal(err)
	}
	androidConfigPath := filepath.Join(tempDir, "android-config.yml")
	iosConfigPath := filepath.Join(tempDir, "ios-config.yml")

	if err := ioutil.WriteFile(androidConfigPath, []byte("gcloud:\n  app: ./my-android-app.apk\n  test: ./my-android-test-app.apk\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(iosConfigPath, []byte("gcloud:\n  test: ./my-android-app.apk\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		configYMLPath string
		want          string
		wantErr       bool
	}{
		{name: "android-config", want: "android", wantErr: false, configYMLPath: androidConfigPath},
		{name: "ios-config", want: "ios", wantErr: false, configYMLPath: iosConfigPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectPlatform(tt.configYMLPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectPlatform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("detectPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseGitTags(t *testing.T) {
	gitOutput := `22fe7ea7519a32883fe498fc4d58d059b3af8c7f	refs/tags/0.1.0
983b4dd9b7474d2b5fb7ee5561c85b07087a0d40	refs/tags/0.1.1
22fe7ea7519a32883fe498fc4d58d059b3af8c7f	refs/tags/1.1.0
983b4dd9b7474d2b5fb7ee5561c85b07087a0d40	refs/tags/0.1.11
sample disturbing log
983b4dd9b7474d2b5fb7ee5561c85b07087a0d40	refs/tags/0.1.12`

	tests := []struct {
		name         string
		gitOutput    string
		wantVersions []string
	}{
		{name: "parse-git-tags", gitOutput: gitOutput, wantVersions: []string{"0.1.0", "0.1.1", "1.1.0", "0.1.11", "0.1.12"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotVersions := parseGitTags(tt.gitOutput); !reflect.DeepEqual(gotVersions, tt.wantVersions) {
				t.Errorf("parseGitTags() = %v, want %v", gotVersions, tt.wantVersions)
			}
		})
	}
}

func Test_findLatestVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     string
	}{
		{name: "find-latest", want: "1.1.0", versions: []string{"0.1.0", "0.1.1", "pre-release", "1.1.0", "0.1.11"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findLatestVersion(tt.versions); got != tt.want {
				t.Errorf("findLatestVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getLatestVersion(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		want    string
		wantErr bool
	}{
		{name: "git-latest", repoURL: filepath.Join(testGitRepoPath, ".git"), want: "1.0.1", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getLatestVersion(tt.repoURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("getLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getLatestVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDownloadURLbyVersion(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		version string
		want    string
		wantErr bool
	}{
		{name: "git-latest", repoURL: filepath.Join(testGitRepoPath, ".git"), want: "https://github.com/TestArmada/flank/releases/download/v1.0.1/flank.jar", wantErr: false, version: "latest"},
		{name: "git-custom", repoURL: filepath.Join(testGitRepoPath, ".git"), want: "https://github.com/TestArmada/flank/releases/download/v1.0.0/flank.jar", wantErr: false, version: "v1.0.0"},
		{name: "git-custom-non-versioned", repoURL: filepath.Join(testGitRepoPath, ".git"), want: "https://github.com/TestArmada/flank/releases/download/pre-release/flank.jar", wantErr: false, version: "pre-release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDownloadURLbyVersion(tt.repoURL, tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDownloadURLbyVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getDownloadURLbyVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_exportArtifacts(t *testing.T) {
	srcDir, err := pathutil.NormalizedOSTempDirPath("test-src")
	if err != nil {
		t.Fatal(err)
	}
	destDir, err := pathutil.NormalizedOSTempDirPath("test-dest")
	if err != nil {
		t.Fatal(err)
	}

	if err := createDummyFiles(srcDir, []string{
		"result-dir-1/res1-file1",
		"result-dir-1/res1-file2",
		"result-dir-1/res1-file3",
		"result-dir-1/res1-ont-include/res1-dont-include-file1",
		"result-dir-1/res1-dont-include/res1-dont-include-file2",
		"result-dir-1/res1-dont-include/res1-dont-include-file3",
		"result-dir-2/res2-file1",
		"result-dir-2/res2-file2",
		"result-dir-2/res2-file3",
		"result-dir-2/res2-dont-include/res2-dont-include-file1",
		"result-dir-2/res2-dont-include/res2-dont-include-file2",
		"result-dir-2/res2-dont-include/res2-dont-include-file3",
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		srcDir  string
		destDir string
		wantErr bool
	}{
		{name: "deploy", srcDir: srcDir, destDir: destDir, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := exportArtifacts(tt.srcDir, tt.destDir, nil); (err != nil) != tt.wantErr {
				t.Errorf("exportArtifacts() error = %v, wantErr %v", err, tt.wantErr)
			}
			fi, err := os.Open(tt.destDir)
			if err != nil {
				t.Fatal(err)
			}

			files, err := fi.Readdir(-1)
			if err != nil {
				t.Fatal(err)
			}

			expectedFileNames := []string{"res2-file1", "res2-file2", "res2-file3"}

			if len(files) != len(expectedFileNames) {
				t.Fatal("not all result file copied from result-dir-1 dir")
			}

			for i, file := range files {
				if file.IsDir() {
					t.Fatal("exported artifact cannot be dir")
				}

				// match the names except the last number
				if file.Name()[:len(file.Name())-1] != expectedFileNames[i][:len(expectedFileNames[i])-1] {
					t.Fatal("exported wrong artifact:", file.Name()[:len(file.Name())-1], expectedFileNames[i][:len(expectedFileNames[i])-1])
				}
			}
		})
	}
}
