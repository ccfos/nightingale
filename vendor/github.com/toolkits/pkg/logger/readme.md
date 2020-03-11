### log for golang

#### 常量
- severity
    - DEBUG
    - INFO
    - WARNING
    - ERROR
    - FATAL

#### 格式
2015-06-16 12:00:35 ERROR test.go:12 ...

#### backend
- 实现Log(s Severity, msg []byte) 和 Close()
- 初始时调用`dlog.SetLogging(dlog.INFO, backend)`，也可传字符串`dlog.SetLogging("INFO", backend)`；默认输出到stdout，级别为DEBUG；单独设置日志级别：`dlog.SetSeverity("INFO")`

#### 输出到stderr而不是对应的后端（方便调试用）

    if debug {
        dlog.LogToStderr()
    }

#### log to local file 
    
    b, err := dlog.NewFileBackend("./log") //log文件目录
    if err != nil {
        panic(err)
    }
    dlog.SetLogging("INFO", b)     //只输出大于等于INFO的log
    b.Rotate(10, 1024*1024*500) //自动切分日志，保留10个文件（INFO.log.000-INFO.log.009，循环覆盖），每个文件大小为500M, 因为dlog支持多个文件后端， 所以需要为每个file backend指定具体切分数值
    dlog.Info(1, 2, " test")
    dlog.Close()

- log将输出到指定目录下面`INFO.log`，`WARNING.log`，`ERROR.log`，`FATAL.log`
- 为了配合op的日志切分工具，有个goroutine定期检查log文件是否消失并且创建新的log文件
- 为了性能使用bufio，bufferSize为256kB。log库会自己定期Flush到文件。在主程序退出之前需要调用`dlog.Close()`，否则可能会丢失部分log。

#### syslog

    b, err := dlog.NewSyslogBackend(syslog.LOG_LOCAL3, "passport")
    //b, err := dlog.DialSyslogBackend("tcp", "127.0.0.1:123", LOG_USER, "passport")
    if err != nil {
        //...
    }
    dlog.SetLogging(dlog.INFO, b)
    dlog.Warningf("%d %s", 123, "test")

- 会建立多个writer，priority分别是syslog对应的LOG\_INFO, LOG\_WARNING, LOG\_ERR, LOG\_EMERG，tag对应修改为passport.INFO, passport.WARNING等等

#### 输出到多个后端

    b, _ := dlog.NewMultiBackend(b1, b2)
    dlog.SetLogging("INFO", b)
    defer dlog.Close()
    //...

#### logger

    logger := NewLogger("DEBUG", backend)
    logger.Info("asdfasd")
    logger.Close()

#### 指定depth
* 这种需求通常用于满足外部包装一个dlog helper, 防止depth只能打到helper内部的行号
* 使用接口:
```
func LogDepth(s Severity, depth int, format string, args ...interface{}) {
	logging.printfDepth(s, depth+1, format, args...)
}
```
* 调用方需要指定Severity/depth(从0开始, 每增加1层函数调用frame就+1)/format/args
* 考虑depth是较为高级的参数, 所以只提供一个low level接口, 不再单独封装INFO等public function

#### 按小时rotate
* 在配置文件中配置rotateByHour = true
* 如果是使用`b := dlog.NewFileBackend`得到的后端，请调用`b.SetRotateByHour(true)`来开启按小时滚动
* INFO.log.2016040113, 表示INFO log在2016/04/01, 下午13：00到14：00之间的log, 此log在14：00时生成
* 如果需要定时删除N个小时之前的log，请在配置文件中配置keepHours = N，例如想保留24小时的log，则keepHours = 24
* 如果是使用`b := dlog.NewFileBackend`得到的后端，请调用`b.SetKeepHours(N)`来指定保留多少小时的log

