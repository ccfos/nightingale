---
name: Bug Report
about: Report a bug encountered while operating Nightingale
labels: kind/bug
---

**问题现象**:


**复现方法**:


**环境信息**:

- 夜莺服务端版本 (通过`./n9e-server -v`可得知版本):
- 夜莺客户端版本 (通过`./n9e-agentd -v`可得知版本):
- 操作系统版本 (通过`uname -a`可得知OS版本):

**日志线索**:

*日志分两部分，一个是logs目录下；另一部分是stdout，如果是systemd托管的，可以通过 `journalctl -u <n9e-server|n9e-agentd> -f` 查看*

