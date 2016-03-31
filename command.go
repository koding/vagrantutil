package vagrantutil

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/koding/logging"
)

type command struct {
	log logging.Logger
	cwd string

	onSuccess func()
	onFailure func(err error)

	cmd *exec.Cmd
}

func newCommand(cwd string, log logging.Logger) *command {
	cmd := &command{
		cwd: cwd,
	}

	if log != nil {
		cmd.log = log.New(cwd)
	}

	return cmd
}

func (cmd *command) init(args []string) {
	cmd.cmd = exec.Command("vagrant", args...)
	cmd.cmd.Dir = cmd.cwd

	cmd.debugf("%s: executing: %v", cmd.cwd, cmd.cmd.Args)
}

func (cmd *command) run(args ...string) (string, error) {
	cmd.init(args)

	out, err := cmd.cmd.CombinedOutput()
	if err != nil {
		if len(out) != 0 {
			err = fmt.Errorf("%s: %s", err, out)
		}

		return "", cmd.done(err)
	}

	s := string(out)

	cmd.debugf("execution of %v was successful: %s", cmd.cmd.Args, s)

	return s, cmd.done(nil)

}

// start starts the command and sends back both the stdout and stderr to
// the returned channel. Any error happened during the streaming is passed to
// the Error field.
func (cmd *command) start(args ...string) (ch <-chan *CommandOutput, err error) {
	cmd.init(args)

	stdoutPipe, err := cmd.cmd.StdoutPipe()
	if err != nil {
		return nil, cmd.done(err)
	}

	stderrPipe, err := cmd.cmd.StderrPipe()
	if err != nil {
		return nil, cmd.done(err)
	}

	if err := cmd.cmd.Start(); err != nil {
		return nil, cmd.done(err)
	}

	var wg sync.WaitGroup
	out := make(chan *CommandOutput)

	output := func(r io.Reader) {
		wg.Add(1)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			cmd.debugf("%s", scanner.Text())

			out <- &CommandOutput{Line: scanner.Text(), Error: nil}
		}

		if err := scanner.Err(); err != nil {
			out <- &CommandOutput{Error: err}
		}
		wg.Done()
	}

	go output(stdoutPipe)
	go output(stderrPipe)

	go func() {
		wg.Wait()
		var err error
		if err = cmd.cmd.Wait(); err != nil {
			out <- &CommandOutput{Error: err}
		}

		close(out)
		cmd.done(err)
	}()

	return out, nil
}

func (cmd *command) done(err error) error {
	if err == nil {
		if cmd.onSuccess != nil {
			cmd.onSuccess()
		}

		return nil
	}

	cmd.debugf("execution of %v failed: %s", cmd.cmd.Args, err)

	if cmd.onFailure != nil {
		cmd.onFailure(err)
	}

	return err
}

func (cmd *command) debugf(format string, args ...interface{}) {
	if cmd.log != nil {
		cmd.log.Debug(format, args...)
	}
}
