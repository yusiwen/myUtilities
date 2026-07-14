# myUtilities Improvement Tasks

This document contains a list of actionable improvement tasks for the myUtilities project. Each task is marked with a checkbox that can be checked off when completed.

## Code Organization and Structure

1. [ ] Refactor the project to follow standard Go project layout (cmd/, pkg/, internal/, etc.)
2. [ ] Move version information to a dedicated package for better maintainability
3. [ ] Create separate packages for common utilities instead of embedding them in specific packages
4. [ ] Standardize naming conventions across the codebase
5. [ ] Remove commented-out code and TODOs, replacing them with actual implementations or GitHub issues

## Documentation

6. [ ] Add comprehensive README.md with installation and usage instructions
7. [ ] Add godoc comments to all exported functions, types, and packages
8. [ ] Create usage examples for each command
9. [ ] Document the build process and release workflow
10. [ ] Add CONTRIBUTING.md with guidelines for contributors

## Testing

11. [ ] Implement unit tests for all packages (current test coverage appears to be minimal or non-existent)
12. [ ] Add integration tests for the installer and mock packages
13. [ ] Set up CI to run tests automatically on pull requests
14. [ ] Implement benchmarks for performance-critical code
15. [ ] Add test mocks for external dependencies (GitHub API, search engines)

## Error Handling

16. [ ] Replace panic with proper error handling in installer/search.go
17. [ ] Standardize error messages and error types across the codebase
18. [ ] Implement structured logging instead of fmt.Printf and log.Println
19. [ ] Add context to errors for better debugging
20. [ ] Improve error reporting to users with more actionable messages

## Performance

21. [ ] Optimize GitHub API requests to reduce rate limiting issues
22. [ ] Implement caching for frequently accessed data
23. [ ] Use goroutines for concurrent operations where appropriate
24. [ ] Profile the application to identify bottlenecks
25. [ ] Optimize memory usage, especially when handling large files

## Security

26. [ ] Implement proper input validation for all user inputs
27. [ ] Sanitize file paths in the mock file server
28. [ ] Add HTTPS support to the mock file server
29. [ ] Implement authentication and authorization for the mock file server
30. [ ] Audit dependencies for security vulnerabilities

## Feature Enhancements

31. [ ] Add Windows support for the installer (currently commented out as TODO)
32. [ ] Implement support for more package formats (deb, rpm, etc.)
33. [ ] Add progress reporting during installations
34. [ ] Implement a configuration file for persistent settings
35. [x] Add more mock services beyond the file server (dynamic-server with admin UI)

## Build and Deployment

36. [ ] Update the Makefile to support all target platforms
37. [x] Implement semantic versioning
38. [ ] Automate the release process completely
39. [ ] Add containerization support (Docker)
40. [ ] Create installation packages for different package managers (apt, brew, etc.)

## User Experience

41. [x] Improve command-line help messages and documentation
42. [ ] Add color and formatting to terminal output
43. [ ] Implement interactive mode for complex operations
44. [x] Add command completion for shells
45. [x] Create a web UI for the mock services

## Web UI Candidates (New Feature Ideas)

Simple tools that would benefit from a web UI and gateway integration:

| Priority | Module | Description | Backend Effort | Frontend Effort |
|---|---|---|---|---|
| 🥇 | **JSON Tool** | Format, validate, compress, and query JSON (reuse CodeMirror) | ⭐ ~10 lines | ⭐ ~100 lines |
| 🥇 | **UUID** | Generate UUID v1/v4/v7, single or batch | ⭐ ~20 lines | ⭐ ~50 lines |
| 🥈 | **Timestamp** | Unix timestamp ↔ human date/time, auto-detect format | ⭐ ~30 lines | ⭐ ~60 lines |
| 🥈 | **Hash** | File upload or text input → SHA1/SHA256/SHA512/MD5 | ⭐⭐ ~40 lines | ⭐ ~80 lines |
| 🥉 | **Port Scan** | TCP port scanning from server | ⭐⭐ ~60 lines | ⭐ ~80 lines |
| 🥉 | **DNS Lookup** | DNS record queries (A/AAAA/MX/NS/TXT) | ⭐ ~40 lines | ⭐ ~60 lines |
| — | **HTTP Client** | Web-based curl: method, URL, headers, body → response | ⭐⭐⭐ ~80 lines | ⭐⭐⭐ ~150 lines |
| — | **watch** dashboard | File/git watch events via SSE stream to browser | ⭐⭐ (core/watcher ready) | ⭐⭐⭐ ~200 lines |
| — | **git commit** UI | Stage files, view diff, generate/edit commit message via LLM | ⭐⭐ (core/git + openai ready) | ⭐⭐⭐ ~200 lines |

## Recently Completed

- **Mock Dynamic Server** — Configurable multi-endpoint mock with template engine, conditional responses, delay simulation, and verbose logging
- **Admin Web UI** — Svelte 5 frontend with CodeMirror 6 JSON editor, endpoint CRUD, and config persistence
- **Custom DynamicRouter** — Thread-safe runtime endpoint registry with path parameter matching, replaces static `http.ServeMux`
- **Gateway Integration** — Mock admin available at `/mock/` via `mu gateway`, auto-discovered from `~/.config/mu/mock-config.json`
- **Unified Dark/Light Theme** — All frontends (gateway, mock, WOL, ES) share CSS variables and localStorage-based theme toggle with `mu-theme` key
- **QR Code Web UI** — Svelte 5 frontend with text input, level selector, PNG generation via `/api/qrcode`, and gateway integration at `/qrcode/`
- **JAR Analyzer Web UI** — Svelte 5 frontend with file upload, detailed analysis display, `/api/jarinfo/analyze` API, and gateway integration at `/jarinfo/`
- **Crypto Web UI** — Svelte 5 frontend with password generator, AES/DES/3DES/SM4 encrypt/decrypt, clipboard fallback, and gateway integration at `/crypto/`
- **JWT Decode/Verify** — CLI and web UI for JWT token decoding and HMAC signature verification with auto-detected algorithm and base64 key support
- **Encode/Decode** — CLI and web tab for base64, base64url, hex, URL encode/decode
- **Password Options** — `--no-digits` and `--special` flags for password generator
- **Diff Web UI** — Full-page CodeMirror merge view with real-time diff, synchronized scrolling, file upload, localStorage persistence, and gateway integration at `/diff/`
- **k8s Secret Tool** — CLI tool to generate and decode Kubernetes Opaque Secret YAML from key=value pairs, env files, or stdin