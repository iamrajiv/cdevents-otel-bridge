# Pull Request

## Description

Please include a summary of the changes and which issue is fixed. Include relevant motivation and context.

Fixes # (issue)

## Type of Change

Please delete options that are not relevant.

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Performance improvement
- [ ] Code refactoring
- [ ] Test improvement
- [ ] CI/CD improvement
- [ ] Other (please describe):

## Changes Made

List the main changes in this PR:

- Change 1
- Change 2
- Change 3

## How Has This Been Tested?

Please describe the tests that you ran to verify your changes. Provide instructions so we can reproduce.

- [ ] Unit tests
- [ ] Integration tests
- [ ] Manual testing
- [ ] End-to-end tests

**Test Configuration**:
- Go version:
- Redis version:
- OTel Collector version:
- Platform:

**Test Steps:**
1. Step 1
2. Step 2
3. Step 3

## Test Coverage

```bash
# Paste test coverage results
make test-coverage
```

Current coverage: X%
Previous coverage: Y%
Change: +/- Z%

## Screenshots (if applicable)

Add screenshots to demonstrate the changes.

## Breaking Changes

If this PR introduces breaking changes, describe them here:

- Breaking change 1: Description and migration guide
- Breaking change 2: Description and migration guide

## Documentation

- [ ] Updated README.md (if needed)
- [ ] Updated docs/ (if needed)
- [ ] Updated API documentation (if API changes)
- [ ] Updated configuration guide (if config changes)
- [ ] Updated CHANGELOG.md
- [ ] Added/updated code comments
- [ ] Added/updated examples

## Checklist

Before submitting your PR, please review and check the following:

### Code Quality

- [ ] My code follows the style guidelines of this project
- [ ] I have performed a self-review of my code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] My changes generate no new warnings
- [ ] I have run `make lint` and fixed all issues
- [ ] I have run `make fmt` to format my code

### Testing

- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
- [ ] I have added integration tests (if applicable)
- [ ] Test coverage has not decreased

### Documentation

- [ ] I have made corresponding changes to the documentation
- [ ] I have updated the CHANGELOG.md
- [ ] I have added examples demonstrating the new functionality (if applicable)

### Commit Messages

- [ ] My commits follow the [Conventional Commits](https://www.conventionalcommits.org/) specification
- [ ] My commit messages are clear and descriptive

### Dependencies

- [ ] I have not added new dependencies (or I have discussed it in the issue)
- [ ] I have updated go.mod and go.sum (if dependencies changed)
- [ ] All dependencies are compatible with the license

## Additional Notes

Add any other context about the PR here.

## Reviewer Notes

Notes for reviewers:

- Areas that need special attention:
- Questions for reviewers:
- Known limitations:

## Related Issues/PRs

- Related to #issue-number
- Depends on #pr-number
- Blocks #issue-number

## Deployment Notes

Notes for deployment (if applicable):

- Configuration changes required:
- Migration steps:
- Rollback plan:

---

**By submitting this pull request, I confirm that my contribution is made under the terms of the Apache 2.0 license.**
