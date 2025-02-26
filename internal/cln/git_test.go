package cln_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitRunner_LsRemote(t *testing.T) {
	t.Parallel()
	runner := cln.NewGitRunner()

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()
		results, err := runner.LsRemote("https://github.com/gruntwork-io/terragrunt.git", "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Regexp(t, "^[0-9a-f]{40}$", results[0].Hash)
		assert.Equal(t, "HEAD", results[0].Ref)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()
		_, err := runner.LsRemote("https://github.com/nonexistent/repo.git", "HEAD")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrCommandSpawn)
	})

	t.Run("nonexistent reference", func(t *testing.T) {
		t.Parallel()
		_, err := runner.LsRemote("https://github.com/gruntwork-io/terragrunt.git", "nonexistent-branch")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrNoMatchingReference)
	})
}

func TestGitRunner_Clone(t *testing.T) {
	t.Parallel()

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()
		cloneDir := t.TempDir()
		runner := cln.NewGitRunner().WithWorkDir(cloneDir)
		err := runner.Clone("https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Verify it's a git repository
		_, err = os.Stat(filepath.Join(cloneDir, "HEAD"))
		require.NoError(t, err)
	})

	t.Run("clone without workdir fails", func(t *testing.T) {
		t.Parallel()
		runner := cln.NewGitRunner()
		err := runner.Clone("https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrNoWorkDir)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()
		cloneDir := t.TempDir()
		runner := cln.NewGitRunner().WithWorkDir(cloneDir)
		err := runner.Clone("https://github.com/gruntwork-io/terragrunt-fake.git", false, 1, "")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrGitClone)
	})
}

func TestCreateTempDir(t *testing.T) {
	t.Parallel()
	git := cln.NewGitRunner()
	dir, cleanup, err := git.CreateTempDir()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cleanup())
	})

	// Verify directory exists
	_, err = os.Stat(dir)
	require.NoError(t, err)

	// Verify it's empty
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestGetRepoName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		repo string
		want string
	}{
		{
			name: "simple repo",
			repo: "https://github.com/user/repo.git",
			want: "repo",
		},
		{
			name: "no .git suffix",
			repo: "https://github.com/user/repo",
			want: "repo",
		},
		{
			name: "with path",
			repo: "/path/to/repo.git",
			want: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cln.GetRepoName(tt.repo))
		})
	}
}

func TestGitRunner_LsTree(t *testing.T) {
	t.Parallel()

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()
		cloneDir := t.TempDir()
		runner := cln.NewGitRunner().WithWorkDir(cloneDir)

		// First clone a repository
		err := runner.Clone("https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Then try to ls-tree HEAD
		tree, err := runner.LsTree("HEAD", ".")
		require.NoError(t, err)
		require.NotEmpty(t, tree.Entries())

		// Verify some common files exist in the tree
		found := false
		for _, entry := range tree.Entries() {
			if entry.Path == "README.md" {
				found = true
				assert.Equal(t, "blob", entry.Type)
				assert.Equal(t, "100644", entry.Mode)
				assert.Regexp(t, "^[0-9a-f]{40}$", entry.Hash)
				break
			}
		}
		assert.True(t, found, "README.md should exist in the repository")
	})

	t.Run("ls-tree without workdir fails", func(t *testing.T) {
		t.Parallel()
		runner := cln.NewGitRunner()
		_, err := runner.LsTree("HEAD", ".")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrNoWorkDir)
	})

	t.Run("invalid reference", func(t *testing.T) {
		t.Parallel()
		cloneDir := t.TempDir()
		runner := cln.NewGitRunner().WithWorkDir(cloneDir)

		// First clone a repository
		err := runner.Clone("https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Try to ls-tree an invalid reference
		_, err = runner.LsTree("nonexistent", ".")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrReadTree)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()
		runner := cln.NewGitRunner().WithWorkDir(t.TempDir())

		// Try to ls-tree in an empty directory
		_, err := runner.LsTree("HEAD", ".")
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrReadTree)
	})
}

func TestGitRunner_RequiresWorkDir(t *testing.T) {
	t.Parallel()

	t.Run("with workdir", func(t *testing.T) {
		t.Parallel()
		runner := cln.NewGitRunner().WithWorkDir(t.TempDir())
		err := runner.RequiresWorkDir()
		assert.NoError(t, err)
	})

	t.Run("without workdir", func(t *testing.T) {
		t.Parallel()
		runner := cln.NewGitRunner()
		err := runner.RequiresWorkDir()
		require.Error(t, err)
		var wrappedErr *cln.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cln.ErrNoWorkDir)
	})
}
