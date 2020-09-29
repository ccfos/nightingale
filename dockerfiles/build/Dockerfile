FROM golang
WORKDIR /home/n9e
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/
RUN git clone  https://hub.fastgit.org/didi/nightingale.git
RUN nightingale/control build
