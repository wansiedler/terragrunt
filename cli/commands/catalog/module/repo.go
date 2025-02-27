package module

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gitsight/go-vcsurl"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/hashicorp/go-getter"
	"gopkg.in/ini.v1"
)

const (
	githubHost            = "github.com"
	githubEnterpriseRegex = `^(github\.(.+))$`
	gitlabHost            = "gitlab.com"
	azuredevHost          = "dev.azure.com"
	bitbucketHost         = "bitbucket.org"
	gitlabSelfHostedRegex = `^(gitlab\.(.+))$`

	cloneCompleteSentinel = ".catalog-clone-complete"
)

var (
	gitHeadBranchNameReg    = regexp.MustCompile(`^.*?([^/]+)$`)
	repoNameFromCloneURLReg = regexp.MustCompile(`(?i)^.*?([-a-z_.]+)[^/]*?(?:\.git)?$`)

	modulesPaths = []string{"modules"}

	includedGitFiles = []string{"HEAD", "config"}
)

type Repo struct {
	logger log.Logger

	cloneURL string
	path     string

	RemoteURL  string
	BranchName string

	walkWithSymlinks bool
	useCAS           bool
}

func NewRepo(ctx context.Context, logger log.Logger, cloneURL, tempDir string, walkWithSymlinks bool, allowCAS bool) (*Repo, error) {
	useCAS := false

	if strings.HasPrefix(cloneURL, "cas://") {
		cloneURL = strings.TrimPrefix(cloneURL, "cas://")

		if !allowCAS {
			return nil, errors.Errorf("cas:// protocol is not allowed without using the `cas` experiment. Please enable the experiment and try again.")
		}

		useCAS = true
	}

	repo := &Repo{
		logger:           logger,
		cloneURL:         cloneURL,
		path:             tempDir,
		walkWithSymlinks: walkWithSymlinks,
		useCAS:           useCAS,
	}

	if err := repo.clone(ctx); err != nil {
		return nil, err
	}

	if err := repo.parseRemoteURL(); err != nil {
		return nil, err
	}

	if err := repo.parseBranchName(); err != nil {
		return nil, err
	}

	return repo, nil
}

// FindModules clones the repository if `repoPath` is a URL, searches for Terragrunt modules, indexes their README.* files, and returns module instances.
func (repo *Repo) FindModules(ctx context.Context) (Modules, error) {
	var modules Modules

	// check if root repo path is a module dir
	if module, err := NewModule(repo, ""); err != nil {
		return nil, err
	} else if module != nil {
		modules = append(modules, module)
	}

	for _, modulesPath := range modulesPaths {
		modulesPath = filepath.Join(repo.path, modulesPath)

		if !files.FileExists(modulesPath) {
			continue
		}

		walkFunc := filepath.Walk
		if repo.walkWithSymlinks {
			walkFunc = util.WalkWithSymlinks
		}

		err := walkFunc(modulesPath,
			func(dir string, remote os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !remote.IsDir() {
					return nil
				}

				moduleDir, err := filepath.Rel(repo.path, dir)
				if err != nil {
					return errors.New(err)
				}

				if module, err := NewModule(repo, moduleDir); err != nil {
					return err
				} else if module != nil {
					modules = append(modules, module)
				}

				return nil
			})
		if err != nil {
			return nil, err
		}
	}

	return modules, nil
}

var githubEnterprisePatternReg = regexp.MustCompile(githubEnterpriseRegex)
var gitlabSelfHostedPatternReg = regexp.MustCompile(gitlabSelfHostedRegex)

// ModuleURL returns the URL of the module in this repository. `moduleDir` is the path from the repository root.
func (repo *Repo) ModuleURL(moduleDir string) (string, error) {
	if repo.RemoteURL == "" {
		return filepath.Join(repo.path, moduleDir), nil
	}

	remote, err := vcsurl.Parse(repo.RemoteURL)
	if err != nil {
		return "", errors.New(err)
	}

	// Simple, predictable hosts
	switch remote.Host {
	case githubHost:
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	case gitlabHost:
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	case bitbucketHost:
		return fmt.Sprintf("https://%s/%s/browse/%s?at=%s", remote.Host, remote.FullName, moduleDir, repo.BranchName), nil
	case azuredevHost:
		return fmt.Sprintf("https://%s/_git/%s?path=%s&version=GB%s", remote.Host, remote.FullName, moduleDir, repo.BranchName), nil
	}

	// // Hosts that require special handling
	if githubEnterprisePatternReg.MatchString(string(remote.Host)) {
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	}

	if gitlabSelfHostedPatternReg.MatchString(string(remote.Host)) {
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	}

	return "", errors.Errorf("hosting: %q is not supported yet", remote.Host)
}

type CloneOptions struct {
	SourceURL  string
	TargetPath string
	Context    context.Context
	Logger     log.Logger
}

func (repo *Repo) clone(ctx context.Context) error {
	cloneURL, err := repo.resolveCloneURL()
	if err != nil {
		return err
	}

	// Handle local directory case
	if files.IsDir(cloneURL) {
		return repo.handleLocalDir(cloneURL)
	}

	// Prepare clone options
	opts := CloneOptions{
		SourceURL:  cloneURL,
		TargetPath: repo.path,
		Context:    ctx,
		Logger:     repo.logger,
	}

	if err := repo.prepareCloneDirectory(&opts); err != nil {
		return err
	}

	return repo.performClone(&opts)
}

