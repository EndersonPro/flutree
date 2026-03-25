# Security Policy

## Supported Versions

We release patches for security vulnerabilities. The table below shows which versions are currently supported:

| Version | Supported          |
| ------- | ------------------ |
| 0.7.x   | ✅ Latest version  |
| < 0.7   | ❌ Unsupported    |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it to us responsibly:

### Email
Send an email to [INSERT_EMAIL_HERE] with the subject line "Security Vulnerability Report for flutree"

### Information to Include
When reporting a vulnerability, please include:
- A description of the vulnerability
- Steps to reproduce the issue
- Potential impact of the vulnerability
- Any potential mitigations you've identified
- Your contact information for follow-up

### Response Time
We will acknowledge your report within 48 hours and provide a preliminary assessment of the vulnerability within 7 days.

### What to Expect
After submitting a report:
- You will receive an acknowledgment within 48 hours
- We will investigate the issue and provide updates on our progress
- We will notify you when the vulnerability is fixed
- We will credit you in the security advisory (unless you request anonymity)

## Security Best Practices

When using `flutree`, please follow these security best practices:

### Git Repository Access
- Only use `flutree` with repositories you trust
- Ensure proper authentication and authorization for your Git repositories
- Review any changes before committing

### Local File System
- `flutree` manages worktrees in `~/Documents/worktrees/` by default
- Ensure this directory has appropriate permissions
- Regularly audit the contents of this directory

### Network Security
- `flutree` performs Git operations that may access remote repositories
- Ensure your network connection is secure when using `flutree`
- Configure Git appropriately for your security requirements

## Dependencies

We regularly update our dependencies to include security patches:
- Go modules are kept up-to-date
- We monitor security advisories for our dependencies
- Updates are included in regular releases

## Audit

Our code undergoes regular review as part of our development process, including security considerations for:
- Input validation
- File system operations
- Git command execution
- User data handling

## Disclosure

We believe in responsible disclosure and will publicly announce significant security vulnerabilities through:
- GitHub Releases
- GitHub Security Advisories
- Relevant communication channels

## Questions?

If you have any questions about this security policy or security aspects of `flutree`, please open an issue or contact us directly.