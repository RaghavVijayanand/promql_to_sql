# Contributing to PromQL to ClickHouse SQL Transpiler

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Code of Conduct

Be respectful, professional, and inclusive. We welcome contributions from everyone.

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in Issues
2. If not, create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - Version information
   - Sample PromQL query that causes the issue

### Suggesting Features

1. Check if the feature has been suggested
2. Create a new issue describing:
   - The problem it solves
   - Proposed solution
   - Alternative solutions considered
   - Impact on existing functionality

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/your-feature-name`
3. Make your changes following the code style guidelines
4. Add or update tests as needed
5. Ensure all tests pass: `make test`
6. Update documentation if needed
7. Commit with clear messages
8. Push to your fork
9. Create a Pull Request

## Development Setup

See [DEVELOPMENT.md](docs/DEVELOPMENT.md) for detailed setup instructions.

Quick start:
```bash
git clone <your-fork>
cd transpiler
make deps
make build
make test
```

## Code Style

- Follow Go best practices
- Run `gofmt` before committing
- Add comments for exported functions
- Keep functions focused and small
- Write meaningful variable names

## Testing

- Write unit tests for new functionality
- Ensure existing tests pass
- Aim for high test coverage
- Include both positive and negative test cases

Example test:
```go
func TestNewFeature(t *testing.T) {
    // Arrange
    input := "test input"
    
    // Act
    result, err := NewFeature(input)
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

## Commit Messages

- Use present tense: "Add feature" not "Added feature"
- Be descriptive but concise
- Reference issues: "Fix #123: Handle edge case in parser"

Examples:
- `feat: Add support for histogram_quantile function`
- `fix: Correct rate calculation for counter resets`
- `docs: Update README with new examples`
- `test: Add tests for label matching`
- `refactor: Simplify query builder logic`

## Documentation

- Update README.md for new features
- Add examples in `examples/` directory
- Update relevant docs in `docs/` folder
- Add inline comments for complex logic

## Review Process

1. Maintainers will review your PR
2. Address any feedback
3. Once approved, it will be merged

## Questions?

Feel free to open an issue for questions or reach out to maintainers.

Thank you for contributing! 🎉
