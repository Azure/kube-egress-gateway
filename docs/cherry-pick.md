# Cherry-Picking Commits to Release Branches

This document describes the process for cherry-picking commits (such as CVE fixes, critical bug fixes, or important features) from the `main` branch to release branches like `release/0.0.x`.

## Automated Cherry-Pick Process

The repository includes an automated cherry-pick workflow that simplifies backporting changes to release branches.

### Using Labels for Automated Cherry-Pick

1. **Merge your PR to main**: First, get your pull request merged to the `main` branch as usual.

2. **Add a cherry-pick label**: After the PR is merged, add a label to indicate which release branch(es) should receive the changes:
   - For release/0.0.x: Add label `cherry-pick/release-0.0.x`
   - For release/1.0.x: Add label `cherry-pick/release-1.0.x`
   - Multiple labels can be added to cherry-pick to multiple branches

3. **Automatic PR creation**: The GitHub Actions workflow will automatically:
   - Cherry-pick the merge commit to the target branch(es)
   - Create a new PR with the cherry-picked changes
   - Label the PR with `backport` and `cherry-pick`
   - Link back to the original PR in the description

4. **Review and merge**: Review the automatically created PR and merge it if everything looks good.

### Handling Conflicts

If the automated cherry-pick encounters conflicts:

1. The workflow will create an issue with detailed instructions for manual cherry-pick
2. The issue will include the exact git commands needed to resolve the conflict
3. Follow the instructions in the issue to manually complete the cherry-pick
4. The issue will be labeled with `cherry-pick-conflict`

## Manual Cherry-Pick Process

If you prefer to cherry-pick manually or need to handle complex scenarios:

### Step 1: Identify the Commit

Find the merge commit SHA from the merged PR on main:

```bash
# Option 1: From the GitHub UI - copy the merge commit SHA from the merged PR

# Option 2: From command line
git log --oneline --merges main | grep "Merge pull request #<PR_NUMBER>"
```

### Step 2: Create a Cherry-Pick Branch

```bash
# Fetch the latest release branch
git fetch origin release/0.0.x

# Create a new branch from the release branch
git checkout -b cherry-pick-<description>-to-release-0.0.x origin/release/0.0.x
```

### Step 3: Cherry-Pick the Commit

```bash
# Cherry-pick with -x flag to add reference to original commit
# Use -m 1 for merge commits (parent 1 is the main branch)
git cherry-pick -x -m 1 <merge-commit-sha>
```

### Step 4: Handle Conflicts (if any)

If conflicts occur:

```bash
# Fix conflicts in your editor
# Mark as resolved
git add <conflicted-files>

# Continue cherry-pick
git cherry-pick --continue
```

### Step 5: Push and Create PR

```bash
# Push to remote
git push origin cherry-pick-<description>-to-release-0.0.x

# Create PR using GitHub CLI
gh pr create \
  --base release/0.0.x \
  --head cherry-pick-<description>-to-release-0.0.x \
  --title "[release-0.0.x] <original-pr-title>" \
  --body "Cherry-pick of #<original-pr-number> to release/0.0.x"
```

## Best Practices

### When to Cherry-Pick

Cherry-pick commits to release branches when:

- **Security fixes (CVE)**: Always backport security vulnerabilities to supported releases
- **Critical bug fixes**: Bugs that affect production users on release branches
- **Important features**: Features that were committed to be delivered in a patch release
- **Documentation fixes**: Critical documentation updates

### When NOT to Cherry-Pick

Avoid cherry-picking:

- **Breaking changes**: Changes that break backward compatibility
- **Large refactors**: Extensive code restructuring that could introduce instability
- **New features** (for patch releases): Unless explicitly planned for the release
- **Experimental features**: Features that haven't been thoroughly tested

### Label Conventions

Use these labels to communicate intent:

- `cherry-pick/release-X.Y.x`: Automated cherry-pick to specific branch
- `backport`: Mark PRs that are backports from main
- `cherry-pick-conflict`: Indicates manual intervention needed
- `security`: For CVE fixes and security patches
- `priority/critical`: For urgent fixes that should be released quickly

## Release Branch Naming

Release branches follow the pattern: `release/X.Y.x` where:
- `X` is the major version
- `Y` is the minor version  
- `x` indicates this branch will receive patch releases

Examples:
- `release/0.0.x` - receives patch releases like v0.0.1, v0.0.2, etc.
- `release/1.0.x` - receives patch releases like v1.0.1, v1.0.2, etc.

## Creating a New Release Branch

When starting a new minor version release:

```bash
# Create release branch from main at the appropriate commit
git checkout main
git pull origin main
git checkout -b release/X.Y.x
git push origin release/X.Y.x

# Protect the branch in GitHub settings
# Settings → Branches → Add branch protection rule
```

## Troubleshooting

### Cherry-pick workflow not triggering

- Ensure the PR is merged (not just closed)
- Verify the label name exactly matches `cherry-pick/release-X.Y.x`
- Check GitHub Actions permissions are enabled for the repository

### "Branch does not exist" error

The target release branch must exist before cherry-picking. Create it following the instructions above.

### Complex merge conflicts

For complex conflicts:
1. Consider whether the change should be backported
2. If necessary, create a simplified version of the fix specifically for the release branch
3. Ensure the backported fix receives the same level of testing as the original

## Examples

### Example 1: CVE Fix

```bash
# 1. Fix merged to main as PR #123
# 2. Add label "cherry-pick/release-0.0.x" to PR #123
# 3. Automated workflow creates PR to release/0.0.x
# 4. Review and merge the backport PR
# 5. Tag a new patch release (e.g., v0.0.5)
```

### Example 2: Critical Bug Fix to Multiple Releases

```bash
# 1. Bug fix merged to main as PR #456
# 2. Add labels:
#    - "cherry-pick/release-0.0.x"
#    - "cherry-pick/release-1.0.x"
# 3. Automated workflow creates two PRs (one for each branch)
# 4. Review and merge both backport PRs
# 5. Tag new patch releases on both branches
```

### Example 3: Manual Cherry-Pick with Conflicts

```bash
git fetch origin release/0.0.x
git checkout -b cherry-pick-789-to-release-0.0.x origin/release/0.0.x
git cherry-pick -x -m 1 abc123def456

# Conflicts in file.go
# Edit file.go to resolve conflicts

git add file.go
git cherry-pick --continue
git push origin cherry-pick-789-to-release-0.0.x

gh pr create \
  --base release/0.0.x \
  --head cherry-pick-789-to-release-0.0.x \
  --title "[release-0.0.x] Fix critical memory leak" \
  --body "Cherry-pick of #789 to release/0.0.x with conflict resolution"
```

## Questions?

For questions about the cherry-pick process or release management, please:
- Open an issue with label `question`
- Reach out to the maintainers
- Check existing issues labeled `cherry-pick-conflict` for examples
