package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/bitrise/tools/timeoutcmd"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/hashicorp/go-version"
	"github.com/kballard/go-shellquote"
	"gopkg.in/yaml.v2"
)

const (
	platformAndroid = "android"
	platformIos     = "ios"
	baseURL         = "https://github.com/Flank/flank"
)

type config struct {
	ServiceAccountJSON stepconf.Secret `env:"google_service_account_json,required"`
	ConfigPath         string          `env:"config_path,file"`
	Version            string          `env:"version,required"`
	CommandFlags       string          `env:"command_flags"`
}

// returns android if there is an app field under gcloud in the config yml
func detectPlatform(configYMLPath string) (string, error) {
	var androidApp struct {
		Gcloud struct {
			App string `yaml:"app"`
		} `yaml:"gcloud"`
	}

	ymlBytes, err := fileutil.ReadBytesFromFile(configYMLPath)
	if err != nil {
		return "", err
	}

	if err := yaml.Unmarshal(ymlBytes, &androidApp); err != nil {
		return "", err
	}

	if len(androidApp.Gcloud.App) > 0 {
		return platformAndroid, nil
	}
	return platformIos, nil
}

// stores string under a temp path and exports the path to the corresponding env
func storeCredentials(cred string) error {
	tmpPth, err := pathutil.NormalizedOSTempDirPath("credential")
	if err != nil {
		return err
	}
	pth := filepath.Join(tmpPth, "cred.json")
	if err := fileutil.WriteStringToFile(pth, cred); err != nil {
		return err
	}
	return os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", pth)
}

func parseGitTags(gitOutput string) (versions []string) {
	for _, line := range strings.Split(gitOutput, "\n") {
		if ref := strings.Split(line, "\t"); len(ref) == 2 {
			versions = append(versions, strings.TrimPrefix(ref[1], "refs/tags/"))
		}
	}
	return
}

func findLatestVersion(versions []string) string {
	lastVersionStr := ""
	var lastVersion *version.Version
	for _, v := range versions {
		candidate, err := version.NewVersion(v)
		if err != nil {
			continue
		}

		if lastVersionStr == "" || candidate.GreaterThan(lastVersion) {
			lastVersionStr = v
			lastVersion = candidate
		}
	}

	return lastVersionStr
}

// gets the tags list, splits the lines per tab and finds the prefix-truncated version strings
// if the version string is a valid semver version then this function returns the latest one
func getLatestVersion(repoURL string) (string, error) {
	cmd := command.New("git", "ls-remote", "--tags", "--quiet", repoURL)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run git command, error: %s, output: %s", err, out)
	}

	versions := parseGitTags(out)
	lastVersion := findLatestVersion(versions)

	if lastVersion == "" {
		return "", fmt.Errorf("unable to find latest version")
	}

	return lastVersion, nil
}

// if input version is latest then it returns the fetched latest release version download url otherwise
// returns the release version download url for the given version
func getDownloadURLbyVersion(repoURL, version string) (string, error) {
	if version == "latest" {
		var err error
		if version, err = getLatestVersion(repoURL); err != nil {
			return "", err
		}
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
	}
	return fmt.Sprintf("%s/releases/download/%s/flank.jar", baseURL, version), nil
}

// downloads file from an url to a temp location and returns the full path for the file
func download(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warnf("Failed to close response body, error: %s", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http GET %s non success status code: %d", url, resp.StatusCode)
	}

	tmpPath, err := pathutil.NormalizedOSTempDirPath("flank-bin")
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(tmpPath, "flank.jar")

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return binPath, fileutil.WriteBytesToFile(binPath, bodyData)
}

func failf(format string, args ...interface{}) {
	log.Errorf(format, args...)
	os.Exit(1)
}

