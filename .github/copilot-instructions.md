Always use these make targets for development tasks.

### Building and Running

- `make build` - Build the Docker container for the Go application
- `make start` - Start all services (Docker Compose)
- `make stop` - Stop all services  
- `make clean` - Clean build artifacts

### Dependencies

This project uses Go modules:
- Dependencies are handled by `make deps` target
- Do not suggest direct `go get` commands
- Use `make tidy` to clean up module references

## Development Workflow

1. **Setup**: Always run `make deps` first
2. **Build**: Use `make build` before testing
3. **Test**: Run `make test` to validate changes
4. **Format**: Use `make fmt` to format code
5. **Quality**: Run `make vet` for static analysis and `make lint` for linting

## Coding Best Practices

- Follow idiomatic Go patterns and keep code maintainable and DRY
- Prefer dependency injection for testability and modularity
- Document public function and APIs using GoDoc format
- Explain the purpose of functions, types, and constants
- Keep packages small and focused; separate protocol handling from business logic
- Use interfaces for dependencies to support testing
- Use error wrapping with context information
- Keep functions short and focused; use descriptive variable names
- Maintain thread safety for shared state
- Use efficient data structures and cache expensive results where appropriate
- Use structured error handling and custom error types if needed
- Log important events and add metrics/traces for critical paths
- Use context propagation and explicit error handling
- Use context for goroutine management and cancellation
- Validate inputs early and use guard clauses
- Use `defer` for resource cleanup
- Use constants for timeouts and repeated values
- Guard new or risky changes with feature flags when possible

### Testing and Quality

- `make test` - Run all tests. Always write unit tests for new code and ensure all tests pass.
- Use table-driven tests for Go unit tests when possible.
- `make fmt` - Format Go code. Run before every commit.
- `make vet` - Run go vet for static analysis
- `make lint` - Run golangci-lint (if available) before merging

- Organize tests using table-driven tests or testify suites
- Use clear, descriptive test names
- Use realistic test data for operations, instances, and messages.
- Mock external dependencies for reliable unit testing
- Document the intent of each test and suite using comments, and explain complex setup or assertions inline.
- Use `suite.Run(t, new(MyTestSuite))` as the entry point for each suite, and ensure tests are deterministic and do not depend on external state.

Example test skeleton:

func (s *MyTestSuite) TestMyFunction_Scenario() {
    // Arrange: setup mocks, inputs, and expected state

    // Act: call the function under test

    // Assert: check results and side effects
    s.Require().NoError(err)
    s.Equal(expected, actual)
}