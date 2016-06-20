package easyssh

import (
	"testing"
	"strings"
	"os/user"
)

var sshConfig = &MakeConfig{
	User:     "username",
	Server:   "example.com",
	Password: "password",
	//Key:  "/.ssh/id_rsa",
	Port: "22",
}

func TestParsingClientConfig(t *testing.T) {
	var home string
	if currentUser, err := user.Current(); err != nil {
		t.Errorf("Error determining current user: %s", err)
	} else {
		home = currentUser.HomeDir
	}
	cfg := `
Host notthisone	


     Host
Host bla
	Hostname hostname
	Hostname 1.2.3.4
	User ubuntu
	IdentityFile	~/.ssh/bluurp.pem
Host anotherOne
User user
`
	result, err := parseClientConfig(strings.NewReader(cfg), "bla")
	if err != nil {
		t.Errorf("Error parsing config: %s", err)
	}

	expected:= MakeConfig{
		Server: "1.2.3.4",
		User: "ubuntu",
		Key: home + "/.ssh/bluurp.pem",
		Port: "22",
	}

	if *result != expected {
		t.Errorf("Expected %v, got %v", expected, *result)
	}
}

func TestParsingConnectionString(t *testing.T) {
	var username string
	if currentUser, err := user.Current(); err != nil {
		t.Errorf("Error determining current user: %s", err)
	} else {
		username = currentUser.Username
	}

	testCases := map[string]MakeConfig {
		"blub@bla": {User: "blub", Server: "bla"},
		"blubber": {User: username, Server: "blubber"},
	}

	for input, expected := range testCases{
		c, err := NewConnection(input)

		if err != nil {
			t.Errorf("Error parsing connection string '%s': %s", input, err)
		}

		if c.User != expected.User {
			t.Errorf("Expected username '%s' for '%s', got '%s'", expected.User, input, c.User)
		}

		if c.Server != expected.Server {
			t.Errorf("Expected hostname '%s' for '%s', got '%s'", expected.Server, input, c.Server)
		}
	}
}

/*func TestRun(t *testing.T) {
	commands := []string{
		"echo test", `for i in $(ls); do echo "$i"; done`, "ls",
	}
	for _, cmd := range commands {
		c, err := NewConnection("jordan")
		t.Logf("%v", c)
		if err != nil {
			t.Errorf("Run failed: %s", err)
		}
		out, err := c.Run(cmd)
		t.Logf("%s\n", out)
		if err != nil {
			t.Errorf("Run failed: %s", err)
		}
		if out == "" {
			t.Errorf("Output was empty for command: %s", cmd)
		}
	}
}*/

/*func TestStream(t *testing.T) {
	t.Parallel()
	// input command/output string pairs
	testCases := [][]string{
		{`for i in $(seq 1 5); do echo "$i"; done`, "12345"},
		{`echo "test"`, "test"},
	}
	for _, testCase := range testCases {
		channel, done, err := sshConfig.Stream(testCase[0])
		if err != nil {
			t.Errorf("Stream failed: %s", err)
		}
		stillGoing := true
		output := ""
		for stillGoing {
			select {
			case <-done:
				stillGoing = false
			case line := <-channel:
				output += line
			}
		}
		if output != testCase[1] {
			t.Error("Output didn't match expected: %s", output)
		}
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	commands := []string{
		"echo test", `for i in $(ls); do echo "$i"; done`, "ls",
	}
	for _, cmd := range commands {
		out, err := sshConfig.Run(cmd)
		if err != nil {
			t.Errorf("Run failed: %s", err)
		}
		if out == "" {
			t.Errorf("Output was empty for command: %s", cmd)
		}
	}
}*/
