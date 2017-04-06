// Package easyssh provides a simple implementation of some SSH protocol
// features in Go. You can simply run a command on a remote server or get a file
// even simpler than native console SSH client. You don't need to think about
// Dials, sessions, defers, or public keys... Let easyssh think about it!
package easyssh

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Contains main authority information.
// User field should be a name of user on remote server (ex. john in ssh john@example.com).
// Server field should be a remote machine address (ex. example.com in ssh john@example.com)
// Key is a path to private key on your local machine.
// Port is SSH server port on remote machine.
// Note: easyssh looking for private key in user's home directory (ex. /home/john + Key).
// Then ensure your Key begins from '/' (ex. /.ssh/id_rsa)
type MakeConfig struct {
	User            string
	Server          string
	Key             string
	Port            string
	Password        string
	KeyData         []byte
	HostKeyCallback ssh.HostKeyCallback
}

var sshCfgRegex = regexp.MustCompile(`\s*(\w+)\s+(\S+)\s*`)

func NewConnection(target string) (*MakeConfig, error) {
	cfg := &MakeConfig{
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	overwriteUser := false

	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("Error determining current user: %s", err)
	}

	if pos := strings.Index(target, "@"); pos != -1 {
		cfg.User = target[0:pos]
		cfg.Server = target[pos+1:]
		overwriteUser = true
	} else {
		cfg.Server = target
		cfg.User = currentUser.Username
	}

	file := path.Join(currentUser.HomeDir, ".ssh", "config")

	if _, err := os.Stat(file); os.IsNotExist(err) {
		return cfg, nil
	}

	if sshCfg, err := parseConfigFile(file, cfg.Server); err != nil {
		return nil, fmt.Errorf("Error reading SSH config file '%s': %s", file, err)
	} else if sshCfg != nil {
		if overwriteUser {
			sshCfg.User = cfg.User
		}
		return sshCfg, nil
	}

	return cfg, nil
}

func parseConfigFile(filename string, host string) (*MakeConfig, error) {
	file, _ := os.Open(filename)
	defer file.Close()

	return parseClientConfig(file, host)
}

func parseClientConfig(reader io.Reader, host string) (*MakeConfig, error) {
	var cfg *MakeConfig

	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

lines:
	for scanner.Scan() {
		m := sshCfgRegex.FindStringSubmatch(scanner.Text())
		if len(m) != 3 {
			continue
		}

		key := strings.ToLower(m[1])
		value := m[2]

		switch key {
		case "host":
			if cfg != nil {
				break lines
			}

			if host == value {
				cfg = &MakeConfig{Server: value, Port: "22"}
			}

		case "hostname":
			if cfg != nil {
				cfg.Server = value
			}

		case "user":
			if cfg != nil {
				cfg.User = value
			}

		case "identityfile":
			if cfg == nil {
				continue
			}
			if value[:2] == "~/" {
				usr, err := user.Current()
				if err != nil {
					return nil, err
				}
				value = path.Join(usr.HomeDir, strings.Replace(value, "~/", "", 1))
			}
			cfg.Key = value

		case "port":
			if cfg != nil {
				cfg.Port = value
			}
			/*port, err := strconv.Atoi(next.val)
			if err != nil {
				return nil, err
			}*/
		}
	}

	return cfg, nil
}

// returns ssh.Signer from user you running app home path + cutted key path.
// (ex. pubkey,err := getKeyFile("/.ssh/id_rsa") )
func getKeyFile(keypath string) (ssh.Signer, error) {
	buf, err := ioutil.ReadFile(keypath)
	if err != nil {
		return nil, err
	}

	pubkey, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}

	return pubkey, nil
}

// connects to remote server using MakeConfig struct and returns *ssh.Session
func (ssh_conf *MakeConfig) connect() (*ssh.Session, error) {
	// auths holds the detected ssh auth methods
	auths := []ssh.AuthMethod{}

	// figure out what auths are requested, what is supported
	if ssh_conf.Password != "" {
		auths = append(auths, ssh.Password(ssh_conf.Password))
	}

	if len(ssh_conf.KeyData) > 0 {
		pubkey, err := ssh.ParsePrivateKey(ssh_conf.KeyData)
		if err != nil {
			return nil, err
		}
		auths = append(auths, ssh.PublicKeys(pubkey))
	} else if ssh_conf.Key != "" {
		pubkey, err := getKeyFile(ssh_conf.Key)
		if err != nil {
			return nil, err
		}
		auths = append(auths, ssh.PublicKeys(pubkey))
	}

	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
		defer sshAgent.Close()
	}

	config := &ssh.ClientConfig{
		User:            ssh_conf.User,
		Auth:            auths,
		HostKeyCallback: ssh_conf.HostKeyCallback,
	}

	client, err := ssh.Dial("tcp", ssh_conf.Server+":"+ssh_conf.Port, config)
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}

	return session, nil
}

// Stream returns one channel that combines the stdout and stderr of the command
// as it is run on the remote machine, and another that sends true when the
// command is done. The sessions and channels will then be closed.
func (ssh_conf *MakeConfig) Stream(command string) (output chan string, done chan bool, err error) {
	// connect to remote host
	session, err := ssh_conf.connect()
	if err != nil {
		return output, done, err
	}

	if err := session.RequestPty("xterm", 80, 24, ssh.TerminalModes{}); err != nil {
		return output, done, err
	}

	// connect to both outputs (they are of type io.Reader)
	outReader, err := session.StdoutPipe()
	if err != nil {
		return output, done, err
	}
	errReader, err := session.StderrPipe()
	if err != nil {
		return output, done, err
	}
	// combine outputs, create a line-by-line scanner
	outputReader := io.MultiReader(outReader, errReader)
	err = session.Start(command)
	scanner := bufio.NewScanner(outputReader)
	// continuously send the command's output over the channel
	outputChan := make(chan string)
	done = make(chan bool)
	go func(scanner *bufio.Scanner, out chan string, done chan bool) {
		defer close(outputChan)
		defer close(done)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
		// close all of our open resources
		done <- true
		session.Close()
	}(scanner, outputChan, done)
	return outputChan, done, err
}

// Runs command on remote machine and returns its stdout as a string
func (ssh_conf *MakeConfig) Run(command string) (outStr string, err error) {
	outChan, doneChan, err := ssh_conf.Stream(command)
	if err != nil {
		return outStr, err
	}
	// read from the output channel until the done signal is passed
	stillGoing := true
	for stillGoing {
		select {
		case <-doneChan:
			stillGoing = false
		case line := <-outChan:
			outStr += line + "\n"
		}
	}
	// return the concatenation of all signals from the output channel
	return outStr, err
}

// Scp uploads sourceFile to remote machine like native scp console app.
func (ssh_conf *MakeConfig) Upload(sourceFile, targetFile string) error {
	session, err := ssh_conf.connect()

	if err != nil {
		return err
	}
	defer session.Close()

	src, srcErr := os.Open(sourceFile)

	if srcErr != nil {
		return srcErr
	}

	srcStat, statErr := src.Stat()

	if statErr != nil {
		return statErr
	}

	go func() {
		w, _ := session.StdinPipe()

		fmt.Fprintln(w, "C0644", srcStat.Size(), filepath.Base(targetFile))

		if srcStat.Size() > 0 {
			io.Copy(w, src)
			fmt.Fprint(w, "\x00")
			w.Close()
		} else {
			fmt.Fprint(w, "\x00")
			w.Close()
		}
	}()

	if err := session.Run(fmt.Sprintf("scp -t %s", targetFile)); err != nil {
		return err
	}

	return nil
}
