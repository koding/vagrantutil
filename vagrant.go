package vagrantutil

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

//go:generate stringer -type=BoxSubcommand,Status  -output=stringer.go

type (
	BoxSubcommand int
	Status        int
)

const (
	Add BoxSubcommand = iota
	List
	Outdated
	Remove
	Repackage
	Update
)

const (
	Unknown Status = iota
	NotCreated
	Running
	Saved
)

type Vagrant struct {
	// VagrantfilePath is the directory with specifies the directory where
	// Vagrantfile is being stored.
	VagrantfilePath string
}

// NewVagrant returns a new Vagrant instance for the given name. The name
// should be unique. If the name already exists in the system it'll be used, if
// not a new setup will be createad.
func NewVagrant(name string) (*Vagrant, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	if name == "" {
		return nil, err
	}

	vagrantHome := filepath.Join(u.HomeDir, ".koding-boxes", name)
	if err := os.MkdirAll(vagrantHome, 0755); err != nil {
		return nil, err
	}

	return &Vagrant{
		VagrantfilePath: vagrantHome,
	}, nil
}

func (v *Vagrant) Version() (string, error) {
	out, err := v.runCommand("version")
	if err != nil {
		return "", err
	}

	records, err := parseRecords(out)
	if err != nil {
		return "", err
	}

	versionInstalled, err := parseData(records, "version-installed")
	if err != nil {
		return "", err
	}

	return versionInstalled, nil
}

func (v *Vagrant) Box(subcommand BoxSubcommand) (string, error) {
	out, err := v.runCommand("box", subcommand.String())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

func (v *Vagrant) Status() (Status, error) {
	if err := v.vagrantfileExists(); err != nil {
		return Unknown, err
	}

	out, err := v.runCommand("status")
	if err != nil {
		return Unknown, err
	}

	records, err := parseRecords(out)
	if err != nil {
		return Unknown, err
	}

	status, err := parseData(records, "state")
	if err != nil {
		return Unknown, err
	}

	return toStatus(status), nil
}

// Up executes "vagrant up" for the given vagrantfile. The returned reader
// contains the output stream. The client is responsible of calling the Close
// method of the returned reader.
func (v *Vagrant) Up(vagrantfile string) (io.ReadCloser, error) {
	if vagrantfile == "" {
		return nil, errors.New("Vagrantfile content is empty")
	}

	// if it's exists, don't overwrite anything and use the existing one
	if err := v.vagrantfileExists(); err != nil {
		err := ioutil.WriteFile(v.vagrantfile(), []byte(vagrantfile), 0644)
		if err != nil {
			return nil, err
		}
	} else {
		// TODO(arslan): replace logging with koding/logging
		log.Printf("Using existing Vagrantfile at %s", v.VagrantfilePath)
	}

	cmd := v.createCommand("up")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[error]: vagrant up error: %s", err)
		}
	}()

	return pipe, nil
}

// Destroy executes "vagrant destroy". The returned reader contains the output
// stream. The client is responsible of calling the Close method of the
// returned reader.
func (v *Vagrant) Destroy() (io.ReadCloser, error) {
	if err := v.vagrantfileExists(); err != nil {
		return nil, err
	}

	cmd := v.createCommand("destroy", "--force")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[error]: vagrant up error: %s", err)
		}
	}()

	return pipe, nil
}

// vagrantfile returns the Vagrantfile path
func (v *Vagrant) vagrantfile() string {
	return filepath.Join(v.VagrantfilePath, "Vagrantfile")
}

// vagrantfileExists checks if a Vagrantfile exists in the given path. It
// returns a nil error if exists.
func (v *Vagrant) vagrantfileExists() error {
	if _, err := os.Stat(v.vagrantfile()); os.IsNotExist(err) {
		return err
	}
	return nil
}

func (v *Vagrant) createCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("vagrant", args...)
	cmd.Dir = v.VagrantfilePath
	return cmd
}

func (v *Vagrant) runCommand(args ...string) (string, error) {
	args = append(args, "--machine-readable")
	cmd := exec.Command("vagrant", args...)
	cmd.Dir = v.VagrantfilePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func parseData(records [][]string, typeName string) (string, error) {
	data := ""
	for _, record := range records {
		// first three are defined, after that data is variadic, it contains
		// zero or more information. We should have a data, otherwise it's
		// useless.
		if len(record) < 4 {
			continue
		}

		if typeName == record[2] {
			data = record[3]
		}
	}

	if data == "" {
		return "", fmt.Errorf("couldn't parse data for vagrant type: '%s'", typeName)
	}

	return data, nil
}

func parseRecords(out string) ([][]string, error) {
	buf := bytes.NewBufferString(out)
	c := csv.NewReader(buf)
	return c.ReadAll()
}

// toStatus convers the given state string to Status type
func toStatus(state string) Status {
	switch state {
	case "running":
		return Running
	case "not_created":
		return NotCreated
	case "saved":
		return Saved
	default:
		return Unknown
	}

}
