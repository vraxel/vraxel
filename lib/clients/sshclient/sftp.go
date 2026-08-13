package sshclient

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

// PutFile uploads content to a remote path via SFTP. It creates parent
// directories on the remote host if they do not exist, writes the content,
// and sets the file mode.
func (c *Client) PutFile(src []byte, dst string, mode fs.FileMode) error {
	sshClient := c.SSHClient()
	if sshClient == nil {
		return fmt.Errorf("sshclient: not connected")
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("sshclient: failed to create sftp client: %w", err)
	}
	defer sftpClient.Close()

	// Ensure parent directory exists on the remote host.
	dir := filepath.Dir(dst)
	if _, err := sftpClient.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("sshclient: failed to stat remote dir %q: %w", dir, err)
		}
		if err := sftpClient.MkdirAll(dir); err != nil {
			return fmt.Errorf("sshclient: failed to create remote dir %q: %w", dir, err)
		}
	}

	rf, err := sftpClient.Create(dst)
	if err != nil {
		return fmt.Errorf("sshclient: failed to create remote file %q: %w", dst, err)
	}
	defer rf.Close()

	if _, err := rf.Write(src); err != nil {
		return fmt.Errorf("sshclient: failed to write to remote file %q: %w", dst, err)
	}
	if err := rf.Chmod(mode); err != nil {
		return fmt.Errorf("sshclient: failed to chmod remote file %q: %w", dst, err)
	}

	return nil
}

// FetchFile downloads a remote file via SFTP and copies its content to dst.
func (c *Client) FetchFile(src string, dst io.Writer) error {
	sshClient := c.SSHClient()
	if sshClient == nil {
		return fmt.Errorf("sshclient: not connected")
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("sshclient: failed to create sftp client: %w", err)
	}
	defer sftpClient.Close()

	rf, err := sftpClient.Open(src)
	if err != nil {
		return fmt.Errorf("sshclient: failed to open remote file %q: %w", src, err)
	}
	defer rf.Close()

	if _, err := io.Copy(dst, rf); err != nil {
		return fmt.Errorf("sshclient: failed to read remote file %q: %w", src, err)
	}

	return nil
}
