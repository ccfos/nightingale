<img src="https://s3-gz01.didistatic.com/n9e-pub/image/n9e-logo-bg-white.png" width="200" alt="Nightingale"/>
<br>

[English Introduction](README.md)

Nightingale 是一套衍生自 Open-Falcon 的互联网监控解决方案，融入了部分滴滴生产环境的最佳实践，灵活易用，稳定可靠，是一个生产环境直接可用的版本 :-)

## 文档

使用手册请参考：[夜莺使用手册](https://n9e.didiyun.com/)

## 编译

```bash
mkdir -p $GOPATH/src/github.com/didi
cd $GOPATH/src/github.com/didi
git clone https://github.com/didi/nightingale.git
cd nightingale
./control build
```

## 快速开始

使用 docker 和 docker-compose 环境可以快速部署一整套 nightingale 系统，涵盖了所有的核心组件。

* 强烈建议使用一个新的虚拟环境来部署和测试这个系统。
* 系统组件占用了以下端口，`80`, `5800`, `5810`, `5811`, `5820`, `5821`, `5830`, `5831`, `5840`, `5841`, `6379`, `2058`, `3306`，部署前请确保这些端口没有被使用。


使用 docker-compose 一键构建部署，完成以后可以使用浏览器打开 http://your-env-ip。 默认的登录账号密码均为 `root`。
```bash
$ docker-compose up -d
```

![dashboard](https://user-images.githubusercontent.com/19553554/78956965-8b9c6180-7b16-11ea-9747-6ed5e62b068d.png)

## 版本升级
如果需要从 `v1.4.0` 之前的版本升级到 `v1.4.0` , 按照 [v1.4.0](https://github.com/didi/nightingale/releases/tag/V1.4.0) release 说明操作即可

## 团队

[ulricqin](https://github.com/ulricqin) [710leo](https://github.com/710leo) [jsers](https://github.com/jsers) [hujter](https://github.com/hujter) [n4mine](https://github.com/n4mine) [heli567](https://github.com/heli567)

## 社区

[github.com/n9e](https://github.com/n9e) 是为夜莺所创建的 Organization，用于收集和开发夜莺周边项目。

## License

<img alt="Apache-2.0 license" src="https://s3-gz01.didistatic.com/n9e-pub/image/apache.jpeg" width="128">

Nightingale 基于 Apache-2.0 许可证进行分发和使用，更多信息参见 [LICENSE](LICENSE)。