// lists all dirs inside of ./results dir and selects the latest(by modtime)
// then all the files in the root will be copied from this dir to the root of the dir under BITRISE_DEPLOY_DIR
func exportArtifacts(srcDir, destDir string, copiedHandler func(src, dest string)) error {
	fInfs, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}

	var latestDir string
	var latestModtime time.Time

	for _, fInf := range fInfs {
		if !fInf.IsDir() {
			continue
		}
		if !fInf.ModTime().Before(latestModtime) {
			latestModtime = fInf.ModTime()
			latestDir = filepath.Join(srcDir, fInf.Name())
		}
	}

	fInfs, err = ioutil.ReadDir(latestDir)
	if err != nil {
		return err
	}

	for _, fInf := range fInfs {
		if fInf.IsDir() {
			continue
		}

		srcFile := filepath.Join(latestDir, fInf.Name())
		destinationFile := filepath.Join(destDir, fInf.Name())

		data, err := ioutil.ReadFile(srcFile)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(destinationFile, data, 0644); err != nil {
			return err
		}

		if copiedHandler != nil {
			copiedHandler(srcFile, destinationFile)
		}
	}

	return nil
}

func logExitStatus(exitStatus int) {
	statusCodes := map[int]string{
		1:  "A general failure occurred. Possible causes include: a filename that does not exist or an HTTP/network error.",
		2:  "Usually indicates missing or wrong usage of flags, incorrect parameters, errors in config files.",
		10: "At least one matrix not finished (usually a FTL internal error) or unexpected error occurred.",
		15: "Firebase Test Lab could not determine if the test matrix passed or failed, because of an unexpected error.",
		18: "The test environment for this test execution is not supported because of incompatible test dimensions. This error might occur if the selected Android API level is not supported by the selected device type.",
		19: "The test matrix was canceled by the user.",
		20: "A test infrastructure error occurred.",
	}

	if value, ok := statusCodes[exitStatus]; ok {
		log.Warnf("Flank exited with status %d: %s", exitStatus, value)
	}
}

func main() {
	//
	// configuration
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(cfg)
	fmt.Println()

	//
	// tool setup
	log.Infof("Downloading binary")
	downloadURL, err := getDownloadURLbyVersion(baseURL, cfg.Version)
	if err != nil {
		failf("Failed to get download URL, error: %s", err)
	}

	binaryPath, err := download(downloadURL)
	if err != nil {
		failf("Failed to download binary, error: %s", err)
	}

	log.Donef("- Done")
	fmt.Println()

	// string credentials
	if err := storeCredentials(string(cfg.ServiceAccountJSON)); err != nil {
		failf("Failed to store credential file, error: %s", err)
	}

	//
	// running the tool
	log.Infof("Running test")
	platform, err := detectPlatform(cfg.ConfigPath)
	if err != nil {
		failf("Failed to detect platform, error: %s", err)
	}
	log.Printf("- Detected platform: %s", platform)

	commandFlags, err := shellquote.Split(cfg.CommandFlags)
	if err != nil {
		failf("Failed to split command flags, error: %s", err)
	}

	fmt.Println()
	command := command.New("java", append([]string{"-jar", binaryPath, platform, "run", "-c", cfg.ConfigPath}, commandFlags...)...).
		SetStdin(os.Stdin).
		SetStdout(os.Stdout).
		SetStderr(os.Stderr)

	log.Donef("$ %s", command.PrintableCommandArgs())
	fmt.Println()

	cmdErr := command.Run()

	fmt.Println()
	logExitStatus(timeoutcmd.ExitStatus(cmdErr))
	fmt.Println()

	//
	// exporting generated artifacts

	log.Infof("Exporting artifacts")
	if err := exportArtifacts("./results", os.Getenv("BITRISE_DEPLOY_DIR"),
		func(src, dest string) {
			log.Printf("- copied: %s -> %s", src, dest)
		},
	); err != nil {
		failf("Failed to export artifacts, error: %s", err)
	}
	log.Donef("- Done")

	os.Exit(timeoutcmd.ExitStatus(cmdErr))
}
