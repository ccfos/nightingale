package reader

import (
	"io"
	"os"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/hpcloud/tail"
)

type Reader struct {
	FilePath    string        // 配置的路径 正则路径
	t           *tail.Tail    // tail 捕获输出
	Stream      chan string   // 存储日志流
	CurrentPath string        // 当前的路径
	Close       chan struct{} // 关闭通道
	FD          uint64        // 文件的 inode
}

func NewReader(filepath string, stream chan string) (*Reader, error) {
	r := &Reader{
		FilePath: filepath,
		Stream:   stream,
		Close:    make(chan struct{}),
	}
	path := GetCurrentPath(filepath)
	err := r.openFile(io.SeekEnd, path) //默认打开SeekEnd

	return r, err
}

func (r *Reader) openFile(whence int, filepath string) error {
	seekinfo := &tail.SeekInfo{
		Offset: 0,
		Whence: whence,
	}
	config := tail.Config{
		Location: seekinfo,
		ReOpen:   true,
		Poll:     true,
		Follow:   true,
	}

	t, err := tail.TailFile(filepath, config)
	if err != nil {
		return err
	}
	r.t = t
	r.CurrentPath = filepath
	r.FD = GetFileInodeNum(r.CurrentPath)
	return nil
}

func (r *Reader) StartRead() {
	var readCnt, readSwp int64
	var dropCnt, dropSwp int64

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second * 10):
			}
			rc := readCnt
			dc := dropCnt
			logger.Debugf("read [%d] line in last 10s\n", rc-readSwp)
			logger.Debugf("drop [%d] line in last 10s\n", dc-dropSwp)
			readSwp = rc
			dropSwp = dc
		}
	}()

	for line := range r.t.Lines {
		readCnt = readCnt + 1
		select {
		case r.Stream <- line.Text:
		default:
			dropCnt = dropCnt + 1
			//TODO 数据丢失处理，从现时间戳开始截断上报5周期
			// 是否真的要做？
			// 首先，5 周期也是拍脑袋的，只能拍脑袋丢数据，并不能保证准确性
			// 其次，是当前时间推五周期，并不知道日志是什么时候，这个地方有待斟酌
			// 结论，暂且不做，后人注意
		}
	}
	done <- struct{}{}
}

func (r *Reader) StopRead() error {
	return r.t.Stop()
}

func (r *Reader) Stop() {
	if err := r.StopRead(); err != nil {
		logger.Warningf("stop reader error:%v", err)
	}
	close(r.Close)

}
func (r *Reader) Start() {
	go r.StartRead()
	for {
		select {
		case <-time.After(time.Second):
			r.check()
		case <-r.Close:
			close(r.Stream)
			return
		}
	}
}

func (r *Reader) openAndStart(path string) {
	if err := r.t.StopAtEOF(); err != nil {
		logger.Warningf("file stopAtEOF error:%v\n", err)
	}

	if err := r.openFile(io.SeekStart, path); err == nil { //从文件开始打开
		go r.StartRead()
	} else {
		logger.Warningf("openFile err @check, err: %v\n", err.Error())
	}
}

func (r *Reader) check() {
	nextPath := GetCurrentPath(r.FilePath)

	// 文件名发生变化, 一般发生在配置了动态日志场景
	if r.CurrentPath != nextPath {
		if _, err := os.Stat(nextPath); err != nil {
			logger.Warningf("stat nextpath err: %v\n", err.Error())
			return
		}
		r.openAndStart(nextPath)
		// 执行到这里, 动态日志已经reopen, 无需再进行下面同名文件的inode变化check
		return
	}

	// 同名文件 inode 变化 check，重新打开文件从头开始读取
	// TODO hpcloud/tail 应该感知到这种场景
	newFD := GetFileInodeNum(r.CurrentPath)
	if r.FD == newFD {
		return
	}
	r.FD = newFD
	logger.Warningf("inode changed, reopen file %v\n", r.CurrentPath)
	r.openAndStart(nextPath)
}
