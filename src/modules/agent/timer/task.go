package timer

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/modules/agent/client"
	"github.com/didi/nightingale/src/modules/agent/config"
)

type Task struct {
	sync.Mutex

	Id     int64
	Clock  int64
	Action string
	Status string

	alive  bool
	Cmd    *exec.Cmd
	Stdout bytes.Buffer
	Stderr bytes.Buffer

	Args    string
	Account string
}

func (t *Task) SetStatus(status string) {
	t.Lock()
	t.Status = status
	t.Unlock()
}

func (t *Task) GetStatus() string {
	t.Lock()
	s := t.Status
	t.Unlock()
	return s
}

func (t *Task) GetAlive() bool {
	t.Lock()
	pa := t.alive
	t.Unlock()
	return pa
}

func (t *Task) SetAlive(pa bool) {
	t.Lock()
	t.alive = pa
	t.Unlock()
}

func (t *Task) GetStdout() string {
	t.Lock()
	out := t.Stdout.String()
	t.Unlock()
	return out
}

func (t *Task) GetStderr() string {
	t.Lock()
	out := t.Stderr.String()
	t.Unlock()
	return out
}

func (t *Task) ResetBuff() {
	t.Lock()
	t.Stdout.Reset()
	t.Stderr.Reset()
	t.Unlock()
}

func (t *Task) doneBefore() bool {
	doneFlag := path.Join(config.Config.Job.MetaDir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	return file.IsExist(doneFlag)
}

func (t *Task) loadResult() {
	metadir := config.Config.Job.MetaDir

	doneFlag := path.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	stdoutFile := path.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderrFile := path.Join(metadir, fmt.Sprint(t.Id), "stderr")

	var err error

	t.Status, err = file.ReadStringTrim(doneFlag)
	if err != nil {
		log.Printf("[E] read file %s fail %v", doneFlag, err)
	}
	stdout, err := file.ReadString(stdoutFile)
	if err != nil {
		log.Printf("[E] read file %s fail %v", stdoutFile, err)
	}
	stderr, err := file.ReadString(stderrFile)
	if err != nil {
		log.Printf("[E] read file %s fail %v", stderrFile, err)
	}

	t.Stdout = *bytes.NewBufferString(stdout)
	t.Stderr = *bytes.NewBufferString(stderr)
}

func (t *Task) prepare() error {
	if t.Account != "" {
		// already prepared
		return nil
	}

	IdDir := path.Join(config.Config.Job.MetaDir, fmt.Sprint(t.Id))
	err := file.EnsureDir(IdDir)
	if err != nil {
		log.Printf("[E] mkdir -p %s fail: %v", IdDir, err)
		return err
	}

	writeFlag := path.Join(IdDir, ".write")
	if file.IsExist(writeFlag) {
		// 从磁盘读取
		argsFile := path.Join(IdDir, "args")
		args, err := file.ReadStringTrim(argsFile)
		if err != nil {
			log.Printf("[E] read %s fail %v", argsFile, err)
			return err
		}

		accountFile := path.Join(IdDir, "account")
		account, err := file.ReadStringTrim(accountFile)
		if err != nil {
			log.Printf("[E] read %s fail %v", accountFile, err)
			return err
		}

		t.Args = args
		t.Account = account
	} else {
		// 从远端读取，再写入磁盘
		script, args, account, err := client.Meta(t.Id)
		if err != nil {
			log.Println("[E] query task meta fail:", err)
			return err
		}

		scriptFile := path.Join(IdDir, "script")
		_, err = file.WriteString(scriptFile, script)
		if err != nil {
			log.Printf("[E] write script to %s fail: %v", scriptFile, err)
			return err
		}

		out, err := sys.CmdOutTrim("chmod", "+x", scriptFile)
		if err != nil {
			log.Printf("[E] chmod +x %s fail %v. output: %s", scriptFile, err, out)
			return err
		}

		argsFile := path.Join(IdDir, "args")
		_, err = file.WriteString(argsFile, args)
		if err != nil {
			log.Printf("[E] write args to %s fail: %v", argsFile, err)
			return err
		}

		accountFile := path.Join(IdDir, "account")
		_, err = file.WriteString(accountFile, account)
		if err != nil {
			log.Printf("[E] write account to %s fail: %v", accountFile, err)
			return err
		}

		_, err = file.WriteString(writeFlag, "")
		if err != nil {
			log.Printf("[E] create %s flag file fail: %v", writeFlag, err)
			return err
		}

		t.Args = args
		t.Account = account
	}

	return nil
}

func (t *Task) start() {
	if t.GetAlive() {
		return
	}

	err := t.prepare()
	if err != nil {
		return
	}

	args := t.Args
	if args != "" {
		args = strings.Replace(args, ",,", "' '", -1)
		args = "'" + args + "'"
	}

	scriptFile := path.Join(config.Config.Job.MetaDir, fmt.Sprint(t.Id), "script")
	sh := fmt.Sprintf("%s %s", scriptFile, args)
	var cmd *exec.Cmd
	if t.Account == "root" {
		cmd = exec.Command("sh", "-c", sh)
		cmd.Dir = "/root"
	} else {
		cmd = exec.Command("su", "-c", sh, "-", t.Account)
	}

	cmd.Stdout = &t.Stdout
	cmd.Stderr = &t.Stderr
	t.Cmd = cmd

	err = cmd.Start()
	if err != nil {
		log.Printf("[E] cannot start cmd of task[%d]: %v", t.Id, err)
		return
	}

	go runProcess(t)
}

func (t *Task) kill() {
	go killProcess(t)
}

func runProcess(t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	err := t.Cmd.Wait()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			t.SetStatus("killed")
			logger.Debugf("process of task[%d] killed", t.Id)
		} else {
			t.SetStatus("failed")
			logger.Debugf("process of task[%d] return error: %v", t.Id, err)
		}
	} else {
		t.SetStatus("success")
		logger.Debugf("process of task[%d] done", t.Id)
	}

	persistResult(t)
}

func persistResult(t *Task) {
	metadir := config.Config.Job.MetaDir

	stdout := path.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderr := path.Join(metadir, fmt.Sprint(t.Id), "stderr")
	doneFlag := path.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))

	file.WriteString(stdout, t.GetStdout())
	file.WriteString(stderr, t.GetStderr())
	file.WriteString(doneFlag, t.GetStatus())
}

func killProcess(t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	logger.Debugf("begin kill process of task[%d]", t.Id)

	err := KillProcessByTaskID(t.Id)
	if err != nil {
		t.SetStatus("killfailed")
		logger.Debugf("kill process of task[%d] fail: %v", t.Id, err)
	} else {
		t.SetStatus("killed")
		logger.Debugf("process of task[%d] killed", t.Id)
	}

	persistResult(t)
}