func (repo *Repo) resolveCloneURL() (string, error) {
	if repo.cloneURL == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", errors.New(err)
		}
		return currentDir, nil
	}

	return repo.cloneURL, nil
}

func (repo *Repo) handleLocalDir(repoPath string) error {
	if !filepath.IsAbs(repoPath) {
		absRepoPath, err := filepath.Abs(repoPath)
		if err != nil {
			return errors.New(err)
		}
		repo.logger.Debugf("Converting relative path %q to absolute %q", repoPath, absRepoPath)
		repo.path = absRepoPath

		return nil
	}

	repo.path = repoPath

	return nil
}

func (repo *Repo) prepareCloneDirectory(opts *CloneOptions) error {
	if err := os.MkdirAll(opts.TargetPath, os.ModePerm); err != nil {
		return errors.New(err)
	}

	repoName := repo.extractRepoName()
	repo.path = filepath.Join(opts.TargetPath, repoName)

	// Clean up incomplete clones
	if repo.shouldCleanupIncompleteClone() {
		repo.logger.Debugf("The repo dir exists but %q does not. Removing the repo dir for cloning from the remote source.", cloneCompleteSentinel)
		if err := os.RemoveAll(repo.path); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func (repo *Repo) extractRepoName() string {
	repoName := "temp"
	if match := repoNameFromCloneURLReg.FindStringSubmatch(repo.cloneURL); len(match) > 0 && match[1] != "" {
		repoName = match[1]
	}

	return repoName
}

func (repo *Repo) shouldCleanupIncompleteClone() bool {
	return files.FileExists(repo.path) && !files.FileExists(filepath.Join(repo.path, cloneCompleteSentinel))
}

func (repo *Repo) performClone(opts *CloneOptions) error {
	if repo.useCAS {
		c, err := cas.New(opts.SourceURL, cas.Options{
			Dir:              repo.path,
			IncludedGitFiles: includedGitFiles,
		})
		if err != nil {
			return err
		}

		if err := c.Clone(); err != nil {
			return err
		}

		// Create the sentinel file to indicate that the clone is complete
		f, err := os.Create(filepath.Join(repo.path, cloneCompleteSentinel))
		if err != nil {
			return errors.New(err)
		}

		f.Close()

		return nil
	}

	sourceURL, err := tf.ToSourceURL(opts.SourceURL, "")
	if err != nil {
		return err
	}

	repo.cloneURL = sourceURL.String()
	opts.Logger.Infof("Cloning repository %q to temporary directory %q", repo.cloneURL, repo.path)

	// Add HEAD reference to avoid pathspec error
	sourceURL.RawQuery = (url.Values{"ref": []string{"HEAD"}}).Encode()

	if err := getter.Get(
		repo.path,
		strings.Trim(sourceURL.String(), "/"),
		getter.WithContext(opts.Context),
		getter.WithMode(getter.ClientModeDir),
	); err != nil {
		return err
	}

	// Create the sentinel file to indicate that the clone is complete
	f, err := os.Create(filepath.Join(repo.path, cloneCompleteSentinel))
	if err != nil {
		return errors.New(err)
	}

	f.Close()

	return nil
}

// parseRemoteURL reads the git config `.git/config` and parses the first URL of the remote URLs, the remote name "origin" has the highest priority.
func (repo *Repo) parseRemoteURL() error {
	gitConfigPath := filepath.Join(repo.path, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return errors.Errorf("the specified path %q is not a git repository (no .git/config file found)", repo.path)
	}

	repo.logger.Debugf("Parsing git config %q", gitConfigPath)

	inidata, err := ini.Load(gitConfigPath)
	if err != nil {
		return errors.New(err)
	}

	var sectionName string

	for _, name := range inidata.SectionStrings() {
		if !strings.HasPrefix(name, "remote") {
			continue
		}

		sectionName = name

		if sectionName == `remote "origin"` {
			break
		}
	}

	// no git remotes found
	if sectionName == "" {
		return nil
	}

	repo.RemoteURL = inidata.Section(sectionName).Key("url").String()
	repo.logger.Debugf("Remote url: %q for repo: %q", repo.RemoteURL, repo.path)

	return nil
}

func (repo *Repo) gitHeadfile() string {
	return filepath.Join(repo.path, ".git", "HEAD")
}

// parseBranchName reads `.git/HEAD` file and parses a branch name.
func (repo *Repo) parseBranchName() error {
	data, err := files.ReadFileAsString(repo.gitHeadfile())
	if err != nil {
		return errors.Errorf("the specified path %q is not a git repository (no .git/HEAD file found)", repo.path)
	}

	if match := gitHeadBranchNameReg.FindStringSubmatch(data); len(match) > 0 {
		repo.BranchName = strings.TrimSpace(match[1])

		return nil
	}

	return errors.Errorf("could not get branch name for repo %q", repo.path)
}
