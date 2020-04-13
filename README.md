<img src="https://s3-gz01.didistatic.com/n9e-pub/image/n9e-logo-bg-white.png" width="200" alt="Nightingale"/>
<br>

[中文简介](README_ZH.md)

Nightingale is a fork of Open-Falcon, and all the core modules have been greatly optimized. It integrates the best practices of DiDi. You can think of it as the next generation of Open-Falcon, and use directly in production environment.

## Documentation

Nightingale user manual: [https://n9e.didiyun.com/](https://n9e.didiyun.com/)

## Compile

```bash
mkdir -p $GOPATH/src/github.com/didi
cd $GOPATH/src/github.com/didi
git clone https://github.com/didi/nightingale.git
cd nightingale
./control build
```

## Quickstart with Docker

We has offered a Docker demo for the users who want to give it a try. Before you get started, make sure you have installed **Docker** & **docker-compose** and there are some details you should know.

* We highly recommend users prepare a new VM environment to use it.
* All the core components will be installed on your OS according to the `docker-compose.yaml`.
* Nightingale will use the following ports, `80`, `5800`, `5810`, `5811`, `5820`, `5821`, `5830`, `5831`, `5840`, `5841`, `6379`, `2058`, `3306`.

Okay. Run it! Once the docker finish its jobs, visits http://your-env-ip in your broswer. Default username and password is `root:root`.
```bash
$ docker-compose up -d
```

![dashboard](https://user-images.githubusercontent.com/19553554/78956965-8b9c6180-7b16-11ea-9747-6ed5e62b068d.png)

## Team

[ulricqin](https://github.com/ulricqin) [710leo](https://github.com/710leo) [jsers](https://github.com/jsers) [hujter](https://github.com/hujter) [n4mine](https://github.com/n4mine) [heli567](https://github.com/heli567)

## Community

Nightingale is developed in open. Here we set up an organization, [github.com/n9e](https://github.com/n9e), which is used to communicate and contribute. We sincerely hope more developers can use their creativity to make lots of related projects for the Nightingale ecosystem.

## License

<img alt="Apache-2.0 license" src="https://s3-gz01.didistatic.com/n9e-pub/image/apache.jpeg" width="128">

Nightingale is available under the Apache-2.0 license. See the [LICENSE](LICENSE) file for more info.

