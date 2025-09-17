# Contributing to Go Phoenix Channels

Thank you for your interest in contributing to the Go Phoenix Channels client library! This document provides guidelines and information for contributors.

## Development Setup

### Prerequisites

- Go 1.22 or later
- Elixir 1.18+ and Phoenix Framework (for testing)
- Git

### Setting Up the Development Environment

1. Fork and clone the repository:
```bash
git clone https://github.com/your-username/go-phx-channels.git
cd go-phx-channels
```

2. Install Go dependencies:
```bash
go mod download
```

3. Set up the Phoenix test server:
```bash
cd test_server
mix deps.get
```

### Running Tests

#### Unit Tests
```bash
go test -v ./...
```

#### Integration Tests
Start the Phoenix test server first:
```bash
cd test_server
mix phx.server
```

Then run integration tests:
```bash
go test -v ./... -run Integration
```

#### All Tests
```bash
# Start Phoenix server in background
cd test_server && mix phx.server &

# Run all tests
go test -v ./...
```

## Code Style and Standards

### Go Code Style

- Follow standard Go conventions and use `gofmt`
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and reasonably sized
- Use Go modules for dependency management

### Documentation Standards

- All exported functions must have Go doc comments
- Include usage examples in complex function documentation
- Update README.md for significant feature additions
- Add examples for new functionality

### Testing Standards

- Write unit tests for all new functionality
- Include integration tests for complex features
- Test both success and error scenarios
- Use table-driven tests for multiple test cases
- Mock external dependencies in unit tests

## Project Structure

```
go-phx-channels/
├── socket.go              # Main socket implementation
├── channel.go             # Channel implementation
├── push.go               # Push (request/response) implementation
├── serializer.go         # Message serialization/deserialization
├── gophxchannels.go      # Package exports and utilities
├── *_test.go            # Unit tests
├── integration_test.go   # Integration tests
├── examples/            # Usage examples
├── test_server/         # Phoenix test server
└── docs/               # Additional documentation
```

## Making Contributions

### Before You Start

1. Check existing issues and pull requests to avoid duplication
2. For major changes, open an issue first to discuss the approach
3. Ensure your changes are backward compatible when possible

### Pull Request Process

1. **Create a feature branch:**
```bash
git checkout -b feature/your-feature-name
```

2. **Implement your changes:**
   - Write clean, well-documented code
   - Add appropriate tests
   - Update documentation as needed

3. **Test your changes:**
```bash
# Run unit tests
go test -v ./...

# Run integration tests (with Phoenix server running)
go test -v ./... -run Integration

# Run examples to ensure they still work
cd examples/basic_usage && go run main.go
```

4. **Commit your changes:**
```bash
git add .
git commit -m "Add feature: description of your changes"
```

5. **Push and create a pull request:**
```bash
git push origin feature/your-feature-name
```

### Commit Message Guidelines

- Use clear, descriptive commit messages
- Start with a verb in imperative mood (Add, Fix, Update, etc.)
- Keep the first line under 50 characters
- Include detailed description if necessary

Examples:
```
Add binary payload support for channels
Fix reconnection logic for dropped connections
Update documentation for new authentication options
```

### Pull Request Guidelines

- **Title:** Clear, concise description of the change
- **Description:** Explain what changes were made and why
- **Testing:** Describe how the changes were tested
- **Breaking Changes:** Highlight any breaking changes
- **Issues:** Reference related issues with "Fixes #123" or "Relates to #123"

## Types of Contributions

### Bug Fixes
- Include a test case that reproduces the bug
- Ensure the fix doesn't break existing functionality
- Update documentation if the bug was due to unclear docs

### New Features
- Discuss major features in an issue first
- Follow Phoenix JavaScript client patterns when possible
- Include comprehensive tests and documentation
- Add usage examples

### Documentation Improvements
- Fix typos and improve clarity
- Add missing documentation for existing features
- Update examples for accuracy
- Improve README and getting started guides

### Performance Improvements
- Include benchmarks demonstrating the improvement
- Ensure changes don't negatively impact other functionality
- Document any trade-offs

## Phoenix Protocol Compatibility

This library aims to maintain compatibility with the Phoenix Framework's WebSocket protocol. When making changes:

### Protocol Adherence
- Follow Phoenix message format specifications
- Test against multiple Phoenix versions when possible
- Maintain binary protocol compatibility
- Preserve JavaScript client API patterns

### Testing Against Phoenix
- Test with the included Phoenix test server
- Verify compatibility with real Phoenix applications
- Test both JSON and binary message formats

## Code Review Process

All contributions go through code review:

1. **Automated Checks:** Tests and linting must pass
2. **Peer Review:** Code review by maintainers
3. **Testing:** Integration testing with Phoenix server
4. **Documentation:** Review of docs and examples

### What Reviewers Look For

- **Correctness:** Does the code work as intended?
- **Testing:** Are there sufficient tests?
- **Style:** Does it follow Go conventions?
- **Documentation:** Is it well documented?
- **Compatibility:** Does it maintain Phoenix protocol compatibility?
- **Performance:** Are there any performance regressions?

## Release Process

The project follows semantic versioning (SemVer):

- **Major (1.0.0):** Breaking changes
- **Minor (0.1.0):** New features, backward compatible
- **Patch (0.0.1):** Bug fixes, backward compatible

## Getting Help

- **Questions:** Open a GitHub issue with the "question" label
- **Bugs:** Open a GitHub issue with a detailed reproduction case
- **Features:** Open a GitHub issue to discuss the feature first
- **Chat:** Join discussions in GitHub issues and pull requests

## License

By contributing to Go Phoenix Channels, you agree that your contributions will be licensed under the MIT License.

## Recognition

Contributors are recognized in:
- Git commit history
- Release notes for significant contributions
- README contributors section (for substantial contributions)

Thank you for contributing to Go Phoenix Channels! 🚀