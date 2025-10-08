# SSHExec Package

The SSHExec package provides comprehensive SSH and SFTP client functionality for executing remote commands and transferring files. It supports both interactive and non-interactive sessions with advanced features like terminal handling, session timeouts, and connection management.

## Key Features

- **SSH Command Execution** - Execute commands on remote servers
- **Interactive Shell Support** - Full terminal support with PTY allocation
- **SFTP File Transfer** - Upload and download files via SFTP
- **Connection Management** - Automatic connection handling and cleanup
- **Authentication Methods** - Support for password and key-based authentication
- **Session Timeouts** - Configurable timeouts for long-running operations
- **Terminal Handling** - Proper terminal state management and restoration
- **Window Resizing** - Dynamic terminal window size adjustment

## Main Types

### ExecInput
Configuration structure for SSH execution containing client configuration and execution details.

### ClientConf
SSH client connection configuration including host, port, authentication credentials, and timeouts.

### ExecDetail
Execution-specific configuration including command, I/O streams, environment variables, and terminal settings.

### ExecOutput
Result structure containing stdout, stderr, errors, and warnings from command execution.

## Main Functions

### SSH Execution
- `Exec(i *ExecInput) *ExecOutput` - Execute SSH command with full configuration
- `ExecPrepare(i *ExecInput) (session *ssh.Session, conn *ssh.Client, err error)` - Prepare SSH session for execution
- `ExecRun(session *ssh.Session, conn *ssh.Client, i *ExecInput) *ExecOutput` - Run command on prepared session

### SFTP Operations
- `SftpUpload(i *SftpInput) *SftpOutput` - Upload files via SFTP
- `SftpDownload(i *SftpInput) *SftpOutput` - Download files via SFTP
- `SftpList(i *SftpInput) *SftpOutput` - List remote directory contents

### Terminal Management
- `AddRestoreRequest()` - Request terminal state restoration
- `RestoreTerminal()` - Restore terminal to original state

## Usage Examples

### Basic Command Execution

```go
import "github.com/aerospike/aerolab/pkg/sshexec"

input := &sshexec.ExecInput{
    ClientConf: sshexec.ClientConf{
        Host:           "example.com",
        Port:           22,
        Username:       "user",
        Password:       "password",
        ConnectTimeout: 30 * time.Second,
    },
    ExecDetail: sshexec.ExecDetail{
        Command:        []string{"ls", "-la"},
        SessionTimeout: 5 * time.Minute,
    },
}

output := sshexec.Exec(input)
if output.Err != nil {
    log.Fatal(output.Err)
}

fmt.Printf("Output: %s\n", string(output.Stdout))
```

### Interactive Shell Session

```go
input := &sshexec.ExecInput{
    ClientConf: sshexec.ClientConf{
        Host:     "example.com",
        Port:     22,
        Username: "user",
        PrivateKey: privateKeyBytes,
    },
    ExecDetail: sshexec.ExecDetail{
        Command:  []string{}, // Empty for interactive shell
        Terminal: true,
        Stdin:    os.Stdin,
        Stdout:   os.Stdout,
        Stderr:   os.Stderr,
    },
}

output := sshexec.Exec(input)
if output.Err != nil {
    log.Fatal(output.Err)
}
```

### SFTP File Upload

```go
input := &sshexec.SftpInput{
    ClientConf: sshexec.ClientConf{
        Host:     "example.com",
        Port:     22,
        Username: "user",
        Password: "password",
    },
    SftpDetail: sshexec.SftpDetail{
        Operation:  sshexec.SftpOperationUpload,
        LocalPath:  "/local/file.txt",
        RemotePath: "/remote/file.txt",
        Recursive:  false,
    },
}

output := sshexec.SftpUpload(input)
if output.Err != nil {
    log.Fatal(output.Err)
}
```

### Key-Based Authentication

```go
privateKey, err := ioutil.ReadFile("/path/to/private/key")
if err != nil {
    log.Fatal(err)
}

input := &sshexec.ExecInput{
    ClientConf: sshexec.ClientConf{
        Host:       "example.com",
        Port:       22,
        Username:   "user",
        PrivateKey: privateKey,
    },
    ExecDetail: sshexec.ExecDetail{
        Command: []string{"uptime"},
    },
}

output := sshexec.Exec(input)
```

### Environment Variables

```go
input := &sshexec.ExecInput{
    ClientConf: sshexec.ClientConf{
        Host:     "example.com",
        Port:     22,
        Username: "user",
        Password: "password",
    },
    ExecDetail: sshexec.ExecDetail{
        Command: []string{"echo", "$MY_VAR"},
        Env: []*sshexec.Env{
            {Key: "MY_VAR", Value: "hello world"},
        },
    },
}

output := sshexec.Exec(input)
```

## Advanced Features

### Session Timeout Handling
The package supports configurable session timeouts to prevent hanging connections:

```go
ExecDetail{
    Command:        []string{"long-running-command"},
    SessionTimeout: 10 * time.Minute, // Kill session after 10 minutes
}
```

### Terminal Support
For interactive sessions, the package provides full terminal support including:
- PTY allocation with proper terminal modes
- Window resize handling
- Terminal state restoration
- Color and escape sequence support

### Connection Pooling
The package manages SSH connections efficiently with:
- Automatic connection cleanup
- Session management
- Resource leak prevention

### Error Handling
Comprehensive error handling includes:
- Connection errors with retry logic
- Authentication failures
- Command execution errors
- Timeout handling
- Warning collection for non-fatal issues

## Thread Safety

The SSHExec package is designed to be thread-safe for concurrent operations:
- Session management uses proper locking
- Terminal state restoration is atomic
- Connection handling prevents race conditions

## Platform Support

The package provides cross-platform support with platform-specific implementations for:
- Terminal handling (Windows vs Unix)
- Signal handling
- File path operations

## Security Considerations

- Host key verification can be configured (currently uses `InsecureIgnoreHostKey` for simplicity)
- Private key parsing with proper error handling
- Secure credential handling
- Session cleanup to prevent resource leaks
